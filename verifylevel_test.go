package main

import "testing"

// --verify-level takes full, signature or both, and nothing else.
func TestVerifyLevelOption(t *testing.T) {
	for _, level := range []string{"full", "signature", "both"} {
		o, err := parseOptions([]string{"-V", "--verify-level", level, "game.nsp"})
		if err != nil || o.verifyLevel != level {
			t.Errorf("%s: %q %v", level, o.verifyLevel, err)
		}
	}
	if _, err := parseOptions([]string{"-V", "--verify-level", "quick", "game.nsp"}); err == nil {
		t.Error("an unknown level is taken")
	}
}
