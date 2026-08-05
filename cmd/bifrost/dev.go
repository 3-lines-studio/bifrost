package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/3-lines-studio/bifrost/internal/adapters/cli"
	"github.com/3-lines-studio/bifrost/internal/adapters/fs"
)

func parseDevFlags(args []string) (mainFile string, port, appPort int) {
	port = 3000
	appPort = 8080
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--port":
			if i+1 < len(args) {
				if p, err := strconv.Atoi(args[i+1]); err == nil {
					port = p
					i++
				}
			}
		case strings.HasPrefix(arg, "--port="):
			if p, err := strconv.Atoi(strings.TrimPrefix(arg, "--port=")); err == nil {
				port = p
			}
		case arg == "--app-port":
			if i+1 < len(args) {
				if p, err := strconv.Atoi(args[i+1]); err == nil {
					appPort = p
					i++
				}
			}
		case strings.HasPrefix(arg, "--app-port="):
			if p, err := strconv.Atoi(strings.TrimPrefix(arg, "--app-port=")); err == nil {
				appPort = p
			}
		case mainFile == "" && !strings.HasPrefix(arg, "-"):
			mainFile = arg
		}
	}
	return mainFile, port, appPort
}

func printDevUsage() {
	output := cli.NewOutput()
	output.PrintHeader("Bifrost Dev")
	fmt.Println()
	fmt.Println("Usage: bifrost dev <main.go> [flags]")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  bifrost dev ./main.go")
	fmt.Println("  bifrost dev ./main.go --port 4000")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --port <n>      Dev server port (default: 3000)")
	fmt.Println("  --app-port <n>  App's internal port (default: 8080)")
}

type managedChild struct {
	cmd      *exec.Cmd
	exitCh   chan int
	stopOnce sync.Once
}

func startManagedChild(cwd, binPath string) *managedChild {
	cmd := exec.Command(binPath)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "BIFROST_DEV=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	setSysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		return nil
	}
	mc := &managedChild{
		cmd:    cmd,
		exitCh: make(chan int, 1),
	}
	go func() {
		err := cmd.Wait()
		code := 0
		if err != nil {
			code = 1
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		}
		mc.exitCh <- code
	}()
	return mc
}

func (mc *managedChild) stop() {
	mc.stopOnce.Do(func() {
		signalProcessTree(mc.cmd)
		select {
		case <-mc.exitCh:
		case <-time.After(3 * time.Second):
			forceKillProcessTree(mc.cmd)
			<-mc.exitCh
		}
	})
}

func runDev(args []string) {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		printDevUsage()
		os.Exit(0)
	}

	mainFile, port, appPort := parseDevFlags(args)

	if mainFile == "" {
		printDevUsage()
		output := cli.NewOutput()
		output.PrintError("Missing main.go file argument")
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		output := cli.NewOutput()
		output.PrintHeader("Bifrost Dev")
		output.PrintError("Failed to get current working directory: %v", err)
		os.Exit(1)
	}

	mainFileAbs := mainFile
	if !filepath.IsAbs(mainFile) {
		mainFileAbs = filepath.Join(cwd, mainFile)
	}

	goModRoot := findGoModRoot(filepath.Dir(mainFileAbs))
	appRoot := filepath.Dir(mainFileAbs)

	output := cli.NewOutput()
	output.PrintHeader("Bifrost Dev")

	bifrostDir := filepath.Join(appRoot, ".bifrost")
	fsAdapter := fs.NewOSFileSystem()
	if err := ensureBifrostDir(fsAdapter, bifrostDir); err != nil {
		output.PrintError("Failed to prepare .bifrost directory: %v", err)
		os.Exit(1)
	}

	tmpDir := filepath.Join(appRoot, "tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		output.PrintError("Failed to create tmp directory: %v", err)
		os.Exit(1)
	}
	binPath := filepath.Join(tmpDir, "bifrost-dev-main")

	if err := goBuild(goModRoot, mainFileAbs, binPath); err != nil {
		output.PrintError("Build failed: %v", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mc := startManagedChild(goModRoot, binPath)
	if mc == nil {
		output.PrintError("Failed to start app process")
		os.Exit(1)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	proxy, err := newDevProxy(port, appPort)
	if err != nil {
		mc.stop()
		output.PrintError("Failed to create proxy: %v", err)
		os.Exit(1)
	}
	proxyErrCh, err := proxy.Start(ctx)
	if err != nil {
		mc.stop()
		output.PrintError("Failed to start proxy: %v", err)
		os.Exit(1)
	}

	if err := waitForApp(ctx, appPort, 30*time.Second); err != nil {
		mc.stop()
		output.PrintError("App failed to start: %v", err)
		os.Exit(1)
	}

	fmt.Println()
	output.PrintSuccess("Proxy:   http://localhost:%d", port)
	output.PrintSuccess("App:     http://127.0.0.1:%d (internal)", appPort)
	fmt.Println("  Watching .go files (rebuild + restart) and frontend files (live reload)")
	fmt.Println("  Press Ctrl-C to stop")
	fmt.Println()

	watcher := newFileWatcher(goModRoot)
	goChanges, feChanges := watcher.Start(ctx)

	for {
		var exitCh chan int
		if mc != nil {
			exitCh = mc.exitCh
		}

		select {
		case <-sigCh:
			output.PrintStep("", "Shutting down...")
			cancel()
			shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = proxy.Stop(shutCtx)
			shutCancel()
			if mc != nil {
				mc.stop()
			}
			_ = os.Remove(binPath)
			os.Exit(0)

		case code := <-exitCh:
			output.PrintWarning("App exited (code %d) — saving a .go file will restart", code)
			mc = nil

		case <-goChanges:
			output.PrintStep("", "Go file changed — rebuilding...")
			if err := goBuild(goModRoot, mainFileAbs, binPath); err != nil {
				output.PrintError("Rebuild failed — keeping previous binary running")
				continue
			}
			if mc != nil {
				mc.stop()
			}
			mc = startManagedChild(goModRoot, binPath)
			if mc == nil {
				output.PrintError("Failed to start app after rebuild")
			} else {
				output.PrintSuccess("Rebuild complete")
				if err := waitForApp(ctx, appPort, 5*time.Second); err == nil {
					proxy.BroadcastReload()
				}
			}

		case <-feChanges:
			output.PrintStep("", "Frontend change — reload browser")
			proxy.BroadcastReload()

		case err := <-proxyErrCh:
			if err != nil {
				output.PrintError("Proxy error: %v", err)
			}
		}
	}
}

func goBuild(goModRoot, mainFile, binPath string) error {
	tmpBin := binPath + ".new"
	cmd := exec.Command("go", "build", "-o", tmpBin, filepath.Dir(mainFile))
	cmd.Dir = goModRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return os.Rename(tmpBin, binPath)
}
