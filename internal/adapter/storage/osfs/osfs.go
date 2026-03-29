package osfs

import "os"

// FS implements port.FileSystem using the real OS filesystem.
type FS struct{}

func New() *FS { return &FS{} }

func (FS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (FS) Rename(oldpath, newpath string) error         { return os.Rename(oldpath, newpath) }
func (FS) Remove(path string) error                     { return os.Remove(path) }
func (FS) Stat(path string) (os.FileInfo, error)        { return os.Stat(path) }
func (FS) Open(path string) (*os.File, error)           { return os.Open(path) }
func (FS) Create(path string) (*os.File, error)         { return os.Create(path) }
