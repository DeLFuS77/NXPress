package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/dtrunk90/switch-library-manager-web/par2"
)

// Recovery data: a file "<name>.par2" next to a game, in the PAR 2.0 format (see package par2),
// rebuilds the slices of it that get damaged on the disk over time, as many as it holds recovery
// slices. --par2 writes it for each new file, Verify with --repair rebuilds a damaged file with
// it, and the files that change keep it in step: a file renamed takes its .par2 along, an
// original deleted takes it with it, and a file written again gets it written again.

// recoveryActions are the works whose new file gets its recovery data with --par2
var recoveryActions = map[string]bool{
	"compress": true, "recompress": true, "decompress": true, "convert": true, "tocard": true, "trim": true, "rebuild": true,
}

// recoveryAfter writes the recovery data of the new file a work gave, with --par2, and the one of
// a file written again that had one already, as big as it was; it tells it in a note. A file
// whose recovery data cannot be written is still there, whole: only the note tells it.
func recoveryAfter(ctx context.Context, o Options, action string, output string, out Output) string {
	if output == "" || isDir(output) || isNup(output) || !recoveryActions[action] {
		return ""
	}
	sidecar := par2.Path(output)
	percent := float64(o.Par2)
	if percent <= 0 {
		// a .par2 already there was made for the file before: it is made again, or it would tell
		// the new file damaged
		slices, recovery, err := par2.Describe(sidecar)
		if err != nil {
			if exists(sidecar) {
				os.Remove(sidecar)
			}
			return ""
		}
		percent = float64(recovery) * 100 / float64(slices)
	}
	if err := par2.Create(ctx, output, sidecar, percent, out.Progress(output, "Recovery data")); err != nil {
		if errors.Is(err, context.Canceled) {
			return ""
		}
		return "its recovery data (.par2) could not be written: " + err.Error()
	}
	return "with recovery data (.par2)"
}

// moveRecovery gives the recovery data of a file renamed the new name of the file.
func moveRecovery(from string, to string) {
	if exists(par2.Path(from)) && !SameFile(par2.Path(from), par2.Path(to)) {
		os.Rename(par2.Path(from), par2.Path(to))
	}
}

// removeRecovery deletes the recovery data of a file deleted.
func removeRecovery(path string) {
	os.Remove(par2.Path(path))
}

// repairWithRecovery rebuilds, with --repair, a file Verify found damaged with its recovery
// data, and tells what it did; repaired is true when the file was rebuilt whole, so it is
// checked again.
func repairWithRecovery(ctx context.Context, path string, o Options, out Output) (repaired bool, note string) {
	sidecar := par2.Path(path)
	if !o.Repair || !exists(sidecar) {
		return false, ""
	}
	result, err := par2.Repair(ctx, sidecar, path, out.Progress(path, "Repairing"))
	switch {
	case err == nil && len(result.Damaged) == 0 && !result.WrongLength:
		// its recovery data finds it whole: what Verify found is not damage on the disk
		return false, ""
	case err == nil:
		return true, fmt.Sprintf("repaired with its recovery data: %d damaged slices of %d rebuilt", len(result.Damaged), result.Slices)
	case errors.Is(err, par2.ErrTooDamaged):
		return false, fmt.Sprintf("its recovery data cannot repair it: %d damaged slices, it rebuilds %d", len(result.Damaged), result.Recovery)
	default:
		return false, "its recovery data could not repair it: " + err.Error()
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
