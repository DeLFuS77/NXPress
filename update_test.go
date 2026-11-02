package main

import (
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"1.0.1", "1.0.0", true}, {"1.1.0", "1.0.9", true}, {"2.0.0", "10.0.0", false}, {"1.0.0", "1.0.0", false}, {"1.0.0", "dev-abc", false}} {
		if got := newer(c.a, c.b); got != c.want {
			t.Fatalf("newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}
