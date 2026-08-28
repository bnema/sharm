package domain

import "errors"

var (
	ErrNotFound         = errors.New("resource not found")
	ErrExpired          = errors.New("media has expired")
	ErrDiskFull         = errors.New("disk full")
	ErrPermission       = errors.New("permission denied")
	ErrInvalidUpload    = errors.New("invalid upload")
	ErrUploadOwnership  = errors.New("upload does not belong to user")
	ErrUploadExpired    = errors.New("upload session has expired")
	ErrChunkConflict    = errors.New("upload chunk conflicts with existing data")
	ErrUploadIncomplete = errors.New("upload is incomplete")
	ErrQuotaExceeded    = errors.New("upload quota exceeded")
	ErrUnsupportedMedia = errors.New("unsupported media")
	ErrJobConflict      = errors.New("active job has conflicting parameters")
)
