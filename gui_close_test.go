package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// pageEvents keeps the events sent to the page while a test runs, instead of sending them.
type pageEvents struct {
	mutex  sync.Mutex
	events []string
	data   []any
}

func catchEvents(t *testing.T) *pageEvents {
	t.Helper()
	caught := &pageEvents{}
	sent := emit
	emit = func(ctx context.Context, name string, data ...any) {
		caught.mutex.Lock()
		defer caught.mutex.Unlock()
		caught.events = append(caught.events, name)
		caught.data = append(caught.data, data)
	}
	t.Cleanup(func() { emit = sent })
	return caught
}

func (p *pageEvents) count(name string) int {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	n := 0
	for _, event := range p.events {
		if event == name {
			n++
		}
	}
	return n
}

// Closed with files at work, NXPress stops them and waits for them to remove what they had half
// written before it ends.
func TestClosingWaitsForTheWork(t *testing.T) {
	half := filepath.Join(t.TempDir(), ".game.nsz.nxpress")
	os.WriteFile(half, []byte("half"), 0o644)
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	a := &App{cancel: cancel, finished: finished}
	go func() {
		<-ctx.Done()
		// removing a big file takes a while
		time.Sleep(200 * time.Millisecond)
		os.Remove(half)
		a.mutex.Lock()
		a.cancel, a.finished = nil, nil
		a.mutex.Unlock()
		close(finished)
	}()
	a.closed(context.Background())
	if _, err := os.Stat(half); err == nil {
		t.Fatal("NXPress ended before its work removed what it had half written")
	}
}

// Work that does not stop does not keep NXPress from ending for long.
func TestClosingWaitsAWhileAtMost(t *testing.T) {
	waited := closeWait
	closeWait = 100 * time.Millisecond
	defer func() { closeWait = waited }()
	_, cancel := context.WithCancel(context.Background())
	a := &App{cancel: cancel, finished: make(chan struct{})}
	start := time.Now()
	a.closed(context.Background())
	if time.Since(start) > 2*time.Second {
		t.Fatal("NXPress waited too long for work that did not stop")
	}
	// without work it does not wait at all
	start = time.Now()
	(&App{}).closed(context.Background())
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("NXPress waited without work")
	}
}

// The work Start begins is what closing waits for: a folder watched goes on until it is stopped.
func TestClosingStopsAWatch(t *testing.T) {
	catchEvents(t)
	a := &App{ctx: context.Background()}
	if err := a.Start(Request{Mode: "recompress", Files: []string{t.TempDir()}, Watch: true}); err != nil {
		t.Fatal(err)
	}
	if !a.working() {
		t.Fatal("a watch is not work")
	}
	a.closed(context.Background())
	if a.working() {
		t.Fatal("closing did not wait for the watch to stop")
	}
}

// The question asked as the window closes is in the language the page told, English before.
func TestClosingAsksInTheLanguageOfThePage(t *testing.T) {
	a := &App{}
	if question, yes, no := a.closeQuestion(); question == "" || yes != "Yes" || no != "No" {
		t.Fatalf("before the page: %q %q %q", question, yes, no)
	}
	a.CloseTexts("¿Pararlos y cerrar NXPress?", "Sí", "No")
	if question, yes, no := a.closeQuestion(); question != "¿Pararlos y cerrar NXPress?" || yes != "Sí" || no != "No" {
		t.Fatalf("in Spanish: %q %q %q", question, yes, no)
	}
	// a page without the words keeps the ones it had
	a.CloseTexts("", "", "")
	if question, _, _ := a.closeQuestion(); question != "¿Pararlos y cerrar NXPress?" {
		t.Fatalf("kept: %q", question)
	}
}
