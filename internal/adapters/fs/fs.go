package fs

import (
	"io"
	"os"

	"github.com/nord-launcher/launcher/internal/core/ports"
)

type OSFileSystem struct{}

func NewOSFileSystem() ports.FileSystem {
	return &OSFileSystem{}
}

func (fs *OSFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (fs *OSFileSystem) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (fs *OSFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (fs *OSFileSystem) Exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func (fs *OSFileSystem) Remove(path string) error {
	return os.Remove(path)
}

func (fs *OSFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (fs *OSFileSystem) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (fs *OSFileSystem) Open(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (fs *OSFileSystem) Create(path string) (io.WriteCloser, error) {
	return os.Create(path)
}

func (fs *OSFileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
