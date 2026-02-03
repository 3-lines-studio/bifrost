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
		PrintHeader("Bifrost Build")
		PrintError("Missing main.go file argument")
		fmt.Println()
		PrintInfo("Usage: bifrost-build <main.go>")
		PrintStep(EmojiInfo, "Example: bifrost-build ./main.go")
		os.Exit(1)
	}

	mainFile := os.Args[1]

	PrintHeader("Bifrost Build")

	originalCwd, err := os.Getwd()
	if err != nil {
		PrintError("Failed to get current working directory: %v", err)
		os.Exit(1)
	}

	mainDir := filepath.Dir(mainFile)
	if mainDir != "." && mainDir != "" {
		PrintStep(EmojiFolder, "Changing to directory: %s", mainDir)
		if err := os.Chdir(mainDir); err != nil {
			PrintError("Failed to change to directory %s: %v", mainDir, err)
			os.Exit(1)
		}
	}

	PrintStep(EmojiSearch, "Scanning %s for components...", filepath.Base(mainFile))
	componentPaths, err := extractComponentPaths(filepath.Base(mainFile))
	if err != nil {
		PrintError("Failed to parse %s: %v", mainFile, err)
		os.Exit(1)
	}

	if len(componentPaths) == 0 {
		PrintError("No NewPage calls found in %s", mainFile)
		os.Exit(1)
	}

	PrintSuccess("Found %d component(s)", len(componentPaths))
	for _, path := range componentPaths {
		PrintFile(path)
	}

	entryDir := filepath.Join(originalCwd, BifrostDir)
	outdir := filepath.Join(entryDir, DistDir)

	PrintStep(EmojiFolder, "Creating output directories...")
	if err := os.MkdirAll(entryDir, 0755); err != nil {
		PrintError("Failed to create entry dir: %v", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(outdir, 0755); err != nil {
		PrintError("Failed to create outdir: %v", err)
		os.Exit(1)
	}
	PrintSuccess("Directories ready")

	PrintStep(EmojiFile, "Generating client entry files...")
	var entryFiles []string
	defer func() {
		PrintStep(EmojiGear, "Cleaning up entry files...")
		for _, entryFile := range entryFiles {
			if err := os.Remove(entryFile); err != nil {
				PrintWarning("Failed to remove entry file %s: %v", entryFile, err)
			}
		}
	}()

	for _, componentPath := range componentPaths {
		entryName := EntryNameForPath(componentPath)
		entryPath := filepath.Join(entryDir, entryName+".tsx")

		absComponentPath := filepath.Join(originalCwd, componentPath)

		componentImport, err := ComponentImportPath(entryPath, absComponentPath)
		if err != nil {
			PrintError("Failed to get import path for %s: %v", componentPath, err)
			os.Exit(1)
		}

		if err := writeClientEntry(entryPath, componentImport); err != nil {
			PrintError("Failed to write entry file: %v", err)
			os.Exit(1)
		}
		entryFiles = append(entryFiles, entryPath)
		PrintFile(entryPath)
	}
	PrintSuccess("Generated %d entry file(s)", len(entryFiles))

	PrintStep(EmojiRocket, "Starting Bun renderer...")
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("bifrost-build-%d.sock", os.Getpid()))

	cmd := exec.Command("bun", "run", "--smol", "-")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(), "BIFROST_SOCKET="+socket, "BIFROST_PROD=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = strings.NewReader(BunRendererSource)

	if err := cmd.Start(); err != nil {
		PrintError("Failed to start bun: %v", err)
		os.Exit(1)
	}
	defer cmd.Process.Kill()

	spinner := NewSpinner("Waiting for renderer")
	spinner.Start()
	if err := waitForSocket(socket, 10*time.Second); err != nil {
		spinner.Stop()
		PrintError("%v", err)
		os.Exit(1)
	}
	spinner.Stop()
	PrintSuccess("Renderer ready")

	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socket)
		},
	}
	client := &http.Client{Transport: transport}

	PrintStep(EmojiZap, "Building assets...")
	buildSpinner := NewSpinner("Building client bundle")
	buildSpinner.Start()

	reqBody := map[string]interface{}{
		"entrypoints": entryFiles,
		"outdir":      outdir,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		buildSpinner.Stop()
		PrintError("Failed to marshal request: %v", err)
		os.Exit(1)
	}

	req, err := http.NewRequest("POST", "http://localhost/build", bytes.NewReader(jsonBody))
	if err != nil {
		buildSpinner.Stop()
		PrintError("Failed to create request: %v", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		buildSpinner.Stop()
		PrintError("Build request failed: %v", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var result struct {
		OK    bool `json:"ok"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		buildSpinner.Stop()
		PrintError("Failed to decode response: %v", err)
		os.Exit(1)
	}

	buildSpinner.Stop()

	if result.Error != nil {
		PrintError("Build failed: %s", result.Error.Message)
		os.Exit(1)
	}

	if !result.OK {
		PrintError("Build failed")
		os.Exit(1)
	}

	PrintSuccess("Build successful")

	PrintStep(EmojiPackage, "Generating manifest...")
	man, err := generateManifest(outdir, componentPaths)
	if err != nil {
		PrintError("Failed to generate manifest: %v", err)
		os.Exit(1)
	}

	manifestPath := filepath.Join(entryDir, ManifestFile)
	manifestData, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		PrintError("Failed to marshal manifest: %v", err)
		os.Exit(1)
	}

	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		PrintError("Failed to write manifest: %v", err)
		os.Exit(1)
	}
	PrintFile(manifestPath)
	PrintSuccess("Manifest created")

	PrintStep(EmojiCopy, "Copying public assets...")
	publicSrc := filepath.Join(originalCwd, PublicDir)
	publicDst := filepath.Join(entryDir, PublicDir)
	if err := copyPublicDir(publicSrc, publicDst); err != nil {
		PrintError("Failed to copy public dir: %v", err)
		os.Exit(1)
	}
	PrintSuccess("Assets copied")

	PrintDone("Build complete! You can now compile your Go binary with embedded assets.")
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
