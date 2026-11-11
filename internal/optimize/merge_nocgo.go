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

// Rebuild tells this build cannot rebuild a package.
func Rebuild(ctx context.Context, file string, target string, o Options) (RebuildResult, error) {
	return RebuildResult{}, ErrNotBuilt
}

// CheckRebuilt tells this build cannot check a rebuilt NSP.
func CheckRebuilt(ctx context.Context, path string, progress func(string, int64, int64)) error {
	return ErrNotBuilt
}
