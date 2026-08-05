package fs

import (
	iofs "io/fs"
	"os"
)

type OSFileSystem struct{}

func NewOSFileSystem() *OSFileSystem {
	return &OSFileSystem{}
}

func (fs *OSFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (fs *OSFileSystem) ReadDir(path string) ([]iofs.DirEntry, error) {
	return os.ReadDir(path)
}

func (fs *OSFileSystem) FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (fs *OSFileSystem) WriteFile(path string, data []byte, perm iofs.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (fs *OSFileSystem) MkdirAll(path string, perm iofs.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (fs *OSFileSystem) Remove(path string) error {
	return os.Remove(path)
}
