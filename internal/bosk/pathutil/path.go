package pathutil

import (
	"fmt"
	"net/url"
	"strings"
)

// Validate checks that p is a path suitable for the bosk maintenance API.
// The root path "/" is allowed.
func Validate(p string) error {
	return validate(p, true)
}

// ValidateResourcePath is like Validate but rejects the root path, which is
// not a meaningful target for a managed resource.
func ValidateResourcePath(p string) error {
	return validate(p, false)
}

func validate(p string, allowRoot bool) error {
	if p == "" {
		return fmt.Errorf("path must not be empty")
	}
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("path must begin with \"/\": %q", p)
	}
	if p == "/" {
		if allowRoot {
			return nil
		}
		return fmt.Errorf("the root path is not allowed here")
	}
	if strings.HasSuffix(p, "/") {
		return fmt.Errorf("path must not end with \"/\": %q", p)
	}

	u, err := url.Parse(p)
	if err != nil {
		return fmt.Errorf("invalid path %q: %w", p, err)
	}
	if u.Scheme != "" || u.Host != "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("path must not contain a scheme, host, query, or fragment: %q", p)
	}
	if u.EscapedPath() != p {
		return fmt.Errorf("path must be percent-encoded as a URL path: %q", p)
	}

	for _, segment := range strings.Split(strings.TrimPrefix(p, "/"), "/") {
		if segment == "" {
			return fmt.Errorf("path must not contain an empty segment: %q", p)
		}
		decoded, err := url.PathUnescape(segment)
		if err != nil {
			return fmt.Errorf("invalid percent-encoding in path segment %q: %w", segment, err)
		}
		switch decoded {
		case ".":
			return fmt.Errorf("path must not contain a \".\" segment: %q", p)
		case "..":
			return fmt.Errorf("path must not contain a \"..\" segment: %q", p)
		}
		if strings.HasPrefix(decoded, "{") && strings.HasSuffix(decoded, "}") {
			return fmt.Errorf("parameterized path segments are not supported: %q", segment)
		}
	}

	return nil
}
