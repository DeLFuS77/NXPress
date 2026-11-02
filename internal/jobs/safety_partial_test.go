package jobs

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// safetyPartialNsp writes a game whose check is in part: its program NCA was rewritten by a
// tool, and one of its sections cannot be checked on its own.
func safetyPartialNsp(t *testing.T, path string) string {
	t.Helper()
	program, name := testkit.RewrittenNca()
	testkit.NspWith(t, path, testkit.BaseID, 0, switchfs.ContentMetaType_Application, program, name, nil)
	check, err := switchfs.VerifyFile(context.Background(), path, nil)
	if err != nil || !check.Partial {
		t.Fatalf("the game is not checked in part: %+v %v", check, err)
	}
	return path
}

// A game checked only in part keeps its original: compressed with --rm-source, and
// recompressed in place, where the file is left as it is.
func TestCheckedInPartKeepsTheOriginal(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	nsp := safetyPartialNsp(t, filepath.Join(folder, "game.nsp"))
	name, note, err := compress(context.Background(), nsp, Options{Level: 3, RmSource: true}, 1, testkit.Quiet{})
	if err != nil || !strings.Contains(note, keptInPart) || !testkit.Exists(nsp) {
		t.Fatalf("compress: %q %q %v; the original is there %v", name, note, err, testkit.Exists(nsp))
	}

	before, _ := os.ReadFile(name)
	_, note, err = recompress(context.Background(), name, Options{Level: 5, Block: true, RmSource: true}, 1, testkit.Quiet{})
	if !IsSkipped(err) || err.Error() != keptInPart {
		t.Fatalf("recompress in place: %q %v", note, err)
	}
	if after, _ := os.ReadFile(name); !bytes.Equal(before, after) {
		t.Fatal("the file recompressed in place was replaced")
	}
	if left := testkit.List(folder); len(left) != 2 {
		t.Fatalf("left: %v", left)
	}
}
