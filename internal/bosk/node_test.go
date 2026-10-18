package bosk_test

import (
	"context"
	"errors"
	"testing"

	"github.com/prdoyle/terraform-provider-bosk/internal/bosk"
	"github.com/prdoyle/terraform-provider-bosk/internal/testserver"
)

func newTestStore(t *testing.T, fake *testserver.Fake) *bosk.NodeStore {
	t.Helper()
	return bosk.NewNodeStore(newTestClient(t, fake))
}

func TestNodeStoreLifecycle(t *testing.T) {
	fake := testserver.NewFake(t)
	store := newTestStore(t, fake)
	ctx := context.Background()

	created, err := store.Create(ctx, "/targets/new", []byte(`{"id":"new","name":"New"}`))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.ETag != "" {
		t.Errorf("created ETag = %q; want empty", created.ETag)
	}

	node, err := store.Read(ctx, "/targets/new")
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if got, want := string(node.Value), `{"id":"new","name":"New"}`; got != want {
		t.Errorf("Read value = %q; want %q", got, want)
	}

	updated, err := store.Update(ctx, "/targets/new", []byte(`{"id":"new","name":"Updated"}`), "")
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if got, want := string(updated.Value), `{"id":"new","name":"Updated"}`; got != want {
		t.Errorf("Update value = %q; want %q", got, want)
	}

	if err := store.Delete(ctx, "/targets/new", ""); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := store.Read(ctx, "/targets/new"); !bosk.IsNotFound(err) {
		t.Errorf("Read after Delete returned error %v; want NotFound", err)
	}
}

func TestNodeStoreCreateAlreadyExists(t *testing.T) {
	fake := testserver.NewFake(t)
	store := newTestStore(t, fake)

	_, err := store.Create(context.Background(), "/targets/world", []byte(`{"id":"world","name":"Other"}`))
	var alreadyExists *bosk.AlreadyExistsError
	if !errors.As(err, &alreadyExists) {
		t.Fatalf("Create error = %v; want AlreadyExistsError", err)
	}
}

func TestNodeStoreCreateSilentlyDropped(t *testing.T) {
	fake := testserver.NewFake(t)
	fake.DropWrites("/targets/missing")
	store := newTestStore(t, fake)

	_, err := store.Create(context.Background(), "/targets/missing", []byte(`{"id":"missing"}`))
	var createFailed *bosk.CreateFailedError
	if !errors.As(err, &createFailed) {
		t.Fatalf("Create error = %v; want CreateFailedError", err)
	}
}

func TestNodeStoreVersionedUpdate(t *testing.T) {
	fake := testserver.NewFake(t)
	store := newTestStore(t, fake)
	ctx := context.Background()

	node, err := store.Read(ctx, "/versioned/v1")
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if got, want := node.ETag, `"rev1"`; got != want {
		t.Fatalf("Read ETag = %q; want %q", got, want)
	}

	updated, err := store.Update(ctx, "/versioned/v1", []byte(`{"id":"v1","revision":"rev2","value":"two"}`), node.ETag)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if got, want := updated.ETag, `"rev2"`; got != want {
		t.Errorf("Update ETag = %q; want %q", got, want)
	}
}

func TestNodeStoreUpdateConflict(t *testing.T) {
	fake := testserver.NewFake(t)
	store := newTestStore(t, fake)
	ctx := context.Background()

	node, err := store.Read(ctx, "/versioned/v1")
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}

	fake.SetNode("/versioned/v1", `{"id":"v1","revision":"rev2","value":"other"}`)

	_, err = store.Update(ctx, "/versioned/v1", []byte(`{"id":"v1","revision":"rev3","value":"mine"}`), node.ETag)
	var conflict *bosk.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("Update error = %v; want ConflictError", err)
	}
}

func TestNodeStoreDeleteConflict(t *testing.T) {
	fake := testserver.NewFake(t)
	store := newTestStore(t, fake)
	ctx := context.Background()

	node, err := store.Read(ctx, "/versioned/v1")
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}

	fake.SetNode("/versioned/v1", `{"id":"v1","revision":"rev2","value":"other"}`)

	err = store.Delete(ctx, "/versioned/v1", node.ETag)
	var conflict *bosk.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("Delete error = %v; want ConflictError", err)
	}
	if _, ok := fake.GetNode("/versioned/v1"); !ok {
		t.Error("node was deleted despite the stale ETag")
	}
}

func TestNodeStoreDeleteMissingIsSuccess(t *testing.T) {
	fake := testserver.NewFake(t)
	store := newTestStore(t, fake)

	if err := store.Delete(context.Background(), "/targets/missing", ""); err != nil {
		t.Fatalf("Delete of a missing node returned error: %v", err)
	}
}
