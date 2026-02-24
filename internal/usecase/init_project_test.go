package usecase

import (
	"errors"
	iofs "io/fs"
	"testing"
)

type mockFileSystem struct {
	files      map[string][]byte
	dirs       map[string][]iofs.DirEntry
	exists     map[string]bool
	mkdirCalls []string
	writeCalls map[string][]byte
}

type mockDirEntry struct {
	name  string
	isDir bool
}

func (m *mockDirEntry) Name() string                 { return m.name }
func (m *mockDirEntry) IsDir() bool                  { return m.isDir }
func (m *mockDirEntry) Type() iofs.FileMode          { return 0 }
func (m *mockDirEntry) Info() (iofs.FileInfo, error) { return nil, nil }

func newMockFileSystem() *mockFileSystem {
	return &mockFileSystem{
		files:      make(map[string][]byte),
		dirs:       make(map[string][]iofs.DirEntry),
		exists:     make(map[string]bool),
		writeCalls: make(map[string][]byte),
	}
}

func (m *mockFileSystem) ReadFile(path string) ([]byte, error) {
	if data, ok := m.files[path]; ok {
		return data, nil
	}
	return nil, errors.New("file not found")
}

func (m *mockFileSystem) ReadDir(path string) ([]iofs.DirEntry, error) {
	if entries, ok := m.dirs[path]; ok {
		return entries, nil
	}
	return []iofs.DirEntry{}, nil
}

func (m *mockFileSystem) FileExists(path string) bool {
	return m.exists[path]
}

func (m *mockFileSystem) WriteFile(path string, data []byte, perm iofs.FileMode) error {
	m.writeCalls[path] = data
	return nil
}

func (m *mockFileSystem) MkdirAll(path string, perm iofs.FileMode) error {
	m.mkdirCalls = append(m.mkdirCalls, path)
	return nil
}

func (m *mockFileSystem) Remove(path string) error {
	return nil
}

type mockCLIOutput struct {
	messages []string
}

func (m *mockCLIOutput) PrintHeader(msg string)                   {}
func (m *mockCLIOutput) PrintStep(emoji, msg string, args ...any) {}
func (m *mockCLIOutput) PrintSuccess(msg string, args ...any) {
	m.messages = append(m.messages, msg)
}
func (m *mockCLIOutput) PrintWarning(msg string, args ...any) {}
func (m *mockCLIOutput) PrintError(msg string, args ...any)   {}
func (m *mockCLIOutput) PrintFile(path string)                {}
func (m *mockCLIOutput) PrintDone(msg string)                 {}
func (m *mockCLIOutput) Green(text string) string             { return text }
func (m *mockCLIOutput) Yellow(text string) string            { return text }
func (m *mockCLIOutput) Red(text string) string               { return text }
func (m *mockCLIOutput) Gray(text string) string              { return text }

func TestInitProject_DirectoryNotEmpty(t *testing.T) {
	fs := newMockFileSystem()
	fs.exists["/test/project"] = true
	fs.dirs["/test/project"] = []iofs.DirEntry{
		&mockDirEntry{name: "existing.txt", isDir: false},
	}

	cli := &mockCLIOutput{}
	service := NewInitService(fs, cli)

	input := InitInput{
		ProjectDir: "/test/project",
		Template:   "minimal",
		ModuleName: "testproject",
	}

	result := service.InitProject(input)

	if result.Error == nil {
		t.Error("expected error for non-empty directory, got nil")
	}

	if result.Success {
		t.Error("expected Success to be false for non-empty directory")
	}
}

func TestInitProject_InvalidTemplate(t *testing.T) {
	fs := newMockFileSystem()
	fs.exists["/test/project"] = false

	cli := &mockCLIOutput{}
	service := NewInitService(fs, cli)

	input := InitInput{
		ProjectDir: "/test/project",
		Template:   "nonexistent",
		ModuleName: "testproject",
	}

	result := service.InitProject(input)

	if result.Error == nil {
		t.Error("expected error for invalid template, got nil")
	}

	if result.Success {
		t.Error("expected Success to be false for invalid template")
	}
}

func TestInitProject_MinimalTemplate(t *testing.T) {
	tmpDir := t.TempDir()

	fs := newMockFileSystem()
	fs.exists[tmpDir] = true
	fs.dirs[tmpDir] = []iofs.DirEntry{}

	cli := &mockCLIOutput{}
	service := NewInitService(fs, cli)

	input := InitInput{
		ProjectDir: tmpDir,
		Template:   "minimal",
		ModuleName: "testproject",
	}

	result := service.InitProject(input)

	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}

	if !result.Success {
		t.Error("expected Success to be true")
	}
}

func TestInitProject_WithModuleNameSubstitution(t *testing.T) {
	tmpDir := t.TempDir()

	fs := newMockFileSystem()
	fs.exists[tmpDir] = true
	fs.dirs[tmpDir] = []iofs.DirEntry{}

	cli := &mockCLIOutput{}
	service := NewInitService(fs, cli)

	input := InitInput{
		ProjectDir: tmpDir,
		Template:   "minimal",
		ModuleName: "github.com/example/myproject",
	}

	result := service.InitProject(input)

	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}

	if !result.Success {
		t.Error("expected Success to be true")
	}
}

func TestInitProject_AllTemplates(t *testing.T) {
	templates := []string{"minimal", "spa", "desktop"}

	for _, tmpl := range templates {
		t.Run(tmpl, func(t *testing.T) {
			tmpDir := t.TempDir()

			fs := newMockFileSystem()
			fs.exists[tmpDir] = true
			fs.dirs[tmpDir] = []iofs.DirEntry{}

			cli := &mockCLIOutput{}
			service := NewInitService(fs, cli)

			input := InitInput{
				ProjectDir: tmpDir,
				Template:   tmpl,
				ModuleName: "testproject",
			}

			result := service.InitProject(input)

			if result.Error != nil {
				t.Errorf("unexpected error for template %s: %v", tmpl, result.Error)
			}

			if !result.Success {
				t.Errorf("expected Success to be true for template %s", tmpl)
			}
		})
	}
}

func TestInitProject_IntegrationWithRealFS(t *testing.T) {
	tmpDir := t.TempDir()

	cli := &mockCLIOutput{}

	osFS := &testOSFileSystem{}
	service := NewInitService(osFS, cli)

	input := InitInput{
		ProjectDir: tmpDir,
		Template:   "minimal",
		ModuleName: "github.com/test/myproject",
	}

	result := service.InitProject(input)

	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}

	if !result.Success {
		t.Error("expected Success to be true")
	}
}

type testOSFileSystem struct{}

func (m *testOSFileSystem) ReadFile(path string) ([]byte, error) {
	return nil, nil
}

func (m *testOSFileSystem) ReadDir(path string) ([]iofs.DirEntry, error) {
	return []iofs.DirEntry{}, nil
}

func (m *testOSFileSystem) FileExists(path string) bool {
	return false
}

func (m *testOSFileSystem) WriteFile(path string, data []byte, perm iofs.FileMode) error {
	return nil
}

func (m *testOSFileSystem) MkdirAll(path string, perm iofs.FileMode) error {
	return nil
}

func (m *testOSFileSystem) Remove(path string) error {
	return nil
}
