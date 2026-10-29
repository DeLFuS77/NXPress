package main

import (
	"context"
	"syscall"
	"testing"
)

// The priority changes only once the work is taken, and only when what is asked changes; when it
// cannot go back to normal, NXPress says so too.
func TestPriorityChangesWithTheWorkTaken(t *testing.T) {
	events := catchEvents(t)
	changes := []bool{}
	refuse := false
	changed := changePriority
	changePriority = func(low bool) error {
		changes = append(changes, low)
		if refuse {
			return syscall.EPERM
		}
		return nil
	}
	defer func() { changePriority = changed }()

	a := &App{ctx: context.Background()}
	// refused: no keys to compress
	if err := a.Start(Request{Mode: "compress", Files: []string{t.TempDir()}, LowPriority: true}); err == nil {
		t.Fatal("compressing without keys is taken")
	}
	if len(changes) > 0 {
		t.Fatal("the priority changed for work refused")
	}
	watch := Request{Mode: "recompress", Files: []string{t.TempDir()}, Watch: true, LowPriority: true}
	if err := a.Start(watch); err != nil {
		t.Fatal(err)
	}
	// refused: the files are still at work
	if err := a.Start(Request{Mode: "recompress", Files: []string{t.TempDir()}}); err == nil {
		t.Fatal("new work is taken while files are at work")
	}
	a.closed(context.Background())
	if err := a.Start(watch); err != nil {
		t.Fatal(err)
	}
	a.closed(context.Background())
	if len(changes) != 1 || !changes[0] {
		t.Fatalf("the priority changed %v, want once to low", changes)
	}

	refuse = true
	watch.LowPriority = false
	if err := a.Start(watch); err != nil {
		t.Fatal(err)
	}
	a.closed(context.Background())
	eventually(t, "NXPress did not say the priority could not go back to normal", func() bool {
		events.mutex.Lock()
		defer events.mutex.Unlock()
		for i, event := range events.events {
			if said, ok := events.data[i].([]any); ok && event == "say" && len(said) == 1 && said[0] == "the priority could not go back to normal: "+syscall.EPERM.Error() {
				return true
			}
		}
		return false
	})
	if len(changes) != 2 || changes[1] {
		t.Fatalf("the priority changed %v, want low then back", changes)
	}
}
