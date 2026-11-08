package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	wailsoptions "github.com/wailsapp/wails/v2/pkg/options"
)

// Many files sent from the Explorer at once, the window minimized: bringing the window to the
// front waits for its thread, and that thread waits for the files to be taken. The files are
// taken without waiting for the window, which one call at a time brings to the front.
func TestSecondInstanceDoesNotWaitForTheWindow(t *testing.T) {
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "a.nsp"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	calls := make(chan struct{}, 16)
	before := bringToFront
	bringToFront = func() {
		calls <- struct{}{}
		<-release
	}
	defer func() { bringToFront = before }()

	a := &App{}
	taken := make(chan struct{})
	go func() {
		for range 10 {
			a.secondInstance(wailsoptions.SecondInstanceData{Args: []string{"--open", "a.nsp"}, WorkingDirectory: folder})
		}
		close(taken)
	}()
	select {
	case <-taken:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("the files wait for the window to come to the front")
	}
	if opened := a.TakeOpened(); len(opened) != 10 {
		t.Errorf("%d files taken", len(opened))
	}
	<-calls
	if len(calls) != 0 {
		t.Errorf("%d more calls brought the window to the front while one was going on", len(calls))
	}
	close(release)
	// once it ends, the next files bring the window to the front again
	for deadline := time.Now().Add(5 * time.Second); a.fronting.Load(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the window never stopped coming to the front")
		}
	}
	bringToFront = func() { calls <- struct{}{} }
	a.secondInstance(wailsoptions.SecondInstanceData{Args: []string{"--open", "a.nsp"}, WorkingDirectory: folder})
	select {
	case <-calls:
	case <-time.After(5 * time.Second):
		t.Error("the window does not come to the front again")
	}
}
