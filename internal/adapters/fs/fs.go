package fs

import (
	iofs "io/fs"
)

type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	ReadDir(path string) ([]iofs.DirEntry, error)
	FileExists(path string) bool
	WriteFile(path string, data []byte, perm iofs.FileMode) error
	MkdirAll(path string, perm iofs.FileMode) error
	Remove(path string) error
}
