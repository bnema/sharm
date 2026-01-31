package domain

import "errors"

var (
	ErrNotFound = errors.New("resource not found")
	ErrExpired  = errors.New("media has expired")
)
