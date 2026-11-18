//go:build !windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func runNativeDev(app appRouter) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var child *exec.Cmd
	stopChild := func() {
		if child == nil || child.Process == nil {
			return
		}
		_ = syscall.Kill(-child.Process.Pid, syscall.SIGTERM)
		_, _ = child.Process.Wait()
		child = nil
	}
	defer stopChild()

	build := func() (string, error) {
		prepared, err := prepareAppRouter(context.Background(), app.ProjectRoot, app.RouteRoot, 0, 0)
		if err != nil {
			return "", err
		}
		app = prepared
		executable := filepath.Join(app.ProjectRoot, ".bifrost", "dev-app")
		command := exec.Command("go", "build", "-o", executable, ".")
		command.Dir = app.WorkDir
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			return "", err
		}
		return executable, nil
	}

	start := func() error {
		executable, err := build()
		if err != nil {
			return err
		}
		stopChild()
		process := exec.Command(executable)
		process.Dir = app.ProjectRoot
		process.Stdout = os.Stdout
		process.Stderr = os.Stderr
		process.Env = append(os.Environ(), "BIFROST_DEV=1", "BIFROST_WASM_DIR="+wasmOutput(app.ProjectRoot))
		process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := process.Start(); err != nil {
			return err
		}
		child = process
		return nil
	}

	if err := buildWasm(ctx, app.ProjectRoot, false); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "bifrost:", err)
	}
	if err := start(); err != nil {
		return err
	}
	// The browser build is slower than the server one, so it runs beside the
	// app: the server reads the new wasm once it lands.
	webBuild := make(chan struct{}, 1)
	requestWebBuild := func() {
		select {
		case webBuild <- struct{}{}:
		default:
		}
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-webBuild:
				if err := buildWasm(ctx, app.ProjectRoot, false); err != nil {
					_, _ = fmt.Fprintln(os.Stderr, "bifrost:", err)
				}
			}
		}
	}()
	address := os.Getenv("BIFROST_ADDR")
	if address == "" {
		address = ":8080"
	}
	_, _ = fmt.Fprintln(os.Stdout, "Bifrost dev: native pages on "+address)

	previous, err := goTreeStamp(app.ProjectRoot)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			current, err := goTreeStamp(app.ProjectRoot)
			if err != nil || current == previous {
				continue
			}
			previous = current
			if err := start(); err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "bifrost:", err)
			}
			requestWebBuild()
		}
	}
}
