package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// jsonLines reads every line printed as a JSON object, and fails on one that is not.
func jsonLines(t *testing.T, printed string) []map[string]any {
	t.Helper()
	lines := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSuffix(printed, "\n"), "\n") {
		object := map[string]any{}
		if err := json.Unmarshal([]byte(line), &object); err != nil {
			t.Fatalf("not JSON: %q (%v)", line, err)
		}
		lines = append(lines, object)
	}
	return lines
}

// --machine-readable prints the JSON of nsz: a "type" on every line, the keys nsz gives (and the
// ones NXPress gave before), what -i and --levels tell as "info", and a summary at the end.
func TestMachineReadableAsNsz(t *testing.T) {
	var printed bytes.Buffer
	r := &reporter{out: &printed, machine: true, action: "decompress"}
	r.progress("C:/games/a.nsz", "Decompressing")(1, 3)
	r.done("C:/games/a.nsz", "C:/games/a.nsp", "", nil)
	r.done("C:/games/b.nsz", "", "", errors.New("damaged"))
	r.done("C:/games/c.nsz", "", "", skipped("already there"))
	r.say("a.nsz:\n  0100000000010000 BASE v0")
	r.note("--keep has no effect in NXPress")
	r.finish()
	lines := jsonLines(t, printed.String())
	want := []map[string]any{
		{"type": "progress", "step": "Decompressing", "file": "C:/games/a.nsz", "percent": 33.3, "currentRead": 1.0, "totalRead": 3.0, "done": 1.0, "total": 3.0},
		{"type": "complete", "file": "C:/games/a.nsz", "result": "ok", "output": "C:/games/a.nsp"},
		{"type": "error", "code": 105.0, "message": "b.nsz: damaged", "file": "C:/games/b.nsz", "result": "error", "error": "damaged"},
		{"type": "complete", "file": "C:/games/c.nsz", "result": "skipped", "note": "already there"},
		{"type": "info", "message": "a.nsz:\n  0100000000010000 BASE v0"},
		{"type": "warning", "message": "--keep has no effect in NXPress"},
		{"type": "summary", "success": false, "errorCount": 1.0, "errors": []any{map[string]any{"filename": "C:/games/b.nsz", "message": "damaged"}}},
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("got\n%s", printed.String())
	}
	// spaced and in order as Python prints it, for the programs written for nsz
	if !strings.HasPrefix(printed.String(), `{"type": "progress", "step": "Decompressing", "file": `) {
		t.Fatalf("first line: %q", printed.String())
	}

	printed.Reset()
	(&reporter{out: &printed, machine: true}).finish()
	if printed.String() != `{"type": "summary", "success": true, "errorCount": 0, "errors": []}`+"\n" {
		t.Fatalf("summary without errors: %q", printed.String())
	}

	printed.Reset()
	stop := r.heartbeat(10 * time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	stop()
	for _, line := range jsonLines(t, printed.String()) {
		if line["type"] != "heartbeat" || line["time"].(float64) < 1e9 {
			t.Fatalf("heartbeat: %v", line)
		}
	}
}

// --minimal-output writes the progress over one line, "<spinner> <percentage>% <step>", keeps
// the results and what -i tells on lines of their own, and ends with "100% Done".
func TestMinimalOutputAsNsz(t *testing.T) {
	var printed bytes.Buffer
	r := &reporter{out: &printed, minimal: true}
	progress := r.progress("a.nsp", "Compressing")
	progress(10, 100)
	progress(100, 100)
	r.say("a.nsp:\n  1 title")
	r.done("a.nsp", "a.nsz", "", nil)
	r.finish()
	want := "\r| 10% Compressing" + "\r/ 100% Compressing" + "\r" + strings.Repeat(" ", 18) + "\r" + "a.nsp:\n  1 title\n" +
		"OK a.nsp -> a.nsz\n" + "\r100% Done\n"
	if printed.String() != want {
		t.Fatalf("got %q\nwant %q", printed.String(), want)
	}
	printed.Reset()
	r.overwrite("| 5% Verifying")
	r.overwrite("100% Done")
	if printed.String() != "\r| 5% Verifying\r100% Done     " {
		t.Fatalf("a shorter line clears the rest: %q", printed.String())
	}
}

// Both outputs for other programs cannot be asked at once, as in nsz; --machine-readable from
// the command line gives JSON lines only, ending with the summary.
func TestOutputModesOnTheCommandLine(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runCommand([]string{"--machine-readable", "--minimal-output", "-C", "a.nsp"}, &stdout, &stderr); code != 2 ||
		stdout.String() != `{"type": "error", "code": 107, "message": "--minimal-output and --machine-readable cannot be used together"}`+"\n" ||
		!strings.Contains(stderr.String(), "cannot be used together") {
		t.Fatalf("%d %q %q", code, stdout.String(), stderr.String())
	}

	folder := t.TempDir()
	game := filepath.Join(folder, "a.nsp")
	os.WriteFile(game, []byte("x"), 0o644)
	text, code := runForTest(t, "--machine-readable", "--fat32", "--keep", folder, filepath.Join(folder, "missing.nsp"))
	lines := jsonLines(t, text)
	types := []string{}
	for _, line := range lines {
		types = append(types, line["type"].(string))
	}
	last := lines[len(lines)-1]
	if code != 1 || !reflect.DeepEqual(types, []string{"warning", "error", "complete", "summary"}) || last["errorCount"] != 1.0 {
		t.Fatalf("%d %s", code, text)
	}

	text, code = runForTest(t, "--minimal-output", "--fat32", folder)
	if code != 0 || text != "OK a.nsp (it fits in FAT32 as it is)\n\r100% Done\n" {
		t.Fatalf("minimal: %d %q", code, text)
	}
}
