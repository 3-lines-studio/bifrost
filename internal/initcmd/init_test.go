package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_Minimal(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "myapp")

	err := Run(projectDir, "minimal")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	expectedFiles := []string{
		"main.go",
		"go.mod",
		"package.json",
		"tsconfig.json",
		".gitignore",
		".bifrost/.gitkeep",
		"pages/home.tsx",
		"Makefile",
		".air.toml",
		"Dockerfile",
	}

	for _, file := range expectedFiles {
		path := filepath.Join(projectDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file %s to be created, but it doesn't exist", file)
		}
	}

	goModPath := filepath.Join(projectDir, "go.mod")
	goModContent, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("Failed to read go.mod: %v", err)
	}

	expectedModule := "module myapp"
	if string(goModContent)[:len(expectedModule)] != expectedModule {
		t.Errorf("go.mod doesn't contain expected module. Got:\n%s", string(goModContent))
	}

	if strings.Contains(string(goModContent), "github.com/3-lines-studio/bifrost v0.0.0") {
		t.Errorf("go.mod contains hardcoded bifrost v0.0.0 which is invalid; go mod tidy should resolve the correct version")
	}

	makefilePath := filepath.Join(projectDir, "Makefile")
	makefileContent, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("Failed to read Makefile: %v", err)
	}

	if !strings.Contains(string(makefileContent), "doctor:") {
		t.Errorf("Makefile doesn't contain doctor target")
	}
}

func TestRun_DirectoryNotEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "myapp")

	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(projectDir, "existing.txt"), []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create existing file: %v", err)
	}

	err := Run(projectDir, "minimal")
	if err == nil {
		t.Error("Run() expected error for non-empty directory, got nil")
	}
}

func TestRepairBifrostDir(t *testing.T) {
	tmpDir := t.TempDir()

	err := RepairBifrostDir(tmpDir)
	if err != nil {
		t.Fatalf("RepairBifrostDir() error = %v", err)
	}

	gitkeepPath := filepath.Join(tmpDir, ".bifrost", ".gitkeep")
	if _, err := os.Stat(gitkeepPath); os.IsNotExist(err) {
		t.Errorf("Expected .bifrost/.gitkeep to be created, but it doesn't exist")
	}

	content, err := os.ReadFile(gitkeepPath)
	if err != nil {
		t.Fatalf("Failed to read .gitkeep: %v", err)
	}

	expectedContent := "# This file ensures .bifrost directory exists for go:embed\n"
	if string(content) != expectedContent {
		t.Errorf(".gitkeep content = %q, want %q", string(content), expectedContent)
	}
}

func TestRepairBifrostDir_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()

	err := RepairBifrostDir(tmpDir)
	if err != nil {
		t.Fatalf("First RepairBifrostDir() error = %v", err)
	}

	err = RepairBifrostDir(tmpDir)
	if err != nil {
		t.Fatalf("Second RepairBifrostDir() error = %v", err)
	}

	gitkeepPath := filepath.Join(tmpDir, ".bifrost", ".gitkeep")
	content, err := os.ReadFile(gitkeepPath)
	if err != nil {
		t.Fatalf("Failed to read .gitkeep: %v", err)
	}

	expectedContent := "# This file ensures .bifrost directory exists for go:embed\n"
	if string(content) != expectedContent {
		t.Errorf(".gitkeep content changed = %q, want %q", string(content), expectedContent)
	}
}

func TestRun_Spa(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "myspa")

	err := Run(projectDir, "spa")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	expectedFiles := []string{
		"main.go",
		"go.mod",
		"package.json",
		"tsconfig.json",
		".gitignore",
		".bifrost/.gitkeep",
		"pages/home.tsx",
		"Makefile",
		".air.toml",
		"Dockerfile",
	}

	for _, file := range expectedFiles {
		path := filepath.Join(projectDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file %s to be created, but it doesn't exist", file)
		}
	}

	mainGoPath := filepath.Join(projectDir, "main.go")
	mainGoContent, err := os.ReadFile(mainGoPath)
	if err != nil {
		t.Fatalf("Failed to read main.go: %v", err)
	}

	if !strings.Contains(string(mainGoContent), "WithClientOnly()") {
		t.Errorf("main.go should contain WithClientOnly() for SPA template")
	}

	goModPath := filepath.Join(projectDir, "go.mod")
	goModContent, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("Failed to read go.mod: %v", err)
	}

	if strings.Contains(string(goModContent), "github.com/3-lines-studio/bifrost v0.0.0") {
		t.Errorf("go.mod contains hardcoded bifrost v0.0.0 which is invalid; go mod tidy should resolve the correct version")
	}
}

func TestRun_Desktop(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "mydesktop")

	err := Run(projectDir, "desktop")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	expectedFiles := []string{
		"main.go",
		"go.mod",
		"package.json",
		"tsconfig.json",
		".gitignore",
		".bifrost/.gitkeep",
		"pages/home.tsx",
		"Makefile",
		".air.toml",
		"public/icon.png",
	}

	for _, file := range expectedFiles {
		path := filepath.Join(projectDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file %s to be created, but it doesn't exist", file)
		}
	}

	mainGoPath := filepath.Join(projectDir, "main.go")
	mainGoContent, err := os.ReadFile(mainGoPath)
	if err != nil {
		t.Fatalf("Failed to read main.go: %v", err)
	}

	if !strings.Contains(string(mainGoContent), "systray") {
		t.Errorf("main.go should contain systray for desktop template")
	}

	if !strings.Contains(string(mainGoContent), "webview") {
		t.Errorf("main.go should contain webview for desktop template")
	}

	goModPath := filepath.Join(projectDir, "go.mod")
	goModContent, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("Failed to read go.mod: %v", err)
	}

	if strings.Contains(string(goModContent), "github.com/3-lines-studio/bifrost v0.0.0") {
		t.Errorf("go.mod contains hardcoded bifrost v0.0.0 which is invalid; go mod tidy should resolve the correct version")
	}

	dockerfilePath := filepath.Join(projectDir, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); !os.IsNotExist(err) {
		t.Errorf("desktop template should not have Dockerfile, but it exists")
	}
}

func TestRun_InvalidTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "myapp")

	err := Run(projectDir, "invalid")
	if err == nil {
		t.Error("Run() expected error for invalid template, got nil")
	}
}
