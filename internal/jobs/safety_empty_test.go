package jobs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"
)

// A file of 0 bytes is told plainly, in every action, before it is read.
func TestEmptyFileIsToldPlainly(t *testing.T) {
	testkit.Isolate(t)
	path := filepath.Join(t.TempDir(), "empty.nsp")
	os.WriteFile(path, nil, 0o644)
	for _, o := range []Options{{Verify: true}, {Compress: true, Level: 3}, {Info: true}, {Template: "{name}"}, {Trim: true}, {Split: true}, {PackGames: true}} {
		o.Files, o.Multi, o.Threads = []string{path}, 1, 1
		out := &testkit.Results{}
		if failed := RunJobs(context.Background(), o, out); failed != 1 {
			t.Errorf("%s: %d failed", o.Action(), failed)
		}
		if err := out.Results[path].Err; err == nil || err.Error() != "the file is empty" {
			t.Errorf("%s: %v", o.Action(), err)
		}
	}
	if !testkit.Exists(path) {
		t.Fatal("the empty file is gone")
	}
}
