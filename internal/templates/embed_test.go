package templates

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
)

func TestProcessFilename(t *testing.T) {
	data := TemplateData{
		Module: "test-module",
	}

	tests := []struct {
		name         string
		filename     string
		wantFilename string
		wantIsTmpl   bool
	}{
		{
			name:         "tmpl file gets processed",
			filename:     "main.go.tmpl",
			wantFilename: "main.go",
			wantIsTmpl:   true,
		},
		{
			name:         "regular file unchanged",
			filename:     "package.json",
			wantFilename: "package.json",
			wantIsTmpl:   false,
		},
		{
			name:         "nested tmpl file",
			filename:     "pages/home.tsx.tmpl",
			wantFilename: "pages/home.tsx",
			wantIsTmpl:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFilename, gotIsTmpl := ProcessFilename(tt.filename, data)
			if gotFilename != tt.wantFilename {
				t.Errorf("ProcessFilename(%q) filename = %q, want %q", tt.filename, gotFilename, tt.wantFilename)
			}
			if gotIsTmpl != tt.wantIsTmpl {
				t.Errorf("ProcessFilename(%q) isTmpl = %v, want %v", tt.filename, gotIsTmpl, tt.wantIsTmpl)
			}
		})
	}
}

func TestProcessContent(t *testing.T) {
	data := TemplateData{
		Module: "github.com/example/myapp",
	}

	tests := []struct {
		name       string
		content    string
		isTemplate bool
		want       string
	}{
		{
			name:       "non-template content unchanged",
			content:    "module example\n\ngo 1.21",
			isTemplate: false,
			want:       "module example\n\ngo 1.21",
		},
		{
			name:       "template with Module placeholder",
			content:    "module {{.Module}}\n\ngo 1.21",
			isTemplate: true,
			want:       "module github.com/example/myapp\n\ngo 1.21",
		},
		{
			name:       "template content treated as non-template",
			content:    "module {{.Module}}",
			isTemplate: false,
			want:       "module {{.Module}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProcessContent([]byte(tt.content), tt.isTemplate, data)
			if string(got) != tt.want {
				t.Errorf("ProcessContent(%q) = %q, want %q", tt.content, string(got), tt.want)
			}
		})
	}
}

func TestDeriveModuleName(t *testing.T) {
	tests := []struct {
		name       string
		projectDir string
		want       string
	}{
		{
			name:       "normal directory name",
			projectDir: "/home/user/myapp",
			want:       "myapp",
		},
		{
			name:       "current directory",
			projectDir: ".",
			want:       "myapp",
		},
		{
			name:       "root directory",
			projectDir: "/",
			want:       "myapp",
		},
		{
			name:       "empty directory",
			projectDir: "",
			want:       "myapp",
		},
		{
			name:       "path with multiple components",
			projectDir: "/path/to/project",
			want:       "project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveModuleName(tt.projectDir)
			if got != tt.want {
				t.Errorf("DeriveModuleName(%q) = %q, want %q", tt.projectDir, got, tt.want)
			}
		})
	}
}

func TestGetMinimalTemplate(t *testing.T) {
	fs, err := GetMinimalTemplate()
	if err != nil {
		t.Fatalf("GetMinimalTemplate() error = %v", err)
	}
	if fs == nil {
		t.Error("GetMinimalTemplate() returned nil fs")
	}
}

func TestGetTemplate(t *testing.T) {
	tests := []struct {
		name      string
		template  string
		wantErr   bool
		errTarget error
	}{
		{
			name:     "minimal template",
			template: "minimal",
			wantErr:  false,
		},
		{
			name:     "spa template",
			template: "spa",
			wantErr:  false,
		},
		{
			name:     "desktop template",
			template: "desktop",
			wantErr:  false,
		},
		{
			name:      "invalid template",
			template:  "invalid",
			wantErr:   true,
			errTarget: ErrInvalidTemplate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs, err := GetTemplate(tt.template)
			if tt.wantErr {
				if err == nil {
					t.Errorf("GetTemplate(%q) expected error, got nil", tt.template)
					return
				}
				if tt.errTarget != nil && !errors.Is(err, tt.errTarget) {
					t.Errorf("GetTemplate(%q) error = %v, want error wrapping %v", tt.template, err, tt.errTarget)
					return
				}
				return
			}
			if err != nil {
				t.Fatalf("GetTemplate(%q) error = %v", tt.template, err)
			}
			if fs == nil {
				t.Error("GetTemplate() returned nil fs")
			}
		})
	}
}

func TestGetSpaTemplate_Content(t *testing.T) {
	templateFS, err := GetTemplate("spa")
	if err != nil {
		t.Fatalf("GetTemplate('spa') error = %v", err)
	}

	content, err := fs.ReadFile(templateFS, "pages/home.tsx")
	if err != nil {
		t.Fatalf("Failed to read pages/home.tsx: %v", err)
	}

	if !strings.Contains(string(content), "Single Page Application") {
		t.Error("spa pages/home.tsx should contain 'Single Page Application'")
	}

	mainContent, err := fs.ReadFile(templateFS, "main.go.tmpl")
	if err != nil {
		t.Fatalf("Failed to read main.go.tmpl: %v", err)
	}

	if !strings.Contains(string(mainContent), "WithClientOnly()") {
		t.Error("spa main.go.tmpl should contain WithClientOnly()")
	}
}

func TestGetDesktopTemplate_Content(t *testing.T) {
	templateFS, err := GetTemplate("desktop")
	if err != nil {
		t.Fatalf("GetTemplate('desktop') error = %v", err)
	}

	mainContent, err := fs.ReadFile(templateFS, "main.go.tmpl")
	if err != nil {
		t.Fatalf("Failed to read main.go.tmpl: %v", err)
	}

	if !strings.Contains(string(mainContent), "systray.Run") {
		t.Error("desktop main.go.tmpl should contain systray.Run")
	}

	if !strings.Contains(string(mainContent), "webview.New") {
		t.Error("desktop main.go.tmpl should contain webview.New")
	}

	if !strings.Contains(string(mainContent), "//go:embed public/icon.png") {
		t.Error("desktop main.go.tmpl should embed icon.png")
	}

	_, err = fs.ReadFile(templateFS, "public/icon.png")
	if err != nil {
		t.Fatalf("desktop template should include public/icon.png: %v", err)
	}

	_, err = fs.ReadFile(templateFS, "Dockerfile")
	if !os.IsNotExist(err) {
		t.Error("desktop template should NOT include Dockerfile")
	}
}

func TestGetMinimalTemplate_DockerfileExists(t *testing.T) {
	templateFS, err := GetTemplate("minimal")
	if err != nil {
		t.Fatalf("GetTemplate('minimal') error = %v", err)
	}

	_, err = fs.ReadFile(templateFS, "Dockerfile")
	if err != nil {
		t.Fatalf("minimal template should include Dockerfile: %v", err)
	}
}

func TestGetSpaTemplate_DockerfileExists(t *testing.T) {
	templateFS, err := GetTemplate("spa")
	if err != nil {
		t.Fatalf("GetTemplate('spa') error = %v", err)
	}

	_, err = fs.ReadFile(templateFS, "Dockerfile")
	if err != nil {
		t.Fatalf("spa template should include Dockerfile: %v", err)
	}
}
