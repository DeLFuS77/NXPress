package main

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/DeLFuS77/NXPress/internal/jobs"
	"github.com/DeLFuS77/NXPress/internal/system"
	"github.com/DeLFuS77/NXPress/internal/testkit"
	"github.com/dtrunk90/switch-library-manager-web/settings"
)

// While the work is paused the computer may go to sleep; it is kept awake again as the work goes
// on, and let go once the work ends.
func TestPauseLetsTheComputerSleep(t *testing.T) {
	held := 0
	blocked := system.BlockSleep
	system.BlockSleep = func() func() {
		held++
		return func() { held-- }
	}
	defer func() { system.BlockSleep = blocked }()

	release := system.KeepAwake()
	if held != 1 {
		t.Fatalf("at work: %d holds, want 1", held)
	}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &App{cancel: cancel}
	a.SetPaused(true)
	if held != 0 {
		t.Fatal("paused, the computer is still kept awake")
	}
	a.SetPaused(false)
	if held != 1 {
		t.Fatal("going on, the computer is not kept awake again")
	}
	// cancelled while paused, the pause ends
	a.SetPaused(true)
	a.Cancel()
	if held != 1 {
		t.Fatal("cancelled while paused, the computer is not kept awake until the work ends")
	}
	// two works at once: the computer is kept awake until both end
	second := system.KeepAwake()
	release()
	release()
	if held != 1 {
		t.Fatalf("one of two works ended: %d holds, want 1", held)
	}
	second()
	if held != 0 {
		t.Fatalf("the work ended: %d holds, want 0", held)
	}
}

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
		// the events of a work come together: each is kept as if it came alone
		if name == "batch" && len(data) == 1 {
			if batch, ok := data[0].([]windowEvent); ok {
				for _, event := range batch {
					caught.events = append(caught.events, event.Name)
					caught.data = append(caught.data, []any{event.Data})
				}
				return
			}
		}
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

// A folder added to the window that cannot be read is told on the page, with the files that can.
func TestExpandTellsUnreadableFolders(t *testing.T) {
	caught := catchEvents(t)
	folder := t.TempDir()
	game := filepath.Join(folder, "a.nsp")
	os.WriteFile(game, []byte("x"), 0o644)
	locked := testkit.UnreadableFolder(t, folder)
	a := &App{ctx: context.Background()}
	items := a.Expand([]string{folder}, "verify")
	if len(items) != 1 || items[0].Path != game {
		t.Fatalf("items %v", items)
	}
	if caught.count("say") != 1 {
		t.Fatalf("said %d times", caught.count("say"))
	}
	caught.mutex.Lock()
	said := caught.data[0].([]any)[0].(string)
	caught.mutex.Unlock()
	if said != locked+" cannot be read: its files are left out" {
		t.Fatalf("said %q", said)
	}
}

// Any text file is not a prod.keys: one without header_key is refused, and the keys before stay.
func TestKeysNeedAHeaderKey(t *testing.T) {
	folder := t.TempDir()
	good := filepath.Join(folder, "prod.keys")
	// made up, as in the tests of switchfs: no real key
	header := strings.Repeat("0123456789abcdef", 4)
	os.WriteFile(good, []byte("header_key = "+header+"\nkey_area_key_application_00 = "+strings.Repeat("ab", 16)+"\n"), 0o600)
	notes := filepath.Join(folder, "notes.txt")
	os.WriteFile(notes, []byte("things to do\ncopy = the games\n"), 0o600)

	if _, err := jobs.LoadKeys(notes); err == nil || !strings.Contains(err.Error(), "header_key") {
		t.Fatalf("a text file is taken as keys: %v", err)
	}

	a := &App{}
	if path, err := a.useKeys(good); err != nil || path != good {
		t.Fatalf("the keys are refused: %q %v", path, err)
	}
	if _, err := a.useKeys(notes); err == nil || !strings.Contains(err.Error(), "header_key") {
		t.Fatalf("a text file is taken as keys: %v", err)
	}
	if a.keys != good {
		t.Fatalf("the keys in use are %q, want the ones before", a.keys)
	}
	if keys, _ := settings.SwitchKeys(); keys == nil || keys.GetKey("header_key") != header {
		t.Fatal("the keys before are no longer active")
	}
	// a file that cannot be read leaves them too
	if _, err := a.useKeys(filepath.Join(folder, "missing.keys")); err == nil || a.keys != good {
		t.Fatalf("a missing file: %v, keys %q", err, a.keys)
	}
}

// A portable NXPress on Windows sends no notices of the system: readying them writes in the
// registry. Nothing of the window toolkit is called (it would end the test without a window).
func TestPortableSendsNoNotices(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows registers the notices")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	portable := filepath.Join(filepath.Dir(executable), "settings")
	if _, err := os.Stat(portable); err == nil {
		t.Skip("a settings folder is already next to the test program")
	}
	if err := os.Mkdir(portable, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(portable)

	a := &App{}
	a.Notify("NXPress", "done")
	a.Notify("NXPress", "done again")
	if a.noticesReady {
		t.Fatal("a portable NXPress readied the notices of the system")
	}
}

// Paused, the work waits before going on, until it goes on or is cancelled.
func TestWindowWaitsWhilePaused(t *testing.T) {
	app := &App{}
	job, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &window{job: job, app: app}
	waits := func() chan struct{} {
		released := make(chan struct{})
		go func() {
			w.WaitWhilePaused()
			close(released)
		}()
		return released
	}

	app.paused.Store(true)
	released := waits()
	select {
	case <-released:
		t.Fatal("the work went on while paused")
	case <-time.After(400 * time.Millisecond):
	}
	app.paused.Store(false)
	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("the work did not go on once it was no longer paused")
	}

	app.paused.Store(true)
	released = waits()
	cancel()
	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled, the work still waited")
	}

	// not paused, it does not wait at all
	app.paused.Store(false)
	start := time.Now()
	(&window{job: context.Background(), app: app}).WaitWhilePaused()
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("the work waited without a pause")
	}
}

// The priority changes only once the work is taken, and only when what is asked changes; when it
// cannot go back to normal, NXPress says so too. The processes of the window go down with it, are
// put down again now and then while it is low, and come back with it (#70).
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
	var childrenMutex sync.Mutex
	children := []bool{}
	childrenChanged, every := changeChildrenPriority, childrenEvery
	changeChildrenPriority = func(low bool) {
		childrenMutex.Lock()
		defer childrenMutex.Unlock()
		children = append(children, low)
	}
	childrenEvery = 10 * time.Millisecond
	defer func() { changeChildrenPriority, childrenEvery = childrenChanged, every }()

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
	eventually(t, "the processes of the window were not put down again", func() bool {
		childrenMutex.Lock()
		defer childrenMutex.Unlock()
		return len(children) >= 3
	})

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
	childrenMutex.Lock()
	defer childrenMutex.Unlock()
	for i, low := range children {
		if low != (i < len(children)-1) {
			t.Fatalf("the processes of the window changed %v, want low until NXPress came back", children)
		}
	}
}

// These tests never let a turning off come: they look at what decides it, not at shutDown.

// New work stops the turning off asked for once the work before was done, and tells the page.
func TestStartStopsTheShutdown(t *testing.T) {
	events := catchEvents(t)
	a := &App{ctx: context.Background()}
	a.shutdown = time.AfterFunc(time.Hour, func() {})
	a.shutdownRound = 1
	if err := a.Start(Request{Mode: "recompress", Files: []string{t.TempDir()}, Watch: true}); err != nil {
		t.Fatal(err)
	}
	defer a.closed(context.Background())
	a.mutex.Lock()
	pending := a.shutdown != nil
	a.mutex.Unlock()
	if pending {
		t.Fatal("the turning off goes on with new work")
	}
	if a.shutdownDue(1) {
		t.Fatal("the turning off stopped is still due")
	}
	eventually(t, "the page is not told the turning off is cancelled", func() bool { return events.count("shutdownCancelled") == 1 })
}

// As its time comes, the turning off is not done with files at work, nor once cancelled or asked
// again.
func TestShutdownWaitsForNoWork(t *testing.T) {
	events := catchEvents(t)
	a := &App{ctx: context.Background()}
	ask := func() int {
		a.mutex.Lock()
		defer a.mutex.Unlock()
		a.stopShutdown()
		a.shutdownRound++
		a.shutdown = time.AfterFunc(time.Hour, func() {})
		return a.shutdownRound
	}

	round := ask()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.cancel = cancel
	if a.shutdownDue(round) {
		t.Fatal("the computer turns off with files at work")
	}
	if events.count("shutdownCancelled") != 1 {
		t.Fatal("the page is not told the turning off is cancelled")
	}
	if a.shutdown != nil {
		t.Fatal("the turning off not done is still pending")
	}

	a.cancel = nil
	round = ask()
	if !a.shutdownDue(round) {
		t.Fatal("the turning off is not done without work")
	}
	if a.shutdownDue(round) {
		t.Fatal("the turning off is done twice")
	}

	old := ask()
	round = ask()
	if a.shutdownDue(old) {
		t.Fatal("a turning off asked again is done at its first time")
	}
	a.CancelShutDown()
	if a.shutdownDue(round) {
		t.Fatal("a turning off cancelled is done")
	}
}

// The sound chosen for the end of the work reaches the page whole, as a data URL of its kind;
// what is not a sound, or is too long to send, is refused.
func TestSound(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "done.WAV")
	content := []byte("RIFF\x24\x00\x00\x00WAVEfmt ")
	if err := os.WriteFile(wav, content, 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	url, err := app.Sound(wav)
	if err != nil {
		t.Fatal(err)
	}
	if want := "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(content); url != want {
		t.Errorf("Sound = %q, want %q", url, want)
	}

	text := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(text, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Sound(text); err == nil {
		t.Error("a text file was taken as a sound")
	}

	big := filepath.Join(dir, "long.mp3")
	if err := os.WriteFile(big, make([]byte, maxSoundSize+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Sound(big); err == nil || !strings.Contains(err.Error(), "bigger") {
		t.Errorf("a sound over the size was sent: %v", err)
	}

	if _, err := app.Sound(filepath.Join(dir, "gone.ogg")); err == nil {
		t.Error("a missing sound gave no error")
	}
}

// Hibernating comes with the same count as turning off, and once the computer is back the page is
// told the count is over: nothing watches for a computer still on (#62).
func TestHibernateEndsTheCount(t *testing.T) {
	events := catchEvents(t)
	called := 0
	own := hibernate
	hibernate = func() error { called++; return nil }
	t.Cleanup(func() { hibernate = own })
	a := &App{ctx: context.Background()}
	a.HibernateIn(0)
	eventually(t, "the computer is not hibernated, or the page not told", func() bool {
		a.mutex.Lock()
		defer a.mutex.Unlock()
		return called == 1 && events.count("shutdownCancelled") == 1
	})
	if events.count("shutdownFailed") != 0 {
		t.Fatal("hibernating is taken for a failed turning off")
	}
}

// Once the page told its words, closing with files at work is asked by the page, in the theme of
// the window (#115), and ConfirmClose lets the window close.
func TestCloseAskedByThePage(t *testing.T) {
	a := &App{}
	if a.pageAsksClose() {
		t.Error("the page asks before it told its words")
	}
	a.CloseTexts("¿Pararlos y cerrar NXPress?", "Sí", "No")
	if !a.pageAsksClose() {
		t.Error("the page does not ask once it told its words")
	}
	if a.closeConfirmed.Load() {
		t.Error("closing confirmed before it was asked")
	}
}

// The window is told what a measure found: it has the method the measure looks for (#128).
var _ jobs.Measured = (*window)(nil)

// The events of a work go to the page together, in their order: a thousand files done at once
// are a few events for the window, not a thousand (#135).
func TestWindowBatchSendsTogether(t *testing.T) {
	var (
		mutex   sync.Mutex
		batches int
		names   []string
	)
	sent := emit
	emit = func(ctx context.Context, name string, data ...any) {
		mutex.Lock()
		defer mutex.Unlock()
		if name != "batch" {
			t.Errorf("an event alone: %s", name)
			return
		}
		batches++
		for _, event := range data[0].([]windowEvent) {
			names = append(names, event.Name)
		}
	}
	t.Cleanup(func() { emit = sent })
	w := &window{job: context.Background(), batch: newWindowBatch(context.Background())}
	for i := 0; i < 1000; i++ {
		w.Say("file %d", i)
		w.Done("file", "", "", nil)
	}
	w.batch.flush()
	mutex.Lock()
	defer mutex.Unlock()
	if len(names) != 2000 || names[0] != "say" || names[1] != "done" || names[1999] != "done" {
		t.Fatalf("the events: %d, %v", len(names), names[:4])
	}
	if batches > 5 {
		t.Fatalf("%d batches for events sent at once", batches)
	}
}

// The threads and the files at once of the work at hand change while it goes on, and a request
// carries them, the DLC left as they are and their folder to it; without a work nothing changes.
func TestLiveSettingsOfTheWorkAtHand(t *testing.T) {
	catchEvents(t)
	a := &App{ctx: context.Background()}
	a.SetLive(2, 2)
	if err := a.Start(Request{Mode: "recompress", Files: []string{t.TempDir()}, Watch: true, Threads: 6, Multi: 1}); err != nil {
		t.Fatal(err)
	}
	a.mutex.Lock()
	live := a.live
	a.mutex.Unlock()
	if threads, multi := live.Values(); threads != 6 || multi != 1 {
		t.Fatalf("started with %d threads and %d files at once", threads, multi)
	}
	a.SetLive(3, 4)
	if threads, multi := live.Values(); threads != 3 || multi != 4 {
		t.Fatalf("changed to %d threads and %d files at once", threads, multi)
	}
	a.closed(context.Background())
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.live != nil {
		t.Fatal("the work ended and its settings stayed")
	}
}

// Low priority changes at once, also while files are at work, and only when it changes.
func TestLowPriorityChangesAtOnce(t *testing.T) {
	events := catchEvents(t)
	var mutex sync.Mutex
	changes := []bool{}
	refuse := false
	changed := changePriority
	changePriority = func(low bool) error {
		mutex.Lock()
		defer mutex.Unlock()
		changes = append(changes, low)
		if refuse {
			return syscall.EPERM
		}
		return nil
	}
	childrenChanged := changeChildrenPriority
	changeChildrenPriority = func(bool) {}
	defer func() { changePriority, changeChildrenPriority = changed, childrenChanged }()

	a := &App{ctx: context.Background()}
	if err := a.Start(Request{Mode: "recompress", Files: []string{t.TempDir()}, Watch: true}); err != nil {
		t.Fatal(err)
	}
	a.SetLowPriority(true)
	a.SetLowPriority(true)
	refuse = true
	a.SetLowPriority(false)
	a.closed(context.Background())
	mutex.Lock()
	defer mutex.Unlock()
	if len(changes) != 2 || !changes[0] || changes[1] {
		t.Fatalf("the priority changed %v, want low then back", changes)
	}
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
}

// A request to compress gives the work its DLC left as they are and the folder of what is left;
// Optimize for install, and the other works, take neither.
func TestRequestOfTheFilesLeftAsTheyAre(t *testing.T) {
	asked := Request{Mode: "compress", SkipDlc: true, MoveSkipped: `D:\left`}
	o := (&App{}).requestOptions(asked)
	if !o.SkipDlc || o.MoveSkipped != `D:\left` {
		t.Fatalf("compress: %+v", o)
	}
	asked.Optimize = true
	if o := (&App{}).requestOptions(asked); o.SkipDlc || o.MoveSkipped != "" {
		t.Fatalf("optimize: %+v", o)
	}
	asked.Optimize, asked.Mode = false, "decompress"
	if o := (&App{}).requestOptions(asked); o.SkipDlc || o.MoveSkipped != "" {
		t.Fatalf("decompress: %+v", o)
	}
}

// Recompress leaves as it is what Compress does (#132), and with Optimize for install it is the
// same work as in Compress: the games with their updates, through an NSP.
func TestRequestOfRecompressLeavesAndOptimizes(t *testing.T) {
	asked := Request{Mode: "recompress", SkipDlc: true, SkipSparseUpdates: true, MoveSkipped: `D:\left`}
	o := (&App{}).requestOptions(asked)
	if !o.Recompress || !o.SkipDlc || !o.SkipSparseUpdates || o.MoveSkipped != `D:\left` {
		t.Fatalf("recompress: %+v", o)
	}
	asked.Optimize = true
	o = (&App{}).requestOptions(asked)
	if !o.Optimize || o.Recompress || o.SkipDlc || o.MoveSkipped != "" || o.Action() != "optimize" {
		t.Fatalf("recompress with optimize: %+v", o)
	}
}
