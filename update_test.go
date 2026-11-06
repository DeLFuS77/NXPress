package main

import (
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"1.0.1", "1.0.0", true}, {"1.1.0", "1.0.9", true}, {"2.0.0", "10.0.0", false}, {"1.0.0", "1.0.0", false}, {"1.0.0", "dev-abc", false},
		// a beta comes after the version before it and before its own version
		{"1.8.0", "1.8.0-beta.1", true}, {"1.8.0-beta.1", "1.8.0", false}, {"1.8.0-beta.1", "1.7.2", true}, {"1.7.2", "1.8.0-beta.1", false},
		{"1.8.0-beta.10", "1.8.0-beta.9", true}, {"1.8.0-beta.2", "1.8.0-beta.2", false},
		{"1.8.0-beta.0", "1.7.0", false}, {"1.8.0-beta.x", "1.7.0", false}, {"1.8.0-rc.1", "1.7.0", false}} {
		if got := newer(c.a, c.b); got != c.want {
			t.Fatalf("newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}
