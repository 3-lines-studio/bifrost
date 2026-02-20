package fs

import (
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
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

func (fs *OSFileSystem) CopyFile(src, dst string) error {
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

func (fs *OSFileSystem) WalkDir(root string, fn iofs.WalkDirFunc) error {
	return filepath.WalkDir(root, fn)
}
