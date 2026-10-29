package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A file of 0 bytes is told plainly, in every action, before it is read.
func TestEmptyFileIsToldPlainly(t *testing.T) {
	safetyIsolate(t)
	path := filepath.Join(t.TempDir(), "empty.nsp")
	os.WriteFile(path, nil, 0o644)
	for _, o := range []options{{verify: true}, {compress: true, level: 3}, {info: true}, {template: "{name}"}, {trim: true}, {split: true}, {packGames: true}} {
		o.files, o.multi, o.threads = []string{path}, 1, 1
		out := &safetyResults{}
		if failed := runJobs(context.Background(), o, out); failed != 1 {
			t.Errorf("%s: %d failed", o.action(), failed)
		}
		if err := out.results[path].err; err == nil || err.Error() != "the file is empty" {
			t.Errorf("%s: %v", o.action(), err)
		}
	}
	if !safetyExists(path) {
		t.Fatal("the empty file is gone")
	}
}
