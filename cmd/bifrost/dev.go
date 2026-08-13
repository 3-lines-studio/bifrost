//go:build !windows

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/internal/builder"
)

func runDev(args []string) error {
	flags := flag.NewFlagSet("dev", flag.ContinueOnError)
	dir := flags.String("C", ".", "working directory")
	interval := flags.Duration("poll", 400*time.Millisecond, "Go file polling interval")
	vitePort := flags.Int("vite-port", 5173, "Vite development server port")
	if err := flags.Parse(args); err != nil {
		return err
	}
	packagePath := "."
	if flags.NArg() > 0 {
		packagePath = flags.Arg(0)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *vitePort < 1 || *vitePort > 65535 {
		return fmt.Errorf("invalid Vite port %d", *vitePort)
	}

	var child *exec.Cmd
	var devDir string
	stopChild := func() {
		if child == nil || child.Process == nil {
			return
		}
		_ = syscall.Kill(-child.Process.Pid, syscall.SIGTERM)
		_, _ = child.Process.Wait()
		child = nil
	}
	defer stopChild()

	buildAndStart := func() error {
		if err := builder.Build(ctx, builder.Options{Package: packagePath, Dir: *dir, Development: true, SourceMaps: false, OnDescribe: printRouteTable, OnOutput: func(output string) { devDir = output }, Version: bifrost.Version}); err != nil {
			return err
		}
		stopChild()
		child = exec.Command("go", "run", packagePath)
		child.Dir = *dir
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		child.Stdin = os.Stdin
		child.Env = append(os.Environ(), "BIFROST_DEV_DIR="+devDir, fmt.Sprintf("BIFROST_VITE_PORT=%d", *vitePort))
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
