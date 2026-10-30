package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// safetyPause is a window paused until it is resumed.
type safetyPause struct {
	quiet
	resume chan struct{}
	waits  atomic.Int32
}

func (p *safetyPause) waitWhilePaused() {
	p.waits.Add(1)
	<-p.resume
}

// A paused work takes no file, also in an action with no progress (renaming), and goes on
// once resumed.
func TestPausedWorkTakesNoFile(t *testing.T) {
	safetyIsolate(t)
	folder := t.TempDir()
	for i := 0; i < 3; i++ {
		safetyTitleNsp(t, filepath.Join(folder, fmt.Sprintf("game %d.nsp", i)), safetyBaseID+uint64(i)<<16, 0, switchfs.ContentMetaType_Application, i, nil)
	}
	out := &safetyPause{resume: make(chan struct{})}
	done := make(chan int)
	go func() {
		done <- runJobs(context.Background(), options{template: "{id}", files: []string{folder}, multi: 1, threads: 1}, out)
	}()
	for out.waits.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if names := safetyList(folder); names[0] != "game 0.nsp" {
		t.Fatalf("renamed while paused: %v", names)
	}
	close(out.resume)
	if failed := <-done; failed != 0 {
		t.Fatalf("%d failed", failed)
	}
	if names := safetyList(folder); len(names) != 3 || names[0] != "0100000000010000.nsp" {
		t.Fatalf("after: %v", names)
	}
}

// Grouping the files by game stops once the work is cancelled.
func TestGroupingGamesStopsWhenCancelled(t *testing.T) {
	safetyIsolate(t)
	base, update, dlc := safetyGame(t, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if order, _, unread := groupGames(ctx, []string{base, update, dlc}); len(order) != 0 || len(unread) != 0 {
		t.Fatalf("read once cancelled: %v %v", order, unread)
	}
	if order, groups, _ := gameGroups([]string{base, update, dlc}); len(order) != 1 || len(groups[order[0]]) != 3 {
		t.Fatalf("the groups: %v %v", order, groups)
	}
}
