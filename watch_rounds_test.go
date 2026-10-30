package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"
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
	watchFolder(ctx, options{watch: true, files: []string{folder}}, quiet{}, time.Hour, func(ctx context.Context, o options, out output) int {
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
