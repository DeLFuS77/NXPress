package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// trim takes away what installers do not use: the padding after the data of an XCI, or the
// delta fragments of the updates of an NSP or NSZ. In place, or into the output folder.
func trim(ctx context.Context, path string, o Options, out Output) (string, string, error) {
	if extension := strings.ToLower(filepath.Ext(path)); extension == ".nsp" || extension == ".nsz" {
		return stripDeltas(ctx, path, o, out)
	}
	name, inPlace, err := sameOrNewFile(ctx, o, path)
	if err != nil {
		return "", "", err
	}
	nothingToTrim := Skipped("nothing to trim, it ends where its data ends")
	var (
		removed int64
		check   switchfs.FileCheck
	)
	if inPlace {
		// in place the file is cut: it is checked first, and cut only when checked whole
		if facts, err := switchfs.DescribeFile(path); err == nil && facts.Padding == 0 {
			return "", "", nothingToTrim
		}
		if check, err = switchfs.VerifyFile(ctx, path, out.Progress(path, "Verifying")); err != nil {
			return "", "", err
		}
		if check.Partial {
			return "", "", Skipped("it is left as it is: it could only be checked in part")
		}
		removed, err = switchfs.TrimXci(ctx, path, path, out.Progress(path, "Trimming"))
	} else {
		written := temporary(name)
		defer os.Remove(written)
		removed, err = switchfs.TrimXci(ctx, path, written, out.Progress(path, "Trimming"))
		if err == nil {
			check, err = switchfs.VerifyFile(ctx, written, out.Progress(path, "Verifying"))
		}
		if err == nil {
			err = finish(o, written, name)
		}
	}
	if errors.Is(err, switchfs.ErrNothingToTrim) {
		return "", "", nothingToTrim
	}
	if err != nil {
		return "", "", err
	}
	note := fmt.Sprintf("%s of padding taken away", size(removed))
	if modified := modifiedNote(check); modified != "" {
		note += ", " + modified
	}
	if inPlace {
		return name, note, nil
	}
	return removeSource(o, path, check, name, note)
}

// stripDeltas writes an NSP or NSZ again without the delta fragments of its updates.
func stripDeltas(ctx context.Context, path string, o Options, out Output) (string, string, error) {
	deltas, err := switchfs.DeltaFragmentsSize(path)
	if err != nil {
		return "", "", err
	}
	if deltas == 0 {
		return "", "", Skipped("nothing to trim, it holds no delta fragments: only updates have them")
	}
	name, inPlace, err := sameOrNewFile(ctx, o, path)
	if err != nil {
		return "", "", err
	}
	written := temporary(name)
	defer os.Remove(written)
	removed, check, err := switchfs.StripDeltasAndCheck(ctx, path, written, out.Progress(path, "Trimming"))
	if err != nil {
		return "", "", err
	}
	if inPlace {
		err = replaceChecked(written, path, check)
	} else {
		err = finish(o, written, name)
	}
	if err != nil {
		return "", "", err
	}
	note := fmt.Sprintf("%s of delta fragments taken away", size(removed))
	if modified := modifiedNote(check); modified != "" {
		note += ", " + modified
	}
	if inPlace {
		return name, note, nil
	}
	return removeSource(o, path, check, name, note)
}
