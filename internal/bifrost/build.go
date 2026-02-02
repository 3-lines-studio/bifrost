package bifrost

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func BuildCmd() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: build <main.go>\n")
		fmt.Fprintf(os.Stderr, "Example: build ./main.go\n")
		os.Exit(1)
	}

	mainFile := os.Args[1]

	mainDir := filepath.Dir(mainFile)
	if mainDir != "." && mainDir != "" {
		fmt.Printf("Changing to directory: %s\n", mainDir)
		if err := os.Chdir(mainDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to change to directory %s: %v\n", mainDir, err)
			os.Exit(1)
		}
	}

	fmt.Printf("Scanning %s for components...\n", filepath.Base(mainFile))
	componentPaths, err := extractComponentPaths(filepath.Base(mainFile))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to parse %s: %v\n", mainFile, err)
		os.Exit(1)
	}

	if len(componentPaths) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no NewPage calls found in %s\n", mainFile)
		os.Exit(1)
	}

	fmt.Printf("Found %d component(s):\n", len(componentPaths))
	for _, path := range componentPaths {
		fmt.Printf("  - %s\n", path)
	}

	entryDir := BifrostDir
	outdir := filepath.Join(entryDir, DistDir)

	if err := os.MkdirAll(entryDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create entry dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(outdir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create outdir: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nGenerating client entry files...")
	var entryFiles []string
	defer func() {
		fmt.Println("\nCleaning up entry files...")
		for _, entryFile := range entryFiles {
			if err := os.Remove(entryFile); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to remove entry file %s: %v\n", entryFile, err)
			} else {
				fmt.Printf("  Removed %s\n", entryFile)
			}
		}
	}()

	for _, componentPath := range componentPaths {
		entryName := EntryNameForPath(componentPath)
		entryPath := filepath.Join(entryDir, entryName+".tsx")

		componentImport, err := ComponentImportPath(entryPath, componentPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to get import path for %s: %v\n", componentPath, err)
			os.Exit(1)
		}

		if err := writeClientEntry(entryPath, componentImport); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to write entry file: %v\n", err)
			os.Exit(1)
		}
		entryFiles = append(entryFiles, entryPath)
		fmt.Printf("  Created %s\n", entryPath)
	}

	fmt.Println("\nStarting Bun renderer...")
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("bifrost-build-%d.sock", os.Getpid()))

	cmd := exec.Command("bun", "run", "-")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(), "BIFROST_SOCKET="+socket, "BIFROST_PROD=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = strings.NewReader(BunRendererSource)

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to start bun: %v\n", err)
		os.Exit(1)
	}
	defer cmd.Process.Kill()

	if err := waitForSocket(socket, 10*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socket)
		},
	}
	client := &http.Client{Transport: transport}

	fmt.Println("\nBuilding assets...")
	reqBody := map[string]interface{}{
		"entrypoints": entryFiles,
		"outdir":      outdir,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to marshal request: %v\n", err)
		os.Exit(1)
	}

	req, err := http.NewRequest("POST", "http://localhost/build", bytes.NewReader(jsonBody))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create request: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: build request failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to decode response: %v\n", err)
		os.Exit(1)
	}

	if result.Error != "" {
		fmt.Fprintf(os.Stderr, "Error: build failed: %s\n", result.Error)
		os.Exit(1)
	}

	if !result.OK {
		fmt.Fprintf(os.Stderr, "Error: build failed\n")
		os.Exit(1)
	}

	fmt.Println("Build successful!")

	fmt.Println("\nGenerating manifest...")
	man, err := generateManifest(outdir, componentPaths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to generate manifest: %v\n", err)
		os.Exit(1)
	}

	manifestPath := filepath.Join(entryDir, ManifestFile)
	manifestData, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to marshal manifest: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to write manifest: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created %s\n", manifestPath)

	fmt.Println("\nCopying public assets...")
	publicSrc := PublicDir
	publicDst := filepath.Join(entryDir, PublicDir)
	if err := copyPublicDir(publicSrc, publicDst); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to copy public dir: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nBuild complete! You can now compile your Go binary with embedded assets.")
}

func copyPublicDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if !info.IsDir() {
		return nil
	}

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return copyFile(path, dstPath)
	})
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, srcInfo.Mode())
}
