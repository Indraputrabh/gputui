package gpu

import (
	"errors"
	"fmt"
)

// ErrNotImplemented marks scaffold paths that are wired but not yet implemented.
var ErrNotImplemented = errors.New("gpu collector backend not implemented")

// NotImplementedError includes backend context for scaffold failures.
type NotImplementedError struct {
	Source Source
	Detail string
}

func (e *NotImplementedError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("%s: %s", e.Source, ErrNotImplemented)
	}
	return fmt.Sprintf("%s: %s (%s)", e.Source, ErrNotImplemented, e.Detail)
}

func (e *NotImplementedError) Unwrap() error {
	return ErrNotImplemented
}
