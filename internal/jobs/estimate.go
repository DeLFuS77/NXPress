package jobs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// how much of a game is compressed to tell its estimate
const (
	estimateSamples    = 8
	estimateSampleSize = 16 << 20
)

// estimate tells the size and the time a compression would give, from samples of the game; with
// --levels, at several levels, to choose one.
func estimate(ctx context.Context, path string, o Options, threads int, out Output) (string, string, error) {
	if o.Levels {
		return compareLevels(ctx, path, o, threads, out)
	}
	result, err := switchfs.EstimateCompression(ctx, path, switchfs.CompressOptions{
		Level: strconv.Itoa(o.Level), Workers: threads, Blocks: o.Block, Sparse: o.Sparse, Progress: out.Progress(path, "Estimating"),
	}, estimateSamples, estimateSampleSize)
	if errors.Is(err, switchfs.ErrNothingToMeasure) {
		return nothingToEstimate(path)
	}
	if err != nil {
		return "", "", err
	}
	return "", fmt.Sprintf("about %s -> %s, %s smaller, %s at level %d", size(result.InputSize), size(result.OutputSize),
		percent(result.InputSize-result.OutputSize, result.InputSize), duration(result.Duration), o.Level), nil
}

// nothingToEstimate ends the estimate of a game with nothing to compress: a game in sparse
// storage is skipped with why, as compressing skips it.
func nothingToEstimate(path string) (string, string, error) {
	if errors.Is(switchfs.NotCompressible(path), switchfs.ErrSparseStorage) {
		return "", "", Skipped(sparseSkipped)
	}
	return "", "nothing in it can be compressed", nil
}

// the levels compared, from light to maximum
var comparedLevels = []int{3, 9, 12, 15, 18, 19}

// compareLevels tells the size and the time of a compression at several levels.
func compareLevels(ctx context.Context, path string, o Options, threads int, out Output) (string, string, error) {
	lines := []string{}
	progress := out.Progress(path, "Estimating")
	for i, level := range comparedLevels {
		result, err := switchfs.EstimateCompression(ctx, path, switchfs.CompressOptions{
			Level: strconv.Itoa(level), Workers: threads, Blocks: o.Block,
		}, estimateSamples/2, estimateSampleSize)
		if errors.Is(err, switchfs.ErrNothingToMeasure) {
			return nothingToEstimate(path)
		}
		if err != nil {
			return "", "", err
		}
		progress(int64(i+1), int64(len(comparedLevels)))
		lines = append(lines, fmt.Sprintf("level %d: about %s, %s smaller, %s", level, size(result.OutputSize),
			percent(result.InputSize-result.OutputSize, result.InputSize), duration(result.Duration)))
	}
	out.Say("%s:\n  %s", filepath.Base(path), strings.Join(lines, "\n  "))
	return "", fmt.Sprintf("%d levels compared", len(comparedLevels)), nil
}

// duration tells a time for people, rounded.
func duration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d s", max(1, int(d.Round(time.Second).Seconds())))
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Round(time.Minute).Minutes()))
	}
	return fmt.Sprintf("%d h %d min", int(d.Hours()), int(d.Round(time.Minute).Minutes())%60)
}
