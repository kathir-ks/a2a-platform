// internal/repository/memory/errors.go
package memory

import "errors"

// ErrNotFound indicates that the requested resource was not found in memory.
var ErrNotFound = errors.New("resource not found in memory")

// ErrAlreadyExists indicates that a resource with the given ID/key already exists.
var ErrAlreadyExists = errors.New("resource already exists")