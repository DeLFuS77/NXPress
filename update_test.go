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

// The window opens the pages of NXPress on GitHub and its Discord server (#86), and no other.
func TestPageOfNXPress(t *testing.T) {
	for url, want := range map[string]bool{
		"https://github.com/DeLFuS77/NXPress":                    true,
		"https://github.com/DeLFuS77/NXPress/releases/tag/1.7.4": true,
		"https://discord.gg/acQVZSEBR":                           true,
		"https://discord.gg/another":                             false,
		"https://github.com/DeLFuS77/NXPress.evil.example":       false,
		"https://github.com/DeLFuS77/NXPressCopy":                false,
		"http://github.com/DeLFuS77/NXPress":                     false,
		"file:///C:/Windows/System32/calc.exe":                   false,
		"https://discord.gg/acQVZSEBR/../../evil.example":        false,
		"": false,
	} {
		if got := pageOfNXPress(url); got != want {
			t.Errorf("pageOfNXPress(%q) = %v", url, got)
		}
	}
}

// The notes of a release show without the table of its downloads (#91).
func TestReleaseNotes(t *testing.T) {
	body := "### Improvements\n\n- One thing\n\n### Downloads\n\n| System | File |\n"
	if got := releaseNotes(body); got != "### Improvements\n\n- One thing" {
		t.Errorf("%q", got)
	}
	if got := releaseNotes("  - Only this  "); got != "- Only this" {
		t.Errorf("%q", got)
	}
}
