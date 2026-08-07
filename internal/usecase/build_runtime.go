package usecase

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func (s *BuildService) runExportMode(moduleRoot, appRoot, bifrostDir string, manifest *core.Manifest, mainFile string) error {
	binaryPath := filepath.Join(bifrostDir, "temp-app")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", binaryPath, filepath.Dir(mainFile))
	cmd.Dir = moduleRoot
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
	exportCmd.Dir = appRoot
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
