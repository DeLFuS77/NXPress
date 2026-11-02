package jobs

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// safetyCanceller cancels the work when a step starts.
type safetyCanceller struct {
	testkit.Quiet
	step   string
	cancel context.CancelFunc
}

func (c *safetyCanceller) Progress(path string, step string) func(int64, int64) {
	if step == c.step {
		c.cancel()
	}
	return func(int64, int64) {}
}

// A split that fails at its second part leaves no part under its name: they all get their
// names once every one is written and checked.
func TestSplitNamesNoPartBeforeAllAreWritten(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	base := testkit.TitleNsp(t, filepath.Join(folder, "b.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	dlc := testkit.TitleNsp(t, filepath.Join(folder, "d.nsp"), testkit.DlcID, 0, switchfs.ContentMetaType_AddOnContent, 3, nil)
	super := filepath.Join(folder, "Game.nsp")
	if _, _, _, err := switchfs.MergeNewestAndCheck(context.Background(), []string{base, dlc}, super, true, nil); err != nil {
		t.Fatal(err)
	}
	os.Remove(base)
	os.Remove(dlc)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, _, err := split(ctx, super, Options{Overwrite: true, RmSource: true}, &safetyCanceller{step: "Splitting DLC", cancel: cancel}); err == nil {
		t.Fatal("the split went on once cancelled")
	}
	if left := testkit.List(folder); !reflect.DeepEqual(left, []string{"Game.nsp"}) {
		t.Fatalf("left: %v", left)
	}

	name, note, err := split(context.Background(), super, Options{RmSource: true}, testkit.Quiet{})
	if err != nil || name != "" || !strings.HasPrefix(note, "2 files: ") {
		t.Fatalf("split: %q %q %v", name, note, err)
	}
	if left := testkit.List(folder); !reflect.DeepEqual(left, []string{"Game [0100000000010000][v0].nsp", "Game [0100000000011001][v0].nsp"}) {
		t.Fatalf("left: %v", left)
	}
}
