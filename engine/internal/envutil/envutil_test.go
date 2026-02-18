package envutil

import "testing"

func TestParseBool(t *testing.T) {
	cases := map[string]bool{
		"1":     true,
		"true":  true,
		"TRUE":  true,
		"yes":   true,
		"on":    true,
		"false": false,
		"0":     false,
		"":      false,
	}
	for input, want := range cases {
		if got := ParseBool(input); got != want {
			t.Fatalf("ParseBool(%q) = %v, want %v", input, got, want)
		}
	}
}
