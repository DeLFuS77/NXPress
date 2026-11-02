package jobs

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/DeLFuS77/NXPress/internal/testkit"
)

// The rounds of a watched folder claim their new names in one set: a later round does not take
// the name of the new file of a round before, -w or not.
func TestWatchRoundsShareTheirNewNames(t *testing.T) {
	folder := t.TempDir()
	first := filepath.Join(folder, "a", "Game.nsp")
	second := filepath.Join(folder, "b", "Game.nsp")
	name := filepath.Join(folder, "out", "Game.nsz")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var watched context.Context
	WatchFolder(ctx, Options{Watch: true, Files: []string{folder}}, testkit.Quiet{}, time.Hour, func(ctx context.Context, o Options, out Output) int {
		watched = ctx
		cancel()
		return 0
	})
	if watched == nil {
		t.Fatal("no round")
	}
	// each round adds its files to the set of the watch, as runJobs does
	if err := claim(withDestinations(watched, []string{first}), first, name); err != nil {
		t.Fatal(err)
	}
	if err := claim(withDestinations(watched, []string{second}), second, name); err == nil {
		t.Fatal("a later round took the name of the new file of one before")
	}
}
