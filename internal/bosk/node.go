package bosk

import (
	"context"
	"fmt"

	"github.com/prdoyle/terraform-provider-bosk/internal/bosk/jsonutil"
)

// Node is the observable state of a bosk node.
type Node struct {
	Path  string
	Value []byte
	ETag  string
}

// NodeStore implements the provider's CRUD semantics over a Client.
// It is deliberately independent of Terraform.
type NodeStore struct {
	client *Client
}

func NewNodeStore(client *Client) *NodeStore {
	return &NodeStore{client: client}
}

// AlreadyExistsError indicates that Create found an existing node.
type AlreadyExistsError struct {
	Path string
	Node *Node
}

func (e *AlreadyExistsError) Error() string {
	return fmt.Sprintf("bosk node already exists at %s", e.Path)
}

// ConflictError indicates that a write was not observable as intended, usually
// because another writer changed the node concurrently.
type ConflictError struct {
	Path     string
	Reason   string
	Desired  []byte
	Observed *Node
}

func (e *ConflictError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("bosk node %s: %s", e.Path, e.Reason)
	}
	return fmt.Sprintf("bosk node %s changed concurrently", e.Path)
}

// CreateFailedError indicates that a create was accepted but the node is not
// present afterwards, usually because the parent path does not exist.
type CreateFailedError struct {
	Path string
	Err  error
}

func (e *CreateFailedError) Error() string {
	return fmt.Sprintf("bosk accepted the create for %s but the node is not present", e.Path)
}

func (e *CreateFailedError) Unwrap() error {
	return e.Err
}

// Read returns the current value of path.
func (s *NodeStore) Read(ctx context.Context, path string) (*Node, error) {
	response, err := s.client.Get(ctx, path, GetOptions{Fresh: true})
	if err != nil {
		return nil, err
	}
	return &Node{
		Path:  path,
		Value: response.Body,
		ETag:  response.ETag,
	}, nil
}

// Create creates the node at path, failing if it already exists.
func (s *NodeStore) Create(ctx context.Context, path string, value []byte) (*Node, error) {
	existing, err := s.Read(ctx, path)
	switch {
	case err == nil:
		return nil, &AlreadyExistsError{Path: path, Node: existing}
	case !IsNotFound(err):
		return nil, err
	}

	err = s.client.Put(ctx, path, value, PutOptions{IfNoneMatchStar: true})
	if err != nil {
		if IsPreconditionFailed(err) {
			return nil, &AlreadyExistsError{Path: path}
		}
		return nil, err
	}

	created, err := s.Read(ctx, path)
	if err != nil {
		if IsNotFound(err) {
			return nil, &CreateFailedError{Path: path, Err: err}
		}
		return nil, err
	}

	equal, err := jsonutil.Equal(created.Value, value)
	if err != nil {
		return nil, fmt.Errorf("comparing created node at %s: %w", path, err)
	}
	if !equal {
		return nil, &ConflictError{
			Path:     path,
			Reason:   "the node was created concurrently with a different value",
			Desired:  value,
			Observed: created,
		}
	}

	return created, nil
}

// Update replaces the node at path. If etag is non-empty, the write is
// conditional on the node still having that ETag.
func (s *NodeStore) Update(ctx context.Context, path string, value []byte, etag string) (*Node, error) {
	err := s.client.Put(ctx, path, value, PutOptions{IfMatch: etag})
	if err != nil {
		if IsPreconditionFailed(err) {
			return nil, &ConflictError{
				Path:    path,
				Reason:  "the node changed after Terraform planned this update",
				Desired: value,
			}
		}
		return nil, err
	}

	observed, err := s.Read(ctx, path)
	if err != nil {
		if IsNotFound(err) {
			return nil, &ConflictError{
				Path:    path,
				Reason:  "the node no longer exists",
				Desired: value,
			}
		}
		return nil, err
	}

	equal, err := jsonutil.Equal(observed.Value, value)
	if err != nil {
		return nil, fmt.Errorf("comparing updated node at %s: %w", path, err)
	}
	if !equal {
		return nil, &ConflictError{
			Path:     path,
			Reason:   "the node changed concurrently; run terraform plan again",
			Desired:  value,
			Observed: observed,
		}
	}

	return observed, nil
}

// Delete deletes the node at path. If etag is non-empty, the delete is
// conditional on the node still having that ETag.
func (s *NodeStore) Delete(ctx context.Context, path string, etag string) error {
	err := s.client.Delete(ctx, path, DeleteOptions{IfMatch: etag})
	if err != nil {
		if IsNotFound(err) {
			return nil
		}
		if IsPreconditionFailed(err) {
			return &ConflictError{
				Path:   path,
				Reason: "the node changed after Terraform planned this delete",
			}
		}
		return err
	}

	observed, err := s.Read(ctx, path)
	if err != nil {
		if IsNotFound(err) {
			return nil
		}
		return err
	}

	return &ConflictError{
		Path:     path,
		Reason:   "the node still exists after the delete, or was re-created",
		Observed: observed,
	}
}
