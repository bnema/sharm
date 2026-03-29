package port

import "os"

// FileSystem abstracts file operations for use by services.
type FileSystem interface {
	MkdirAll(path string, perm os.FileMode) error
	Rename(oldpath, newpath string) error
	Remove(path string) error
	RemoveAll(path string) error
	Stat(path string) (os.FileInfo, error)
	Chmod(path string, mode os.FileMode) error
	Open(path string) (*os.File, error)
	Create(path string) (*os.File, error)
	OpenFile(path string, flag int, perm os.FileMode) (*os.File, error)
	CreateTemp(dir, pattern string) (*os.File, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
}
