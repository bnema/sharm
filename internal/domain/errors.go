package domain

import "errors"

var (
	ErrNotFound   = errors.New("resource not found")
	ErrExpired    = errors.New("media has expired")
	ErrDiskFull   = errors.New("disk full")
	ErrPermission = errors.New("permission denied")
)
