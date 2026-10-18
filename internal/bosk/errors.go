package bosk

import (
	"errors"
	"fmt"
)

// NotFoundError indicates that the requested node does not exist.
type NotFoundError struct {
	Path string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("bosk node not found: %s", e.Path)
}

// IsNotFound reports whether err indicates a missing node.
func IsNotFound(err error) bool {
	var target *NotFoundError
	return errors.As(err, &target)
}

// PreconditionError indicates that a conditional request was rejected.
// The current maintenance API treats a mismatched precondition as a successful
// no-op, so this is only returned by servers that implement standard HTTP
// precondition semantics.
type PreconditionError struct {
	Method string
	Path   string
}

func (e *PreconditionError) Error() string {
	return fmt.Sprintf("bosk precondition failed for %s %s", e.Method, e.Path)
}

// IsPreconditionFailed reports whether err indicates a failed precondition.
func IsPreconditionFailed(err error) bool {
	var target *PreconditionError
	return errors.As(err, &target)
}

// APIError is an unexpected HTTP response from the maintenance endpoint.
type APIError struct {
	Method     string
	URL        string
	StatusCode int
	Status     string
	Body       string
}

func (e *APIError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("bosk API error: %s %s returned %d %s", e.Method, e.URL, e.StatusCode, e.Status)
	}
	return fmt.Sprintf("bosk API error: %s %s returned %d %s: %s", e.Method, e.URL, e.StatusCode, e.Status, e.Body)
}
