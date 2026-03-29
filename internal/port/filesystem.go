package port

import "os"

// FileSystem abstracts file operations for use by services.
type FileSystem interface {
	MkdirAll(path string, perm os.FileMode) error
	Rename(oldpath, newpath string) error
	Remove(path string) error
	Stat(path string) (os.FileInfo, error)
	Open(path string) (*os.File, error)
	Create(path string) (*os.File, error)
}
