package service

import "testing"

func TestIsDigits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{name: "empty", input: "", expect: false},
		{name: "spaces", input: "   ", expect: false},
		{name: "alpha", input: "12ab34", expect: false},
		{name: "with dash", input: "123-456", expect: false},
		{name: "digits", input: "79927398713", expect: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isDigits(tt.input); got != tt.expect {
				t.Fatalf("isDigits(%q) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}

func TestIsValidLuhn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{name: "empty", input: "", expect: false},
		{name: "valid", input: "79927398713", expect: true},
		{name: "valid second", input: "12345678903", expect: true},
		{name: "invalid", input: "12345678901", expect: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isValidLuhn(tt.input); got != tt.expect {
				t.Fatalf("isValidLuhn(%q) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}
