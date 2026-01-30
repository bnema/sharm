package domain

import "errors"

var (
	ErrNotFound = errors.New("media not found")
	ErrExpired  = errors.New("media has expired")
)
