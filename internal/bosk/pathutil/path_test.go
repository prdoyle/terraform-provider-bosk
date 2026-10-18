package pathutil

import "testing"

func TestValidate(t *testing.T) {
	valid := []string{
		"/",
		"/targets",
		"/targets/world",
		"/targets/a%2Fb",
		"/targets/100%25",
		"/targets/a%20b",
		"/targets/%C3%A9",
		"/versioned/rev%201%25%22%C3%A9",
	}
	for _, path := range valid {
		if err := Validate(path); err != nil {
			t.Errorf("Validate(%q) returned error: %v", path, err)
		}
	}
}

func TestValidateResourcePathRejectsRoot(t *testing.T) {
	if err := ValidateResourcePath("/"); err == nil {
		t.Error("ValidateResourcePath(/) returned nil; want an error")
	}
}

func TestValidateRejects(t *testing.T) {
	invalid := []string{
		"",
		"targets",
		"//",
		"/targets/",
		"/targets//world",
		"/targets/./world",
		"/targets/../world",
		"/targets/a b",
		"/targets/100%",
		"/targets?x=1",
		"/targets#fragment",
		"http://example.com/targets",
		"/targets/{id}",
		"/targets/é",
	}
	for _, path := range invalid {
		if err := Validate(path); err == nil {
			t.Errorf("Validate(%q) returned nil; want an error", path)
		}
	}
}
