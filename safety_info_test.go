package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// Info of a file that cannot be read fails, with the bytes it found where a header should be
// shown as ?, and the work ends with an error.
func TestInfoOfAFileThatCannotBeReadFails(t *testing.T) {
	safetyIsolate(t)
	path := filepath.Join(t.TempDir(), "damaged.nsp")
	os.WriteFile(path, append([]byte{'E', 0x00, 0xFF, 0xFE}, make([]byte, 0x200)...), 0o644)
	var text bytes.Buffer
	if failed := runJobs(context.Background(), options{info: true, files: []string{path}, multi: 1, threads: 1}, &reporter{out: &text}); failed != 1 {
		t.Fatalf("%d failed: %s", failed, text.String())
	}
	line := text.String()
	if !strings.HasPrefix(line, "FAILED damaged.nsp: cannot be read: ") || !strings.Contains(line, "E???") || !utf8.ValidString(line) || strings.ContainsRune(line, 0) {
		t.Fatalf("%q", line)
	}
}

// An error is shown with what is not printable as ?, and a file skipped stays skipped.
func TestShownErrors(t *testing.T) {
	if got := shown(errors.New("got 'E\x00\xff'")).Error(); got != "got 'E??'" {
		t.Fatalf("%q", got)
	}
	if err := shown(skipped("left \x01 as it is")); !isSkipped(err) || err.Error() != "left ? as it is" {
		t.Fatalf("%v", err)
	}
	plain := errors.New("ゲーム.nsp is already there")
	if shown(plain) != plain || shown(nil) != nil {
		t.Fatal("an error that is printable is shown as it is")
	}
}
