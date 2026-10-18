package jsonutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Equal reports whether a and b are semantically equal JSON values.
// Object key order and insignificant whitespace are ignored, and numbers are
// compared using their original lexical form rather than being converted to
// float64.
func Equal(a, b []byte) (bool, error) {
	normalizedA, err := normalize(a)
	if err != nil {
		return false, fmt.Errorf("first value: %w", err)
	}
	normalizedB, err := normalize(b)
	if err != nil {
		return false, fmt.Errorf("second value: %w", err)
	}
	return bytes.Equal(normalizedA, normalizedB), nil
}

// Valid reports whether data is a single well-formed JSON value.
func Valid(data []byte) bool {
	return json.Valid(data)
}

func normalize(data []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}

	return json.Marshal(value)
}
