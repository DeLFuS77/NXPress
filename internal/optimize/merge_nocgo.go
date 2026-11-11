//go:build !cgo

package optimize

import "context"

// Built tells this build has Optimize for install: it needs cgo.
const Built = false

// Run tells this build cannot optimize.
func Run(ctx context.Context, files []string, target string, o Options) (Result, error) {
	return Result{}, ErrNotBuilt
}

// CheckMerged tells this build cannot check an optimized NSP.
func CheckMerged(ctx context.Context, path string, want [32]byte, progress func(string, int64, int64)) error {
	return ErrNotBuilt
}
