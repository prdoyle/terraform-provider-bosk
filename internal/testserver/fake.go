package testserver

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// Server is the part of a test server that provider configuration needs.
// A real bosk instance can implement this without implementing the fake's
// control methods.
type Server interface {
	Endpoint() string
	Token() string
	Username() string
	Password() string
	Close()
}

// AuthMode selects the authentication the fake server requires.
type AuthMode string

const (
	AuthNone   AuthMode = "none"
	AuthBearer AuthMode = "bearer"
	AuthBasic  AuthMode = "basic"
)

// Node is the fake server's representation of a bosk node.
type Node struct {
	Value []byte
	ETag  string
}

// Fake is an in-memory implementation of the bosk maintenance API.
// It is strict about the parts of the contract the provider relies on.
type Fake struct {
	Server *httptest.Server

	basePath string

	mu              sync.Mutex
	committed       map[string]*Node
	pending         []func()
	buffered        bool
	auth            AuthMode
	token           string
	username        string
	password        string
	failures        []failure
	requireFresh    bool
	dropWrites      map[string]bool
	requiredHeaders map[string]string
	requests        int
}

var _ Server = (*Fake)(nil)

type failure struct {
	method    string
	path      string
	status    int
	body      string
	headers   map[string]string
	remaining int
	delay     time.Duration
}

// NewFake starts a fake bosk server with a small, known state tree.
func NewFake(t *testing.T) *Fake {
	t.Helper()

	f := &Fake{
		basePath:        "/bosk/state",
		committed:       map[string]*Node{},
		buffered:        true,
		auth:            AuthNone,
		dropWrites:      map[string]bool{},
		requiredHeaders: map[string]string{},
	}
	f.setNode("/targets/world", []byte(`{"id":"world","name":"World"}`))
	f.setNode("/targets/a%2Fb", []byte(`{"id":"a/b","name":"Slashy"}`))
	f.setNode("/versioned/v1", []byte(`{"id":"v1","revision":"rev1","value":"one"}`))
	f.setNode("/versioned/v2", []byte(`{"id":"v2","revision":"rev 1%\"é","value":"two"}`))
	f.setNode("/config", []byte(`{"hostname":"old"}`))
	f.setNode("/targets", []byte(`[{"id":"world","name":"World"}]`))

	f.Server = httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(f.Server.Close)
	return f
}

// Endpoint returns the maintenance base URL, including the base path.
func (f *Fake) Endpoint() string {
	return f.Server.URL + f.basePath
}

func (f *Fake) Token() string    { return f.token }
func (f *Fake) Username() string { return f.username }
func (f *Fake) Password() string { return f.password }

// Close stops the fake server.
func (f *Fake) Close() {
	f.Server.Close()
}

// WithBearer configures the fake to require a bearer token.
func (f *Fake) WithBearer(token string) *Fake {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth = AuthBearer
	f.token = token
	return f
}

// WithBasic configures the fake to require HTTP Basic authentication.
func (f *Fake) WithBasic(username, password string) *Fake {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth = AuthBasic
	f.username = username
	f.password = password
	return f
}

// RequireFreshReads makes GET requests without Cache-Control: no-cache fail.
// This is a strict meta-test for the provider's use of fresh reads.
func (f *Fake) RequireFreshReads() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requireFresh = true
}

// RequireHeader makes every request fail unless the named header has value.
func (f *Fake) RequireHeader(name, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requiredHeaders[name] = value
}

// DropWrites makes PUT requests to path return 202 without applying them.
// It simulates a write that is accepted but silently ignored.
func (f *Fake) DropWrites(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dropWrites[f.basePath+path] = true
}

// SetNode replaces a node directly, as an out-of-band writer would.
func (f *Fake) SetNode(path string, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setNodeLocked(path, []byte(value))
}

// DeleteNode deletes a node directly, as an out-of-band writer would.
func (f *Fake) DeleteNode(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.committed, f.basePath+path)
}

// GetNode returns a node directly without going through HTTP.
func (f *Fake) GetNode(path string) (*Node, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	node, ok := f.committed[f.basePath+path]
	if !ok {
		return nil, false
	}
	copyNode := *node
	copyNode.Value = append([]byte(nil), node.Value...)
	return &copyNode, true
}

// Requests returns the number of HTTP requests served.
func (f *Fake) Requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

// FailNext makes the next matching request return status with body.
func (f *Fake) FailNext(method, path string, status int, body string, headers map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures = append(f.failures, failure{
		method:    method,
		path:      path,
		status:    status,
		body:      body,
		headers:   headers,
		remaining: 1,
	})
}

// FailAlways makes every matching request return status with body until cleared.
func (f *Fake) FailAlways(method, path string, status int, body string, headers map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures = append(f.failures, failure{
		method:    method,
		path:      path,
		status:    status,
		body:      body,
		headers:   headers,
		remaining: -1,
	})
}

// DelayNext adds a delay before the next matching request.
func (f *Fake) DelayNext(method, path string, delay time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures = append(f.failures, failure{
		method:    method,
		path:      path,
		remaining: 1,
		delay:     delay,
	})
}

func (f *Fake) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests++
	f.mu.Unlock()

	if !f.authorized(r) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !f.hasRequiredHeaders(r) {
		http.Error(w, "missing required header", http.StatusUnauthorized)
		return
	}

	path := r.URL.EscapedPath()
	if failure, ok := f.takeFailure(r.Method, path); ok {
		if failure.delay > 0 {
			select {
			case <-time.After(failure.delay):
			case <-r.Context().Done():
				return
			}
		}
		if failure.status == 0 {
			return
		}
		for name, value := range failure.headers {
			w.Header().Set(name, value)
		}
		w.WriteHeader(failure.status)
		_, _ = io.WriteString(w, failure.body)
		return
	}

	switch r.Method {
	case http.MethodGet:
		f.get(w, r, path)
	case http.MethodPut:
		f.put(w, r, path)
	case http.MethodDelete:
		f.delete(w, r, path)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (f *Fake) get(w http.ResponseWriter, r *http.Request, path string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.requireFresh && !strings.EqualFold(r.Header.Get("Cache-Control"), "no-cache") {
		http.Error(w, "fake server requires Cache-Control: no-cache for GET", http.StatusBadRequest)
		return
	}
	if strings.EqualFold(r.Header.Get("Cache-Control"), "no-cache") {
		f.applyPendingLocked()
	}

	node, ok := f.committed[path]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if node.ETag != "" {
		w.Header().Set("ETag", node.ETag)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(node.Value)
}

func (f *Fake) put(w http.ResponseWriter, r *http.Request, path string) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}
	if !json.Valid(body) {
		http.Error(w, "body is not valid JSON", http.StatusBadRequest)
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	node, exists := f.committed[path]
	ifNoneMatch := r.Header.Get("If-None-Match")
	if ifNoneMatch != "" && ifNoneMatch != "*" {
		http.Error(w, "If-None-Match must be *", http.StatusBadRequest)
		return
	}
	ifMatch := r.Header.Get("If-Match")
	if ifMatch != "" && ifNoneMatch != "" {
		http.Error(w, "cannot supply both If-Match and If-None-Match", http.StatusBadRequest)
		return
	}

	if ifNoneMatch == "*" && exists {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if ifMatch != "" {
		if !exists {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if node.ETag == "" {
			http.Error(w, "preconditions not supported for node without a revision", http.StatusBadRequest)
			return
		}
		if node.ETag != ifMatch {
			w.WriteHeader(http.StatusAccepted)
			return
		}
	}

	if f.dropWrites[path] {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	newNode := &Node{Value: append([]byte(nil), body...), ETag: etagFor(body)}
	f.enqueueLocked(func() {
		f.committed[path] = newNode
	})
	w.WriteHeader(http.StatusAccepted)
}

func (f *Fake) delete(w http.ResponseWriter, r *http.Request, path string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	node, exists := f.committed[path]
	ifMatch := r.Header.Get("If-Match")
	if ifMatch != "" {
		if !exists {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if node.ETag == "" {
			http.Error(w, "preconditions not supported for node without a revision", http.StatusBadRequest)
			return
		}
		if node.ETag != ifMatch {
			w.WriteHeader(http.StatusAccepted)
			return
		}
	}

	f.enqueueLocked(func() {
		delete(f.committed, path)
	})
	w.WriteHeader(http.StatusAccepted)
}

func (f *Fake) authorized(r *http.Request) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch f.auth {
	case AuthNone:
		return true
	case AuthBearer:
		return r.Header.Get("Authorization") == "Bearer "+f.token
	case AuthBasic:
		username, password, ok := r.BasicAuth()
		return ok && username == f.username && password == f.password
	default:
		return false
	}
}

func (f *Fake) hasRequiredHeaders(r *http.Request) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for name, value := range f.requiredHeaders {
		if r.Header.Get(name) != value {
			return false
		}
	}
	return true
}

func (f *Fake) takeFailure(method, path string) (failure, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.failures {
		candidate := &f.failures[i]
		if candidate.method != method || candidate.path != path {
			continue
		}
		result := *candidate
		if candidate.remaining > 0 {
			candidate.remaining--
			if candidate.remaining == 0 {
				f.failures = append(f.failures[:i], f.failures[i+1:]...)
			}
		}
		return result, true
	}
	return failure{}, false
}

func (f *Fake) enqueueLocked(action func()) {
	if f.buffered {
		f.pending = append(f.pending, action)
		return
	}
	action()
}

func (f *Fake) applyPendingLocked() {
	for _, action := range f.pending {
		action()
	}
	f.pending = nil
}

func (f *Fake) setNode(path string, value []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setNodeLocked(path, value)
}

func (f *Fake) setNodeLocked(path string, value []byte) {
	fullPath := f.basePath + path
	f.committed[fullPath] = &Node{
		Value: append([]byte(nil), value...),
		ETag:  etagFor(value),
	}
}

func etagFor(value []byte) string {
	var object map[string]any
	if err := json.Unmarshal(value, &object); err != nil {
		return ""
	}
	revision, ok := object["revision"].(string)
	if !ok {
		return ""
	}
	return `"` + strings.ReplaceAll(url.QueryEscape(revision), "+", "%20") + `"`
}
