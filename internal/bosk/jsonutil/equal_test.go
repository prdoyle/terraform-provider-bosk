package jsonutil

import "testing"

func TestEqual(t *testing.T) {
	tests := []struct {
		name  string
		a     string
		b     string
		equal bool
	}{
		{
			name:  "same",
			a:     `{"a":1,"b":2}`,
			b:     `{"a":1,"b":2}`,
			equal: true,
		},
		{
			name:  "whitespace and key order",
			a:     "{ \"a\": 1, \"b\": 2 }",
			b:     `{"b":2,"a":1}`,
			equal: true,
		},
		{
			name:  "large integer precision preserved",
			a:     `{"n":9007199254740993}`,
			b:     `{"n":9007199254740993}`,
			equal: true,
		},
		{
			name:  "different integer lexemes",
			a:     `{"n":1}`,
			b:     `{"n":1.0}`,
			equal: false,
		},
		{
			name:  "different values",
			a:     `{"a":1}`,
			b:     `{"a":2}`,
			equal: false,
		},
		{
			name:  "arrays",
			a:     `[1,2,3]`,
			b:     `[1, 2, 3]`,
			equal: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			equal, err := Equal([]byte(test.a), []byte(test.b))
			if err != nil {
				t.Fatalf("Equal returned error: %v", err)
			}
			if equal != test.equal {
				t.Errorf("Equal(%q, %q) = %v; want %v", test.a, test.b, equal, test.equal)
			}
		})
	}
}

func TestEqualRejectsInvalidJSON(t *testing.T) {
	if _, err := Equal([]byte(`{`), []byte(`{}`)); err == nil {
		t.Error("Equal returned nil error for invalid JSON")
	}
}

func TestEqualRejectsMultipleValues(t *testing.T) {
	if _, err := Equal([]byte(`{} {}`), []byte(`{}`)); err == nil {
		t.Error("Equal returned nil error for multiple JSON values")
	}
}
