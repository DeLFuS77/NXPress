package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// restartCalls stands in for the start of the new version and the close of the window, and
// counts them.
type restartCalls struct {
	started, quit atomic.Int32
	fail          atomic.Bool
}

func catchRestart(t *testing.T) *restartCalls {
	t.Helper()
	calls := &restartCalls{}
	start, quit := startDetached, quitWindow
	startDetached = func(string, ...string) error {
		if calls.fail.Load() {
			return errors.New("cannot start")
		}
		calls.started.Add(1)
		return nil
	}
	quitWindow = func(context.Context) { calls.quit.Add(1) }
	t.Cleanup(func() { startDetached, quitWindow = start, quit })
	return calls
}

func restartRefused(t *testing.T, a *App, calls *restartCalls, during string) {
	t.Helper()
	err := a.RestartUpdated("verify", nil)
	if err == nil || !strings.HasPrefix(err.Error(), "work:") {
		t.Fatalf("restart during %s: %v, want a work going on", during, err)
	}
	if calls.started.Load() != 0 || calls.quit.Load() != 0 {
		t.Fatalf("NXPress restarted during %s", during)
	}
	a.mutex.Lock()
	restarting := a.restarting
	a.mutex.Unlock()
	if restarting {
		t.Fatalf("after a restart refused during %s, no work can start", during)
	}
}

// restartChecks runs in node over app.js: what the restart into a new version waits for.
const restartChecks = `
"use strict";
const fs = require("fs"), vm = require("vm");
const source = fs.readFileSync(process.argv[1], "utf8").replace(/\r\n/g, "\n");
const start = source.indexOf("function restartWaits(");
const context = {};
vm.createContext(context);
vm.runInContext(source.slice(start, source.indexOf("\n}\n", start) + 3) + "this.restartWaits = restartWaits;", context);
const idle = { busy: false, starting: false, openedWaiting: [], shutdownAt: 0 };
const cases = [
  ["nothing going on", idle, false, false],
  ["a work (a watch of folders too)", { ...idle, busy: true }, false, true],
  ["a work starting", { ...idle, starting: true }, false, true],
  ["a check of the files opened about to start", idle, true, true],
  ["files of the Explorer still to come in", { ...idle, openedWaiting: [{ files: ["a.nsp"] }] }, false, true],
  ["a count to turn the computer off", { ...idle, shutdownAt: Date.now() + 60000 }, false, true],
];
const problems = [];
for (const [what, state, startPending, want] of cases) {
  if (context.restartWaits(state, startPending) !== want) problems.push(what + ": the restart " + (want ? "does not wait" : "waits"));
}
console.log(problems.join("\n"));
`

// The page waits to restart into the new version while anything goes on (#91), the works it is
// about to start included.
func TestRestartWaitsInThePage(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	out, err := exec.Command(node, "-e", restartChecks, "frontend/app.js").CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, problem := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if problem != "" {
			t.Error(problem)
		}
	}
}

// The restart into a new version waits for every work (#91): it is refused during a work of the
// files, a watch of folders (which goes on until it is stopped), and a count to turn the computer
// off; the page waits for their end.
func TestRestartWaitsForEveryWork(t *testing.T) {
	catchEvents(t)
	calls := catchRestart(t)
	a := &App{ctx: context.Background(), installed: "v9.9.9"}

	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.cancel = cancel
	restartRefused(t, a, calls, "a work")
	a.cancel = nil

	if err := a.Start(Request{Mode: "recompress", Files: []string{t.TempDir()}, Watch: true}); err != nil {
		t.Fatal(err)
	}
	restartRefused(t, a, calls, "a watch of folders")
	a.closed(context.Background())

	a.shutdown = time.AfterFunc(time.Hour, func() {})
	restartRefused(t, a, calls, "the count to turn the computer off")
	a.CancelShutDown()

	if err := a.RestartUpdated("verify", nil); err != nil {
		t.Fatal(err)
	}
	if calls.started.Load() != 1 || calls.quit.Load() != 1 {
		t.Fatalf("NXPress started %d new versions and closed %d times, want 1 and 1", calls.started.Load(), calls.quit.Load())
	}
}

// Once the restart is asked, no work starts, and the chores going on (the log of the work that
// just ended, the metadata of a NUP being written) end before NXPress restarts; the ones the
// user asks for meanwhile are refused.
func TestRestartWaitsForTheChores(t *testing.T) {
	catchEvents(t)
	calls := catchRestart(t)
	a := &App{ctx: context.Background(), installed: "v9.9.9"}
	done, err := a.chore(true)
	if err != nil {
		t.Fatal(err)
	}
	restarted := make(chan error, 1)
	go func() { restarted <- a.RestartUpdated("verify", nil) }()
	eventually(t, "the restart did not begin", func() bool {
		a.mutex.Lock()
		defer a.mutex.Unlock()
		return a.restarting
	})

	if err := a.Start(Request{Mode: "recompress", Files: []string{t.TempDir()}, Watch: true}); !errors.Is(err, errRestarting) {
		t.Fatalf("a work starts while NXPress restarts: %v", err)
	}
	fixed := false
	fix := fixNupMeta
	fixNupMeta = func(string) (string, error) { fixed = true; return "", nil }
	defer func() { fixNupMeta = fix }()
	if _, err := a.FixNupMeta("game.nup"); !errors.Is(err, errRestarting) || fixed {
		t.Fatalf("a NUP is written while NXPress restarts: %v", err)
	}
	// the log of the work that ended is still written, and waited for
	logged, err := a.chore(false)
	if err != nil {
		t.Fatalf("the log of the work that ended is refused: %v", err)
	}

	time.Sleep(5 * choreTick)
	if calls.started.Load() != 0 {
		t.Fatal("NXPress restarted before its chores ended")
	}
	done()
	time.Sleep(5 * choreTick)
	if calls.started.Load() != 0 {
		t.Fatal("NXPress restarted before the log was written")
	}
	logged()
	select {
	case err := <-restarted:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the restart did not happen once the chores ended")
	}
	if calls.started.Load() != 1 || calls.quit.Load() != 1 {
		t.Fatalf("NXPress started %d new versions and closed %d times, want 1 and 1", calls.started.Load(), calls.quit.Load())
	}
}

// A new version that does not start lets the works start again: NXPress stays as it was.
func TestRestartThatFailsLetsWorkStart(t *testing.T) {
	catchEvents(t)
	calls := catchRestart(t)
	calls.fail.Store(true)
	a := &App{ctx: context.Background(), installed: "v9.9.9"}
	if err := a.RestartUpdated("verify", nil); err == nil || !strings.HasPrefix(err.Error(), "restart:") {
		t.Fatalf("restart that cannot start the new version: %v", err)
	}
	if calls.quit.Load() != 0 {
		t.Fatal("NXPress closed without its new version")
	}
	done, err := a.chore(true)
	if err != nil {
		t.Fatalf("after a restart that failed, the chores are refused: %v", err)
	}
	done()
	if err := a.Start(Request{Mode: "recompress", Files: []string{t.TempDir()}, Watch: true}); err != nil {
		t.Fatalf("after a restart that failed, no work starts: %v", err)
	}
	a.closed(context.Background())
}

// Closing waits for a chore going on to end, for closeWait at most: the log of the work that just
// ended is written whole.
func TestClosingWaitsForTheChores(t *testing.T) {
	wait := closeWait
	closeWait = time.Second
	defer func() { closeWait = wait }()
	a := &App{ctx: context.Background()}
	done, _ := a.chore(false)
	ended := atomic.Bool{}
	go func() {
		time.Sleep(10 * choreTick)
		ended.Store(true)
		done()
	}()
	a.closed(context.Background())
	if !ended.Load() {
		t.Fatal("NXPress closed before its chore ended")
	}

	// one that never ends does not keep NXPress open
	a.chore(false)
	began := time.Now()
	a.closed(context.Background())
	if waited := time.Since(began); waited > 3*time.Second {
		t.Fatalf("closing waited %v for a chore that does not end", waited)
	}
}
