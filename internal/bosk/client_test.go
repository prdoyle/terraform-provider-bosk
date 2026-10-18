package bosk_test

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/prdoyle/terraform-provider-bosk/internal/bosk"
	"github.com/prdoyle/terraform-provider-bosk/internal/testserver"
)

func newTestClient(t *testing.T, fake *testserver.Fake) *bosk.Client {
	t.Helper()
	endpoint, err := url.Parse(fake.Endpoint())
	if err != nil {
		t.Fatalf("parsing fake endpoint: %v", err)
	}
	client, err := bosk.NewClient(bosk.Config{
		Endpoint:       endpoint,
		Token:          fake.Token(),
		Username:       fake.Username(),
		Password:       fake.Password(),
		RequestTimeout: 5 * time.Second,
		Retry: bosk.RetryPolicy{
			MaxAttempts: 3,
			BaseDelay:   time.Millisecond,
			MaxDelay:    5 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}
	return client
}

func TestClientGetFreshAndETag(t *testing.T) {
	fake := testserver.NewFake(t)
	client := newTestClient(t, fake)
	ctx := context.Background()

	response, err := client.Get(ctx, "/targets/world", bosk.GetOptions{Fresh: true})
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got, want := string(response.Body), `{"id":"world","name":"World"}`; got != want {
		t.Errorf("Get body = %q; want %q", got, want)
	}
	if response.ETag != "" {
		t.Errorf("Get ETag = %q; want empty for a node without a revision", response.ETag)
	}

	response, err = client.Get(ctx, "/versioned/v1", bosk.GetOptions{Fresh: true})
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got, want := response.ETag, `"rev1"`; got != want {
		t.Errorf("Get ETag = %q; want %q", got, want)
	}
}

func TestFreshReadCommitsQueuedWrite(t *testing.T) {
	fake := testserver.NewFake(t)
	client := newTestClient(t, fake)
	ctx := context.Background()

	body := []byte(`{"id":"new","name":"New"}`)
	if err := client.Put(ctx, "/targets/new", body, bosk.PutOptions{}); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	if _, err := client.Get(ctx, "/targets/new", bosk.GetOptions{}); !bosk.IsNotFound(err) {
		t.Errorf("Get without fresh read returned error %v; want NotFound", err)
	}

	response, err := client.Get(ctx, "/targets/new", bosk.GetOptions{Fresh: true})
	if err != nil {
		t.Fatalf("Get with fresh read returned error: %v", err)
	}
	if got := string(response.Body); got != string(body) {
		t.Errorf("Get with fresh read = %q; want %q", got, body)
	}
}

func TestClientPathEncodingPreserved(t *testing.T) {
	fake := testserver.NewFake(t)
	client := newTestClient(t, fake)

	response, err := client.Get(context.Background(), "/targets/a%2Fb", bosk.GetOptions{Fresh: true})
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got, want := string(response.Body), `{"id":"a/b","name":"Slashy"}`; got != want {
		t.Errorf("Get body = %q; want %q", got, want)
	}
}

func TestClientAuth(t *testing.T) {
	fake := testserver.NewFake(t).WithBearer("secret-token")
	client := newTestClient(t, fake)

	if _, err := client.Get(context.Background(), "/targets/world", bosk.GetOptions{Fresh: true}); err != nil {
		t.Fatalf("authenticated Get returned error: %v", err)
	}

	endpoint, _ := url.Parse(fake.Endpoint())
	unauthenticated, err := bosk.NewClient(bosk.Config{
		Endpoint:       endpoint,
		RequestTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("creating unauthenticated client: %v", err)
	}
	_, err = unauthenticated.Get(context.Background(), "/targets/world", bosk.GetOptions{Fresh: true})
	var apiErr *bosk.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 401 {
		t.Errorf("unauthenticated Get error = %v; want a 401 APIError", err)
	}
}

func TestClientRetriesRetryAfter(t *testing.T) {
	fake := testserver.NewFake(t)
	client := newTestClient(t, fake)

	fake.FailNext("GET", "/bosk/state/targets/world", 503, "busy", map[string]string{"Retry-After": "0"})

	response, err := client.Get(context.Background(), "/targets/world", bosk.GetOptions{Fresh: true})
	if err != nil {
		t.Fatalf("Get returned error after retry: %v", err)
	}
	if response.StatusCode != 200 {
		t.Errorf("Get status = %d; want 200", response.StatusCode)
	}
	if fake.Requests() < 2 {
		t.Errorf("requests = %d; want at least 2", fake.Requests())
	}
}

func TestClientAPIErrorIncludesBody(t *testing.T) {
	fake := testserver.NewFake(t)
	client := newTestClient(t, fake)
	fake.FailAlways("GET", "/bosk/state/targets/world", 400, "bad value", nil)

	_, err := client.Get(context.Background(), "/targets/world", bosk.GetOptions{Fresh: true})
	var apiErr *bosk.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Get error = %v; want an APIError", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("status = %d; want 400", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Body, "bad value") {
		t.Errorf("body = %q; want it to contain %q", apiErr.Body, "bad value")
	}
}

func TestClientConditionalPut(t *testing.T) {
	fake := testserver.NewFake(t)
	client := newTestClient(t, fake)
	ctx := context.Background()

	response, err := client.Get(ctx, "/versioned/v1", bosk.GetOptions{Fresh: true})
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	initialETag := response.ETag

	stale := []byte(`{"id":"v1","revision":"stale","value":"stale"}`)
	if err := client.Put(ctx, "/versioned/v1", stale, bosk.PutOptions{IfMatch: `"stale"`}); err != nil {
		t.Fatalf("stale conditional Put returned error: %v", err)
	}
	response, err = client.Get(ctx, "/versioned/v1", bosk.GetOptions{Fresh: true})
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got := response.ETag; got != initialETag {
		t.Errorf("ETag after stale Put = %q; want %q", got, initialETag)
	}

	updated := []byte(`{"id":"v1","revision":"rev2","value":"two"}`)
	if err := client.Put(ctx, "/versioned/v1", updated, bosk.PutOptions{IfMatch: initialETag}); err != nil {
		t.Fatalf("conditional Put returned error: %v", err)
	}
	response, err = client.Get(ctx, "/versioned/v1", bosk.GetOptions{Fresh: true})
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got, want := response.ETag, `"rev2"`; got != want {
		t.Errorf("ETag after Put = %q; want %q", got, want)
	}
}
