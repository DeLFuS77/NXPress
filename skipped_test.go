package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// A file left as it is is told as skipped, with why: not as done, and not as failed.
func TestSkippedIsToldApart(t *testing.T) {
	err := skipped("nothing in it can be compressed, it is left as it is: it holds no game data (control)")
	if !isSkipped(err) || isSkipped(errors.New("broken")) || isSkipped(nil) {
		t.Fatal("isSkipped")
	}
	var text bytes.Buffer
	(&reporter{out: &text}).done("game.nsp", "", "", err)
	if !strings.HasPrefix(text.String(), "SKIPPED game.nsp (nothing in it can be compressed") {
		t.Fatalf("text: %q", text.String())
	}
	var machine bytes.Buffer
	(&reporter{out: &machine, machine: true}).done("game.nsp", "", "", err)
	var result map[string]any
	if json.Unmarshal(machine.Bytes(), &result) != nil || result["result"] != "skipped" || result["error"] != nil || !strings.Contains(result["note"].(string), "control") {
		t.Fatalf("machine: %s", machine.String())
	}
}
