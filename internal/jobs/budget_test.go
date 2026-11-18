package jobs

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

const mib = 1 << 20

// A file starting takes the threads free by its size among the files that start with it, one at
// least and never more than its stream keeps busy; what it cannot use goes to the others.
func TestShareThreadsBySize(t *testing.T) {
	for _, c := range []struct {
		name   string
		free   int
		pieces []piece
		want   []int
	}{
		{"one big and six small", 21, []piece{{8 << 30, 0}, {1 << 30, 0}, {1 << 30, 0}, {1 << 30, 0}, {1 << 30, 0}, {1 << 30, 0}, {1 << 30, 0}}, []int{9, 2, 2, 2, 2, 2, 2}},
		{"the same size", 21, []piece{{mib, 0}, {mib, 0}, {mib, 0}, {mib, 0}, {mib, 0}, {mib, 0}, {mib, 0}}, []int{3, 3, 3, 3, 3, 3, 3}},
		{"what a stream cannot use goes to the others", 21, []piece{{4 << 30, 2}, {4 << 30, 0}}, []int{2, 19}},
		{"a small one takes one", 21, []piece{{64 * mib, 0}, {8 << 30, 0}}, []int{1, 20}},
		{"none can use all", 21, []piece{{64 * mib, 2}, {96 * mib, 3}}, []int{2, 3}},
		{"one at least", 3, []piece{{100 << 30, 0}, {mib, 0}, {mib, 0}}, []int{1, 1, 1}},
		{"fewer threads than files", 2, []piece{{mib, 0}, {mib, 0}, {mib, 0}}, []int{1, 1, 0}},
		{"no thread free", 0, []piece{{mib, 0}}, []int{0}},
		{"alone", 8, []piece{{mib, 0}}, []int{8}},
		{"alone with its stream", 8, []piece{{mib, 3}}, []int{3}},
	} {
		if got := shareThreads(c.free, c.pieces); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// Whatever the files, the shares never add up to more than the threads free, each takes one at
// least while there are enough, none more than it can use, and no thread a file could use is left.
func TestShareThreadsKeepsItsBounds(t *testing.T) {
	random := rand.New(rand.NewPCG(1, 2))
	for range 20000 {
		free := random.IntN(40)
		pieces := make([]piece, 1+random.IntN(10))
		usable := 0
		for i := range pieces {
			pieces[i] = piece{bytes: random.Int64N(64 << 30), most: random.IntN(6)}
			if pieces[i].most == 0 {
				usable = free
			}
			usable += pieces[i].most
		}
		shares := shareThreads(free, pieces)
		total := 0
		for i, share := range shares {
			total += share
			if pieces[i].most > 0 && share > pieces[i].most {
				t.Fatalf("%d free, %v: %v, above the most of %d", free, pieces, shares, i)
			}
			if free >= len(pieces) && share < 1 {
				t.Fatalf("%d free, %v: %v, none for %d", free, pieces, shares, i)
			}
		}
		if total > free {
			t.Fatalf("%d free, %v: %v, %d given", free, pieces, shares, total)
		}
		if free >= len(pieces) && total < min(free, usable) {
			t.Fatalf("%d free, %v: %v, %d left that could be used", free, pieces, shares, free-total)
		}
		if free < len(pieces) && total != free {
			t.Fatalf("%d free, %v: %v", free, pieces, shares)
		}
	}
}

// With a total of threads and -m left to choose, one file for every three threads; -m stays the
// most, and without a total it is what it was.
func TestFilesAtOnceFromTheTotal(t *testing.T) {
	for _, c := range []struct {
		o    Options
		want int
	}{
		{Options{TotalThreads: 21}, 7},
		{Options{TotalThreads: 21, Multi: 2}, 2},
		{Options{TotalThreads: 2}, 1},
		{Options{Multi: 0}, 1},
		{Options{Multi: 4}, 4},
	} {
		if got := filesAtOnce(c.o); got != c.want {
			t.Errorf("%+v: %d, want %d", c.o, got, c.want)
		}
	}
}

// testSharing is a work on files of these sizes in MiB, file0.nsp, file1.nsp..., whose streams
// keep busy the threads of most (none for no limit), with the memory taking memory threads.
func testSharing(o Options, live *Live, memory int, most map[string]int, sizes ...int64) *sharing {
	s := &sharing{live: live, base: o, memory: memory, wake: make(chan struct{}), usable: func(path string, _ Options) int { return most[path] }}
	for i, size := range sizes {
		s.waiting = append(s.waiting, &shareFile{path: fmt.Sprintf("file%d.nsp", i), bytes: size * mib, most: -1})
	}
	return s
}

// started starts the files that can start now, and gives them with the threads each took.
func started(s *sharing) ([]*shareFile, []int) {
	files, threads := []*shareFile{}, []int{}
	for {
		f, o, _, _ := s.next()
		if f == nil {
			return files, threads
		}
		if o.Threads != f.threads {
			panic("the options do not carry the threads taken")
		}
		files, threads = append(files, f), append(threads, f.threads)
	}
}

func paths(files []*shareFile) []string {
	names := []string{}
	for _, f := range files {
		names = append(names, f.path)
	}
	return names
}

// The biggest files start first when asked, the next one at each start, and as given otherwise;
// the window can change it while the work goes on.
func TestBiggestFirst(t *testing.T) {
	live := NewLive(1, 1)
	s := testSharing(Options{}, live, 0, nil, 10, 50, 30, 90, 20)
	order := []string{}
	for range 2 {
		files, _ := started(s)
		order = append(order, paths(files)...)
		s.end(files[0])
	}
	live.SetTotal(0, true)
	for s.left() > 0 {
		files, _ := started(s)
		order = append(order, paths(files)...)
		s.end(files[0])
	}
	if want := []string{"file0.nsp", "file1.nsp", "file3.nsp", "file2.nsp", "file4.nsp"}; !slices.Equal(order, want) {
		t.Fatalf("order %v, want %v", order, want)
	}
}

// With a total, the big file takes the bigger share and the small ones what is left, so the last
// one ends as soon as it can, and all of them start at once: the threads at work add up to the
// total and no more.
func TestTotalSharedAsFilesStart(t *testing.T) {
	s := testSharing(Options{TotalThreads: 21, BiggestFirst: true}, nil, 0, nil, 1024, 1024, 8192, 1024, 1024, 1024, 1024)
	files, threads := started(s)
	if !slices.Equal(threads, []int{9, 2, 2, 2, 2, 2, 2}) || files[0].path != "file2.nsp" {
		t.Fatalf("%v took %v", paths(files), threads)
	}
	if s.inUse != 21 {
		t.Fatalf("%d threads at work", s.inUse)
	}
}

// The threads a file cannot keep busy stay free for the others: a file whose stream keeps two
// busy takes two, and the next one the rest.
func TestTotalLeavesWhatAStreamCannotUse(t *testing.T) {
	s := testSharing(Options{TotalThreads: 8, Multi: 2}, nil, 0, map[string]int{"file0.nsp": 2}, 4096, 100)
	_, threads := started(s)
	if !slices.Equal(threads, []int{2, 6}) {
		t.Fatalf("took %v", threads)
	}
}

// A file starts only while a thread is free, the memory caps the total, and the threads of a file
// that ends go to the next one.
func TestTotalWaitsForFreeThreads(t *testing.T) {
	s := testSharing(Options{TotalThreads: 21}, nil, 4, nil, 100, 100, 100, 100, 100, 100)
	files, threads := started(s)
	if !slices.Equal(threads, []int{1, 1, 1, 1}) {
		t.Fatalf("the memory takes 4 threads, the files took %v", threads)
	}
	s.end(files[0])
	more, threads := started(s)
	if len(more) != 1 || threads[0] != 1 || s.inUse != 4 {
		t.Fatalf("after a file ended: %v took %v, %d at work", paths(more), threads, s.inUse)
	}
}

// The threads a file gives back go to the next NCA that starts: a file left alone takes them all
// as its next NCA starts, or what its stream keeps busy.
func TestFreedThreadsGoToTheNextNca(t *testing.T) {
	s := testSharing(Options{TotalThreads: 8, Multi: 2}, nil, 0, nil, 1000, 100)
	files, threads := started(s)
	if !slices.Equal(threads, []int{7, 1}) {
		t.Fatalf("took %v", threads)
	}
	big, small := files[0], files[1]
	s.end(small)
	if got := s.renew(context.Background(), big, 500*mib, 0); got != 8 || s.inUse != 8 {
		t.Fatalf("the next NCA took %d, %d at work", got, s.inUse)
	}
	if got := s.renew(context.Background(), big, 20*mib, 3); got != 3 || s.inUse != 3 {
		t.Fatalf("a small NCA took %d, %d at work", got, s.inUse)
	}
	// without a total, an NCA keeps the threads of its file
	s.base.TotalThreads = 0
	if got := s.renew(context.Background(), big, 20*mib, 0); got != 3 {
		t.Fatalf("without a total the NCA took %d", got)
	}
}

// The window changes the total while the work goes on: lower, no file starts and no NCA takes
// threads until fewer are at work; higher, more start at once; off, each file takes its threads.
func TestTotalChangesWhileTheWorkGoesOn(t *testing.T) {
	live := NewLiveFor(Options{Threads: 1, Multi: 2, TotalThreads: 4})
	s := testSharing(Options{}, live, 0, nil, 100, 100, 100, 100, 100, 100)
	files, threads := started(s)
	if !slices.Equal(threads, []int{2, 2}) {
		t.Fatalf("took %v", threads)
	}
	live.SetTotal(1, false)
	if more, _ := started(s); len(more) != 0 {
		t.Fatalf("%v started above the total", paths(more))
	}
	// the next NCA of a file at work waits while the other holds more than the total
	got := make(chan int)
	go func() { got <- s.renew(context.Background(), files[0], 50*mib, 0) }()
	select {
	case n := <-got:
		t.Fatalf("an NCA took %d threads above a total of 1", n)
	case <-time.After(50 * time.Millisecond):
	}
	live.SetTotal(3, false)
	if n := <-got; n != 1 || s.inUse != 3 {
		t.Fatalf("the NCA took %d once the total went up, %d at work", n, s.inUse)
	}
	s.end(files[0])
	s.end(files[1])
	live.Set(1, 3)
	live.SetTotal(9, false)
	more, threads := started(s)
	if len(more) != 3 || !slices.Equal(threads, []int{3, 3, 3}) {
		t.Fatalf("after the total went up: %v took %v", paths(more), threads)
	}
	for _, f := range more {
		s.end(f)
	}
	live.SetTotal(0, false)
	if _, threads := started(s); !slices.Equal(threads, []int{1}) {
		t.Fatalf("without a total the file took %v", threads)
	}
}

// Whatever happens, no file starts and no NCA takes threads above the total (capped by the
// memory), each takes one at least, and the threads at work add up to the ones taken.
func TestTotalNeverGoesOver(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	random := rand.New(rand.NewPCG(3, 4))
	for round := range 300 {
		total := 1 + random.IntN(24)
		live := NewLiveFor(Options{Threads: 1 + random.IntN(4), Multi: random.IntN(6), TotalThreads: total, BiggestFirst: random.IntN(2) == 0})
		sizes := []int64{}
		most := map[string]int{}
		for i := range 5 + random.IntN(20) {
			sizes = append(sizes, 1+random.Int64N(8192))
			most[fmt.Sprintf("file%d.nsp", i)] = random.IntN(5)
		}
		memory := random.IntN(30)
		s := testSharing(Options{}, live, memory, most, sizes...)
		atWork := []*shareFile{}
		check := func(what string, threads int) {
			t.Helper()
			now, _ := live.Total()
			sum := 0
			for _, f := range atWork {
				sum += f.threads
			}
			if sum != s.inUse {
				t.Fatalf("round %d, %s: %d in use, the files hold %d", round, what, s.inUse, sum)
			}
			if now > 0 && threads > 0 && s.inUse > s.budget(now) {
				t.Fatalf("round %d, %s: %d at work, the total is %d (memory %d)", round, what, s.inUse, now, memory)
			}
		}
		for s.left() > 0 || len(atWork) > 0 {
			switch step := random.IntN(10); {
			case step < 4:
				f, o, _, _ := s.next()
				if f == nil {
					continue
				}
				if f.threads < 1 {
					t.Fatalf("round %d: a file started with %d threads", round, f.threads)
				}
				if most := most[f.path]; o.TotalThreads > 0 && most > 0 && f.threads > most {
					t.Fatalf("round %d: %d threads for a stream that keeps %d busy", round, f.threads, most)
				}
				atWork = append(atWork, f)
				check("start", f.threads)
			case step < 7 && len(atWork) > 0:
				i := random.IntN(len(atWork))
				s.end(atWork[i])
				atWork = slices.Delete(atWork, i, i+1)
				check("end", 0)
			case step < 9 && len(atWork) > 0:
				f := atWork[random.IntN(len(atWork))]
				// a cancelled context: an NCA that would wait takes none instead
				got := s.renew(cancelled, f, random.Int64N(4096)*mib, random.IntN(4))
				check("NCA", got)
			case step == 9:
				live.SetTotal(random.IntN(24), random.IntN(2) == 0)
				live.Set(1+random.IntN(4), random.IntN(6))
			}
		}
	}
}

// files of these sizes in MiB in a folder, named by their size, the given order: file0.nsp...
func sizedFiles(t *testing.T, sizes ...int64) []string {
	t.Helper()
	folder := t.TempDir()
	paths := []string{}
	for i, size := range sizes {
		path := filepath.Join(folder, fmt.Sprintf("file%d.nsp", i))
		file, err := os.Create(path)
		if err == nil {
			err = file.Truncate(size * mib)
		}
		if err == nil {
			err = file.Close()
		}
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	return paths
}

// noMemoryLimit lets a test share its total whatever the memory free.
func noMemoryLimit(t *testing.T) {
	before := memoryThreads
	memoryThreads = func(int) int { return 0 }
	t.Cleanup(func() { memoryThreads = before })
}

// The files of a work start with their share of the total, the biggest first, and the threads at
// work never add up to more than the total.
func TestEachFileSharesTheTotal(t *testing.T) {
	noMemoryLimit(t)
	paths := sizedFiles(t, 1, 1, 8, 1, 1, 1, 1)
	h := newHeldJobs()
	var mutex sync.Mutex
	inUse, most, took := 0, 0, map[string]int{}
	job := func(path string, o Options) {
		mutex.Lock()
		inUse += o.Threads
		most = max(most, inUse)
		took[filepath.Base(path)] = o.Threads
		mutex.Unlock()
		h.job(path, o)
		mutex.Lock()
		inUse -= o.Threads
		mutex.Unlock()
	}
	done := make(chan struct{})
	go func() {
		eachFile(context.Background(), Options{TotalThreads: 21, BiggestFirst: true}, paths, testkit.Quiet{}, job)
		close(done)
	}()
	h.waitFor(t, 7, 7)
	close(h.release)
	<-done
	small := []int{}
	for name, threads := range took {
		if name != "file2.nsp" {
			small = append(small, threads)
		}
	}
	slices.Sort(small)
	if took["file2.nsp"] != 9 || !slices.Equal(small, []int{2, 2, 2, 2, 2, 2}) || most > 21 {
		t.Fatalf("took %v, %d at once", took, most)
	}
}

// A file whose biggest NCA makes fewer jobs of zstd than the threads free takes what its stream
// keeps busy, and leaves the others free.
func TestEachFileTakesWhatItsStreamKeepsBusy(t *testing.T) {
	if !switchfs.NativeZstd {
		t.Skip("the encoder in Go has no jobs")
	}
	noMemoryLimit(t)
	testkit.Isolate(t)
	base, update, _ := testkit.Game(t, t.TempDir())
	h := newHeldJobs()
	done := make(chan struct{})
	go func() {
		eachFile(context.Background(), Options{Compress: true, Level: 18, TotalThreads: 8}, []string{base, update}, testkit.Quiet{}, h.job)
		close(done)
	}()
	h.waitFor(t, 2, 2)
	close(h.release)
	<-done
	if h.started[0].Threads != 1 || h.started[1].Threads != 1 {
		t.Fatalf("small games took %d and %d threads", h.started[0].Threads, h.started[1].Threads)
	}
}

// The jobs of zstd follow the level and the window, as switchfs sets them, and a file keeps busy
// as many threads as its biggest NCA makes jobs; blocks, the extra compression and the works that
// do not compress have no limit.
func TestFrameThreads(t *testing.T) {
	for _, c := range []struct {
		level int
		long  bool
		want  int64
	}{{19, true, 32 * mib}, {12, true, 32 * mib}, {11, true, 16 * mib}, {9, true, 8 * mib}, {7, true, 4 * mib}, {3, true, 2 * mib},
		{19, false, 32 * mib}, {16, false, 16 * mib}, {8, false, 8 * mib}, {2, false, 4 * mib}, {1, false, 2 * mib}} {
		if got := zstdJobSize(c.level, c.long); got != c.want {
			t.Errorf("level %d, long %v: %d MiB, want %d", c.level, c.long, got/mib, c.want/mib)
		}
	}
	testkit.Isolate(t)
	folder := t.TempDir()
	program := testkit.ProgramNcaWith(testkit.GameData(3*mib, 7), nil)
	nsp := testkit.NspWith(t, filepath.Join(folder, "game.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, program, testkit.ContentName(program, ".nca"), nil)
	for _, c := range []struct {
		o    Options
		want int
	}{
		{Options{Compress: true, Level: 1}, 2},
		{Options{Compress: true, Level: 1, ShortWindow: true}, 2},
		{Options{Compress: true, Level: 18}, 1},
		{Options{Compress: true, Level: 1, Block: true}, 0},
		{Options{Compress: true, Level: 1, Archive: true}, 0},
		{Options{Verify: true, Level: 1}, 0},
	} {
		if !switchfs.NativeZstd {
			c.want = 0
		}
		if got := frameThreads(nsp, c.o); got != c.want {
			t.Errorf("%+v: %d, want %d", c.o, got, c.want)
		}
	}
}

// The memory caps the total, and the work tells it.
func TestTotalCappedByTheMemory(t *testing.T) {
	before := memoryThreads
	memoryThreads = func(int) int { return 3 }
	t.Cleanup(func() { memoryThreads = before })
	h := newHeldJobs()
	out := &recording{}
	done := make(chan struct{})
	go func() {
		eachFile(context.Background(), Options{TotalThreads: 8, Level: 19}, files(5), out, h.job)
		close(done)
	}()
	h.waitFor(t, 2, 2)
	h.stays(t, 2, 2)
	close(h.release)
	<-done
	if len(out.said) != 1 || h.started[0].Threads+h.started[1].Threads != 3 {
		t.Fatalf("said %v, took %d and %d", out.said, h.started[0].Threads, h.started[1].Threads)
	}
}

// A compression with a total of threads, the biggest first and the total changed while it goes
// on does every file, and checks each.
func TestCompressionWithATotalOfThreads(t *testing.T) {
	testkit.Isolate(t)
	base, update, dlc := testkit.Game(t, t.TempDir())
	o := Options{Compress: true, Level: 3, TotalThreads: 3, BiggestFirst: true, Files: []string{base, update, dlc}}
	if err := o.Check(); err != nil {
		t.Fatal(err)
	}
	live := NewLiveFor(o)
	out := &changeTotalOnProgress{live: live}
	if failed := RunJobs(WithLive(context.Background(), live), o, out); failed != 0 {
		t.Fatalf("%d failed: %v", failed, out.Results.Results)
	}
	for _, path := range []string{base, update, dlc} {
		if err := out.Results.Results[path].Err; err != nil {
			t.Errorf("%s: %v", path, err)
		}
		if !testkit.Exists(path[:len(path)-len(".nsp")] + ".nsz") {
			t.Errorf("%s was not compressed", path)
		}
	}
}

// changeTotalOnProgress raises the total and turns biggest first off as the first file tells its
// progress.
type changeTotalOnProgress struct {
	testkit.Results
	live *Live
	once sync.Once
}

func (c *changeTotalOnProgress) Progress(string, string) func(int64, int64) {
	return func(int64, int64) { c.once.Do(func() { c.live.SetTotal(5, false) }) }
}
