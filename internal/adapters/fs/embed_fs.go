package fs

import (
	"embed"
	"errors"
	iofs "io/fs"
)

type EmbedFileSystem struct {
	fs embed.FS
}

func NewEmbedFileSystem(fs embed.FS) *EmbedFileSystem {
	return &EmbedFileSystem{fs: fs}
}

func (fs *EmbedFileSystem) ReadFile(path string) ([]byte, error) {
	return fs.fs.ReadFile(path)
}

func (fs *EmbedFileSystem) ReadDir(path string) ([]iofs.DirEntry, error) {
	return fs.fs.ReadDir(path)
}

func (fs *EmbedFileSystem) FileExists(path string) bool {
	_, err := fs.fs.ReadFile(path)
	return err == nil
}

func (fs *EmbedFileSystem) WriteFile(path string, data []byte, perm iofs.FileMode) error {
	return errors.New("embed filesystem is read-only")
}

func (fs *EmbedFileSystem) MkdirAll(path string, perm iofs.FileMode) error {
	return errors.New("embed filesystem is read-only")
}

func (fs *EmbedFileSystem) Remove(path string) error {
	return errors.New("embed filesystem is read-only")
}
