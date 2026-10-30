package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The thread of the taskbar always ends on the last state asked for, however many come while
// it shows one, and never goes back to an older one.
func TestTaskbarKeepsTheLastState(t *testing.T) {
	slot := newTaskbarSlot()
	const last = 2000
	go func() {
		for done := uint64(1); done <= last; done++ {
			slot.put(taskbarState{flag: taskbarNormal, done: done, total: last})
		}
	}()
	shown := uint64(0)
	deadline := time.Now().Add(5 * time.Second)
	for shown < last {
		if time.Now().After(deadline) {
			t.Fatalf("the last state shown is %d of %d", shown, last)
		}
		state := slot.take()
		if state.done < shown {
			t.Fatalf("%d shown after %d", state.done, shown)
		}
		shown = state.done
		time.Sleep(time.Millisecond)
	}
}

// taskbarNow is the last state asked for the taskbar.
func taskbarNow() taskbarState {
	taskbar.mutex.Lock()
	defer taskbar.mutex.Unlock()
	return taskbar.state
}

// The taskbar shows progress only while there is work, and ends as the work did: nothing once
// stopped, red with files that failed.
func TestTaskbarFollowsTheWork(t *testing.T) {
	catchEvents(t)
	a := &App{ctx: context.Background()}
	setTaskbarProgress("none", 7, 7)
	a.SetTaskbar("normal", 3, 9)
	if now := taskbarNow(); now != (taskbarState{flag: taskbarNone, done: 7, total: 7}) {
		t.Fatalf("progress without work is shown: %+v", now)
	}

	if err := a.Start(Request{Mode: "recompress", Files: []string{t.TempDir()}, Watch: true}); err != nil {
		t.Fatal(err)
	}
	a.SetTaskbar("paused", 3, 9)
	if now := taskbarNow(); now != (taskbarState{flag: taskbarPaused, done: 3, total: 9}) {
		t.Fatalf("progress at work is not shown: %+v", now)
	}
	a.closed(context.Background())
	if now := taskbarNow(); now.flag != taskbarNone {
		t.Fatalf("stopped, the taskbar shows %+v", now)
	}
	a.SetTaskbar("normal", 9, 9)
	if now := taskbarNow(); now.flag != taskbarNone {
		t.Fatalf("progress told late is shown: %+v", now)
	}

	folder := t.TempDir()
	os.WriteFile(filepath.Join(folder, "broken.nsz"), []byte("not a game at all"), 0o644)
	if err := a.Start(Request{Mode: "recompress", Files: []string{folder}}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the work did not end", func() bool { return !a.working() })
	if now := taskbarNow(); now != (taskbarState{flag: taskbarError, done: 1, total: 1}) {
		t.Fatalf("with a file failed, the taskbar shows %+v", now)
	}
}
