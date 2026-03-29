package osfs

import (
	"os"

	"github.com/bnema/sharm/internal/port"
)

var _ port.FileSystem = (*FS)(nil)

// FS implements port.FileSystem using the real OS filesystem.
type FS struct{}

func New() *FS { return &FS{} }

func (FS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (FS) Rename(oldpath, newpath string) error         { return os.Rename(oldpath, newpath) }
func (FS) Remove(path string) error                     { return os.Remove(path) }
func (FS) RemoveAll(path string) error                  { return os.RemoveAll(path) }
func (FS) Stat(path string) (os.FileInfo, error)        { return os.Stat(path) }
func (FS) Chmod(path string, mode os.FileMode) error    { return os.Chmod(path, mode) }
func (FS) Open(path string) (*os.File, error)           { return os.Open(path) }
func (FS) Create(path string) (*os.File, error)         { return os.Create(path) }
func (FS) OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flag, perm)
}
func (FS) CreateTemp(dir, pattern string) (*os.File, error) { return os.CreateTemp(dir, pattern) }
func (FS) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}
