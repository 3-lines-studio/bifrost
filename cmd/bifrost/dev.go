//go:build !windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/internal/builder"
	"github.com/3-lines-studio/bifrost/internal/protocol"
)

func runDev(args []string) error {
	flags := flag.NewFlagSet("dev", flag.ContinueOnError)
	dir := flags.String("C", ".", "working directory")
	interval := flags.Duration("poll", 400*time.Millisecond, "Go file polling interval")
	vitePort := flags.Int("vite-port", 0, "Vite development server port (0 picks a free port)")
	viteConfig := flags.String("vite-config", "", "path to the Vite configuration file")
	prepareOnly := flags.Bool("prepare-only", false, "prepare development assets without starting the application")
	if err := flags.Parse(args); err != nil {
		return err
	}
	packagePath := "."
	if flags.NArg() > 0 {
		packagePath = flags.Arg(0)
	}
	if *vitePort < 0 || *vitePort > 65535 {
		return fmt.Errorf("invalid Vite port %d", *vitePort)
	}
	if *vitePort == 0 {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return fmt.Errorf("bifrost: pick Vite development port: %w", err)
		}
		*vitePort = listener.Addr().(*net.TCPAddr).Port
		if err := listener.Close(); err != nil {
			return err
		}
	}
	if *prepareOnly {
		return builder.Build(context.Background(), builder.Options{Package: packagePath, Dir: *dir, Development: true, ExternalDevelopment: true, SourceMaps: false, ViteConfig: *viteConfig, OnDescribe: printRouteTable, Version: bifrost.Version})
	}
	if _, err := exec.LookPath("bun"); err != nil {
		return errors.New("bifrost: Bun is required for development; install it from https://bun.sh")
	}
	if err := requireViteInstalled(*dir); err != nil {
		return err
	}

	absolute, err := filepath.Abs(*dir)
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(absolute))
	name := "bifrost-dev-" + hex.EncodeToString(digest[:8])
	lock, err := os.OpenFile(filepath.Join(os.TempDir(), name+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("bifrost: development server is already running for this directory")
	}
	socketDir := filepath.Join(os.TempDir(), name+"-"+fmt.Sprint(os.Getpid()))
	defer func() { _ = os.RemoveAll(socketDir) }()
	socket := filepath.Join(socketDir, "vite.sock")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var child *exec.Cmd
	var bridge *exec.Cmd
	var bridgeDone chan struct{}
	var sourceRoot string
	var devDir string

	stopProcess := func(process *exec.Cmd) {
		if process == nil || process.Process == nil {
			return
		}
		_ = syscall.Kill(-process.Process.Pid, syscall.SIGTERM)
		_, _ = process.Process.Wait()
	}
	defer func() {
		stopProcess(child)
		stopProcess(bridge)
	}()

	bridgeAlive := func() bool {
		if bridgeDone == nil {
			return false
		}
		select {
		case <-bridgeDone:
			return false
		default:
			return true
		}
	}

	ensureBridge := func() error {
		if bridgeAlive() {
			return nil
		}
		stopProcess(bridge)
		bridge = nil
		bridgeDone = nil
		if sourceRoot == "" || devDir == "" {
			return errors.New("bifrost: bridge state is missing")
		}
		script := filepath.Join(devDir, "entries", "vite-dev.ts")
		if _, err := os.Stat(script); err != nil {
			return fmt.Errorf("bifrost: development Vite bridge: %w", err)
		}
		if err := os.MkdirAll(socketDir, 0o700); err != nil {
			return err
		}
		_ = os.Remove(socket)
		process := exec.Command("bun", "run", script)
		process.Dir = sourceRoot
		process.Stdout = os.Stdout
		process.Stderr = os.Stderr
		process.Env = append(os.Environ(),
			"BIFROST_SOCKET="+socket,
			"BIFROST_VITE_ROOT="+sourceRoot,
			fmt.Sprintf("BIFROST_VITE_PORT=%d", *vitePort),
			"BIFROST_ROUTES_FILE="+filepath.Join(socketDir, "routes.json"),
			"BIFROST_VITE_CONFIG="+*viteConfig,
			"BIFROST_DEV_ENTRIES="+filepath.Join(devDir, "entries"),
		)
		process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := process.Start(); err != nil {
			return fmt.Errorf("bifrost: start Vite development bridge: %w", err)
		}
		bridge = process
		done := make(chan struct{})
		bridgeDone = done
		go func() {
			_ = process.Wait()
			close(done)
		}()
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			if bridgeHealthy(socket) {
				return nil
			}
			select {
			case <-done:
				return errors.New("bifrost: Vite development bridge exited during startup")
			default:
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			time.Sleep(50 * time.Millisecond)
		}
		return errors.New("bifrost: Vite development bridge did not become healthy")
	}

	buildAndStart := func() error {
		if err := builder.Build(ctx, builder.Options{Package: packagePath, Dir: *dir, Development: true, SourceMaps: false, ViteConfig: *viteConfig, OnDescribe: func(description protocol.DescribeResult) {
			printRouteTable(description)
			sourceRoot = description.SourceRoot
			if err := writeRoutesFile(filepath.Join(socketDir, "routes.json"), description.Spec.Routes); err != nil {
				fmt.Fprintln(os.Stderr, "bifrost: write development routes:", err)
			}
		}, OnOutput: func(output string) { devDir = output }, Version: bifrost.Version}); err != nil {
			return err
		}
		if err := ensureBridge(); err != nil {
			return err
		}
		stopProcess(child)
		child = exec.Command("go", "run", packagePath)
		child.Dir = *dir
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		child.Stdin = os.Stdin
		child.Env = append(os.Environ(), "BIFROST_DEV_DIR="+devDir, fmt.Sprintf("BIFROST_VITE_PORT=%d", *vitePort), "BIFROST_VITE_SOCKET="+socket, "BIFROST_VITE_CONFIG="+*viteConfig)
		child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		return child.Start()
	}
	if err := buildAndStart(); err != nil {
		return err
	}
	last, err := goTreeStamp(*dir)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if !bridgeAlive() {
				fmt.Fprintln(os.Stderr, "bifrost: development bridge stopped; restarting Vite")
				if err := ensureBridge(); err != nil {
					fmt.Fprintln(os.Stderr, "bifrost: bridge restart:", err)
					continue
				}
			}
			current, err := goTreeStamp(*dir)
			if err != nil {
				fmt.Fprintln(os.Stderr, "bifrost: watch:", err)
				continue
			}
			if current == last {
				continue
			}
			last = current
			if err := buildAndStart(); err != nil && !errors.Is(err, context.Canceled) {
				fmt.Fprintln(os.Stderr, "bifrost: rebuild:", err)
			}
		}
	}
}

// requireViteInstalled fails early with an actionable message when Vite is
// missing from node_modules at dir or any of its parents.
func requireViteInstalled(dir string) error {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	current := absolute
	for {
		if _, err := os.Stat(filepath.Join(current, "node_modules", "vite", "package.json")); err == nil {
			return nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return errors.New("bifrost: Vite is not installed; run `bun install` first")
}

func writeRoutesFile(path string, routes []protocol.RouteSpec) error {
	data, err := json.Marshal(routes)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func bridgeHealthy(socket string) bool {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 250 * time.Millisecond}).DialContext(ctx, "unix", socket)
			},
		},
		Timeout: 500 * time.Millisecond,
	}
	response, err := client.Get("http://bifrost/health")
	if err != nil {
		return false
	}
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode == http.StatusOK
}

func goTreeStamp(root string) (string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	var latest int64
	var count int64
	err = filepath.WalkDir(absolute, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".bifrost", "node_modules":
				if path != absolute {
					return filepath.SkipDir
				}
			}
			return nil
		}
		name := entry.Name()
		if filepath.Ext(name) != ".go" && name != "go.mod" && name != "go.sum" && name != "package.json" && name != "bun.lock" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if modified := info.ModTime().UnixNano(); modified > latest {
			latest = modified
		}
		count++
		return nil
	})
	return fmt.Sprintf("%d:%d", latest, count), err
}
