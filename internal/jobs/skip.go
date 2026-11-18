package jobs

import (
	"context"
	"errors"
	"sync"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// SkippedByHand is why a file skipped from the list while the work went on is left as it is.
const SkippedByHand = "skipped by hand: left as it is"

// Skips are the files of a work that are skipped by hand while it goes on: the one at work stops
// where it is (its half written file is removed, as when the whole work is cancelled), and one
// still waiting is left out when its turn comes. The rest of the work goes on.
type Skips struct {
	mutex   sync.Mutex
	working map[string]context.CancelFunc
	asked   map[string]bool
}

func NewSkips() *Skips {
	return &Skips{working: map[string]context.CancelFunc{}, asked: map[string]bool{}}
}

// Skip skips path: it stops it if it is at work, else it is left out when its turn comes.
func (s *Skips) Skip(path string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.asked[path] = true
	if stop, ok := s.working[path]; ok {
		stop()
	}
}

// begin gives the context path works with, or false when it was skipped before its turn.
func (s *Skips) begin(ctx context.Context, path string) (context.Context, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.asked[path] {
		return ctx, false
	}
	job, stop := context.WithCancel(ctx)
	s.working[path] = stop
	return job, true
}

// end tells path is no longer at work, and whether it was skipped while it was.
func (s *Skips) end(path string) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if stop, ok := s.working[path]; ok {
		stop()
		delete(s.working, path)
	}
	return s.asked[path]
}

type skipsKey struct{}

// WithSkips gives ctx the files a work skips by hand.
func WithSkips(ctx context.Context, s *Skips) context.Context {
	return context.WithValue(ctx, skipsKey{}, s)
}

// Asked tells whether path was skipped by hand.
func (s *Skips) Asked(path string) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.asked[path]
}

// SkipsOf gives the files a work skips by hand, nil when it cannot skip them.
func SkipsOf(ctx context.Context) *Skips {
	s, _ := ctx.Value(skipsKey{}).(*Skips)
	return s
}

// runSkippable runs the job of path with a context of its own when the work can skip files by
// hand: a job stopped that way is told as skipped, not failed.
func runSkippable(ctx context.Context, path string, job func(context.Context) (string, string, error)) (string, string, error) {
	s := SkipsOf(ctx)
	if s == nil {
		return job(ctx)
	}
	own, ok := s.begin(ctx, path)
	if !ok {
		return "", "", Skipped(SkippedByHand)
	}
	output, note, err := job(own)
	if s.end(path) && err != nil && ctx.Err() == nil {
		return "", "", Skipped(SkippedByHand)
	}
	return output, note, err
}

// sparseUpdateSkipped is why the update of a game in sparse storage is left as it is with it.
const sparseUpdateSkipped = "its game uses sparse storage and is left as it is: the update is left with it"

// sparsePairs gives, with -C or --recompress, --skip-sparse-updates and without the extra
// compression, the updates among files whose game, among files too, is in sparse storage and so
// left as it is: they are left as they are with it, whatever comes first in the list. DLC are
// compressed. In Recompress the game is an NSZ or XCZ that holds it in NCZSECT2 (#132).
func sparsePairs(ctx context.Context, o Options, action string, files []string) map[string]string {
	pairs := map[string]string{}
	if (action != "compress" && action != "recompress") || !o.SkipSparseUpdates || o.Sparse {
		return pairs
	}
	order, groups, _ := groupGames(ctx, files)
	for _, id := range order {
		group := groups[id]
		if len(group) < 2 {
			continue
		}
		kinds := map[string]map[string]bool{}
		sparse := false
		for _, file := range group {
			titles, err := ReadTitles(file)
			if err != nil {
				continue
			}
			kinds[file] = map[string]bool{}
			for _, title := range titles {
				kinds[file][title.Type] = true
			}
			if kinds[file]["BASE"] && !sparse && ctx.Err() == nil {
				sparse = inSparseStorage(action, file)
			}
		}
		if !sparse {
			continue
		}
		for _, file := range group {
			if kinds[file]["UPD"] && !kinds[file]["BASE"] {
				pairs[file] = sparseUpdateSkipped
			}
		}
	}
	return pairs
}

// inSparseStorage tells whether a file holds a game in sparse storage, which the action leaves as
// it is without the extra compression: in Compress an NSP or XCI no compressor shrinks, in
// Recompress an NSZ or XCZ that holds it in the extra compression (NCZSECT2).
func inSparseStorage(action string, path string) bool {
	if action == "recompress" {
		facts, err := describeFile(path)
		return err == nil && facts.Sparse > 0
	}
	return errors.Is(switchfs.NotCompressible(path), switchfs.ErrSparseStorage)
}
