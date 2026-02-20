package fs

import (
	"embed"
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

func (fs *EmbedFileSystem) Sub(dir string) (iofs.FS, error) {
	return iofs.Sub(fs.fs, dir)
}
