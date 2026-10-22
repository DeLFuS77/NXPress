package main

import (
	"context"
	"os"
	"time"
)

// how often a watched folder is looked at
const watchEvery = 30 * time.Second

// watchJobs does the action on the files given, then keeps looking at them: a new file is
// taken once its size stays the same from one look to the next (it is no longer being copied).
// It goes on until ctx ends, and gives how many files failed.
func watchJobs(ctx context.Context, o options, out output, every time.Duration) int {
	extensions := takes[o.action()]
	seen := map[string]bool{}
	files, _ := inputs(o.files, extensions)
	for _, path := range files {
		seen[path] = true
	}
	failed := runJobs(ctx, o, out)
	out.say("watching for new files")
	sizes := map[string]int64{}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return failed
		case <-ticker.C:
		}
		files, _ := inputs(o.files, extensions)
		ready := []string{}
		for _, path := range files {
			if seen[path] {
				continue
			}
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			if last, ok := sizes[path]; ok && last == info.Size() && info.Size() > 0 {
				ready = append(ready, path)
				seen[path] = true
				delete(sizes, path)
				continue
			}
			sizes[path] = info.Size()
		}
		if len(ready) == 0 {
			continue
		}
		next := o
		next.files = ready
		failed += runJobs(ctx, next, out)
	}
}
