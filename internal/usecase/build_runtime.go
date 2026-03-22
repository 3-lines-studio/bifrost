package usecase

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func (s *BuildService) runExportMode(originalCwd, bifrostDir string, manifest *core.Manifest, mainFile string) error {
	binaryPath := filepath.Join(bifrostDir, "temp-app")
	cmd := exec.Command("go", "build", "-o", binaryPath, mainFile)
	cmd.Dir = originalCwd
	cmd.Env = append(os.Environ(),
		"BIFROST_EXPORT=1",
		"BIFROST_EXPORT_DIR="+bifrostDir,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to build app for export: %v\nOutput: %s", err, output)
	}

	if err := os.Chmod(binaryPath, 0755); err != nil {
		return fmt.Errorf("failed to make binary executable: %w", err)
	}

	defer func() { _ = os.Remove(binaryPath) }()

	exportCmd := exec.Command(binaryPath)
	exportCmd.Dir = originalCwd
	exportCmd.Env = append(os.Environ(),
		"BIFROST_EXPORT=1",
		"BIFROST_EXPORT_DIR="+bifrostDir,
	)
	exportCmd.Stdout = os.Stdout
	exportCmd.Stderr = os.Stderr

	if err := exportCmd.Run(); err != nil {
		return fmt.Errorf("export mode failed: %w", err)
	}

	exportManifestPath := filepath.Join(bifrostDir, "export-manifest.json")
	exportData, err := os.ReadFile(exportManifestPath)
	if err != nil {
		return fmt.Errorf("failed to read export manifest: %w", err)
	}

	var exportManifest core.Manifest
	if err := json.Unmarshal(exportData, &exportManifest); err != nil {
		return fmt.Errorf("failed to parse export manifest: %w", err)
	}

	for entryName, entry := range exportManifest.Entries {
		if existing, ok := manifest.Entries[entryName]; ok {
			existing.StaticRoutes = entry.StaticRoutes
			manifest.Entries[entryName] = existing
		} else {
			manifest.Entries[entryName] = entry
		}
	}

	_ = os.Remove(exportManifestPath)

	return nil
}

func (s *BuildService) compileEmbeddedRuntime(bifrostDir string) error {
	runtimeDir := filepath.Join(bifrostDir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return fmt.Errorf("failed to create runtime dir: %w", err)
	}

	tempSourcePath := filepath.Join(runtimeDir, "renderer.ts")
	sourceContent := s.adapter.ProdRendererSource()

	if err := os.WriteFile(tempSourcePath, []byte(sourceContent), 0644); err != nil {
		return fmt.Errorf("failed to write temp source: %w", err)
	}

	outfile := filepath.Join(runtimeDir, "bifrost-renderer")
	if os.Getenv("GOOS") == "windows" || (os.Getenv("GOOS") == "" && os.PathSeparator == '\\') {
		outfile += ".exe"
	}

	cmd := exec.Command(
		"bun",
		"build",
		"--compile",
		"--outfile",
		outfile,
		"--no-compile-autoload-dotenv",
		"--no-compile-autoload-bunfig",
		tempSourcePath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		_ = os.Remove(tempSourcePath)
		return fmt.Errorf("bun compile failed: %w", err)
	}

	_ = os.Remove(tempSourcePath)
	return nil
}
