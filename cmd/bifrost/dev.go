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
	"sort"
	"strconv"
	"strings"
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
	if flags.NArg() > 1 {
		return fmt.Errorf("dev accepts one package path")
	}
	var router *appRouter
	buildDir := *dir
	projectRoot, routeRoot, isAppRouter, err := appRouterRoots(*dir, packagePath)
	if err != nil {
		return err
	}
	if isAppRouter {
		app, err := prepareAppRouter(context.Background(), projectRoot, routeRoot, 0, 0)
		if err != nil {
			return err
		}
		router = &app
		packagePath = app.Package
		*dir = app.ProjectRoot
		buildDir = app.WorkDir
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
		options := builder.Options{Package: packagePath, Dir: buildDir, Development: true, ExternalDevelopment: true, SourceMaps: false, ViteConfig: *viteConfig, OnDescribe: func(description protocol.DescribeResult) {
			printRouteTable(description, router.routeRows(), router.routeParams(), false)
		}, Version: bifrost.Version}
		if router != nil {
			options.Output = router.Output
		}
		return builder.Build(context.Background(), options)
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
	lock, err := claimDevelopmentServer(os.TempDir(), name, absolute)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	pruneStaleDevelopmentFiles(os.TempDir(), name+".lock")
	socketDir := filepath.Join(os.TempDir(), name+"-"+fmt.Sprint(os.Getpid()))
	defer func() { _ = os.RemoveAll(socketDir) }()
	socket := filepath.Join(socketDir, "vite.sock")
	fmt.Fprintf(os.Stderr, "bifrost: development server for %s, pid %d\n", filepath.ToSlash(absolute), os.Getpid())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var child *exec.Cmd
	var childBinary string
	var generation int
	var bridge *exec.Cmd
	var bridgeDone chan struct{}
	var sourceRoot string
	var buildOutput string
	devDir := filepath.Join(socketDir, "dev")

	// reload reads the route tree again and regenerates the views from it. What
	// the views hold is a path to each page, and a page that moved is a view that
	// points at nothing.
	reload := func(ctx context.Context) error {
		next, err := prepareAppRouter(ctx, projectRoot, routeRoot, 0, 0)
		if err != nil {
			return err
		}
		router = &next
		packagePath = next.Package
		return nil
	}

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
		options := builder.Options{Package: packagePath, Dir: buildDir, Development: true, SourceMaps: false, ViteConfig: *viteConfig, OnDescribe: func(description protocol.DescribeResult) {
			printRouteTable(description, router.routeRows(), router.routeParams(), false)
			sourceRoot = description.SourceRoot
			if err := writeRoutesFile(filepath.Join(socketDir, "routes.json"), description.Spec.Routes); err != nil {
				fmt.Fprintln(os.Stderr, "bifrost: write development routes:", err)
			}
		}, OnOutput: func(output string) { buildOutput = output }, Version: bifrost.Version}
		if router != nil {
			options.Output = router.Output
		}
		if err := builder.Build(ctx, options); err != nil {
			return err
		}
		if err := os.MkdirAll(devDir, 0o700); err != nil {
			return err
		}
		if err := copyEntries(filepath.Join(buildOutput, "entries"), filepath.Join(devDir, "entries")); err != nil {
			return err
		}
		if err := ensureBridge(); err != nil {
			return err
		}
		// The new binary is built next to the old one, and the old one is only
		// stopped once the new one is there. Measured, the other order leaves the
		// port with nothing behind it: a Go file saved mid-edit builds to an error,
		// the app was already killed, and the development server answers nothing
		// until the next successful rebuild. A binary that is being executed cannot
		// be written over either, so each generation gets a path of its own.
		generation++
		nextBinary := childBinaryPath(socketDir, generation)
		build := exec.Command("go", "build", "-o", nextBinary, packagePath)
		build.Dir = buildDir
		build.Stdout = os.Stdout
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			return fmt.Errorf("bifrost: build dev app: %w", err)
		}
		stopProcess(child)
		if childBinary != "" {
			_ = os.Remove(childBinary)
		}
		childBinary = nextBinary
		child = exec.Command(childBinary)
		child.Dir = buildDir
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
	last, err := developmentStamp(*dir, routeRoot)
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
			current, err := developmentStamp(*dir, routeRoot)
			if err != nil {
				fmt.Fprintln(os.Stderr, "bifrost: watch:", err)
				continue
			}
			if current == last {
				continue
			}
			last = current
			// The views are generated from the route tree, and a route that was added
			// or removed is a different tree: the views already on disk point at files
			// that are not there any more, and the build that reads them fails for good
			// because nothing regenerates them. Measured: a deleted route left the
			// server serving a view of it, and every later rebuild failed with
			// "Could not resolve .../app/probe/page.tsx" until the process was killed.
			if isAppRouter {
				if err := reload(ctx); err != nil {
					fmt.Fprintln(os.Stderr, "bifrost: reload routes:", err)
					continue
				}
			}
			if err := buildAndStart(); err != nil && !errors.Is(err, context.Canceled) {
				fmt.Fprintln(os.Stderr, "bifrost: rebuild:", err)
			}
		}
	}
}

// childBinaryPath names one generation of the development binary. The previous
// one is still executing while the next one is built, and a file a process is
// running from cannot be written over.
func childBinaryPath(socketDir string, generation int) string {
	return filepath.Join(socketDir, fmt.Sprintf("app-%d", generation))
}

// developmentStamp is everything that makes the generated app different: the Go
// sources it is built from, and *which* files the route tree has. What is inside
// a page is not here on purpose -- a page is a module of the frontend and Vite
// hands the new one to the browser without a rebuild -- but a page that appears
// or disappears is a route, and a route is compiled in.
func developmentStamp(dir, routeRoot string) (string, error) {
	sources, err := goTreeStamp(dir)
	if err != nil {
		return "", err
	}
	routes, err := routeTreeStamp(routeRoot)
	if err != nil {
		return "", err
	}
	return sources + "\n" + routes, nil
}

// routeTreeStamp names the files of the route tree, sorted so that the order a
// directory happens to be read in is not a change. Measured: with only the Go
// sources watched, `app/tempmedicion/page.tsx` was created and the route never
// existed -- no rebuild, and a 404 -- until the server was restarted. A private
// directory is left out the way the router leaves it out.
func routeTreeStamp(routeRoot string) (string, error) {
	absolute, err := filepath.Abs(routeRoot)
	if err != nil {
		return "", err
	}
	var names []string
	err = filepath.WalkDir(absolute, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == absolute {
				return nil
			}
			switch {
			case entry.Name() == ".bifrost", entry.Name() == "node_modules":
				return filepath.SkipDir
			case strings.HasPrefix(entry.Name(), "_"):
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(absolute, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(names)
	return strings.Join(names, "\n"), nil
}

// claimDevelopmentServer takes the lock of a directory's development server, or
// says which process holds it. The lock is a file whose only content is the pid
// of the run that took it: measured, the message that came out of a taken lock
// was "already running for this directory", which is the one thing the reader
// already knew, and a run whose app had to be killed by hand left a lock with
// nothing in it to find the process by.
func claimDevelopmentServer(base, name, dir string) (*os.File, error) {
	path := filepath.Join(base, name+".lock")
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		if holder := heldBy(path); holder != "" {
			return nil, fmt.Errorf("bifrost: development server is already running for %s, pid %s", filepath.ToSlash(dir), holder)
		}
		return nil, fmt.Errorf("bifrost: development server is already running for %s", filepath.ToSlash(dir))
	}
	if err := lock.Truncate(0); err == nil {
		_, _ = lock.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0)
	}
	return lock, nil
}

// heldBy reads the pid a lock file names, or nothing when it names none.
func heldBy(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

// requireViteInstalled fails early with an actionable message when Vite is
// missing from node_modules at dir or any of its parents.
func pruneStaleDevelopmentFiles(base, current string) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, entry := range entries {
		prefix, ok := strings.CutSuffix(entry.Name(), ".lock")
		if !ok || !strings.HasPrefix(prefix, "bifrost-dev-") || entry.Name() == current {
			continue
		}
		if !claimDevelopmentLock(filepath.Join(base, entry.Name())) {
			continue
		}
		_ = os.Remove(filepath.Join(base, entry.Name()))
		pruneDevelopmentDirectories(base, prefix)
	}
}

func claimDevelopmentLock(path string) bool {
	file, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil
}

func pruneDevelopmentDirectories(base, prefix string) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix+"-") {
			_ = os.RemoveAll(filepath.Join(base, entry.Name()))
		}
	}
}

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

func copyEntries(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		staging := destination + ".tmp"
		if err := os.WriteFile(staging, content, 0o600); err != nil {
			return err
		}
		return os.Rename(staging, destination)
	})
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
