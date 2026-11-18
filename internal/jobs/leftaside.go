package jobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/DeLFuS77/NXPress/internal/durable"
	"github.com/DeLFuS77/NXPress/internal/system"

	"github.com/dtrunk90/switch-library-manager-web/par2"
	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// dlcSkipped is why a DLC is left as it is with --skip-dlc.
const dlcSkipped = "a DLC: DLC are left as they are"

// leftAsTheyAre gives, for -C, the files of the work left as they are before it starts, with
// why: with --skip-sparse-updates the updates whose game in sparse storage is given with them
// (sparsePairs), with --skip-dlc the files of DLC only, and, with --move-skipped, the games in
// sparse storage too (without the extra compression nothing of them is compressed), so they are
// moved before the work starts instead of being found as their turn comes.
func leftAsTheyAre(ctx context.Context, o Options, action string, files []string) map[string]string {
	left := sparsePairs(ctx, o, action, files)
	if action != "compress" || (!o.SkipDlc && o.MoveSkipped == "") {
		return left
	}
	for _, file := range files {
		if ctx.Err() != nil {
			break
		}
		if _, ok := left[file]; ok {
			continue
		}
		titles, err := ReadTitles(file)
		if err != nil {
			// the work tells why when its turn comes
			continue
		}
		kinds := map[string]bool{}
		for _, title := range titles {
			kinds[title.Type] = true
		}
		if o.SkipDlc && len(kinds) == 1 && kinds["DLC"] {
			left[file] = dlcSkipped
			continue
		}
		if o.MoveSkipped != "" && !o.Sparse && kinds["BASE"] && errors.Is(switchfs.NotCompressible(file), switchfs.ErrSparseStorage) {
			left[file] = sparseSkipped
		}
	}
	return left
}

// moveLeft moves, with --move-skipped, the files left as they are into its folder before the
// work starts, and gives the files left to work on. Each file moved is told as skipped, with why
// and where it went; one that cannot be moved stays where it is, told as skipped with why too.
func moveLeft(ctx context.Context, o Options, files []string, left map[string]string, out Output) []string {
	if o.MoveSkipped == "" || len(left) == 0 || ctx.Err() != nil {
		return files
	}
	folder, err := filepath.Abs(o.MoveSkipped)
	if err == nil {
		err = os.MkdirAll(folder, 0o755)
	}
	if err != nil {
		out.Say("the files left as they are stay where they are: %s cannot be used (%v)", o.MoveSkipped, err)
		return files
	}
	rest := []string{}
	moved := 0
	for _, path := range files {
		reason, ok := left[path]
		if !ok || ctx.Err() != nil {
			rest = append(rest, path)
			continue
		}
		to, err := moveInto(ctx, path, folder, out)
		switch {
		case errors.Is(err, context.Canceled):
			// the work was cancelled while it moved: the file stays, and the work tells it
			rest = append(rest, path)
		case err != nil:
			out.Done(path, "", "", Skipped(fmt.Sprintf("%s; it stays where it is: %v", reason, err)))
		case to == path:
			out.Done(path, "", "", Skipped(reason))
		default:
			moved++
			out.Done(path, to, "", Skipped(fmt.Sprintf("%s; moved to %s", reason, folder)))
		}
	}
	if moved > 0 {
		out.Say("%d files left as they are moved to %s", moved, folder)
	}
	return rest
}

// ErrMovedThere is told for a file that has a file of its name in the folder it would be moved to.
var ErrMovedThere = errors.New("a file of its name is there already")

// renameNew and otherDrive are the ones of package system, which the tests stand in for to move
// a file to another drive
var (
	renameNew  = system.RenameNew
	otherDrive = system.OtherDrive
)

// moveInto moves a file into folder under its own name, as safely as NXPress renames a file: a
// file of that name there is never replaced, and the recovery data (.par2) goes with it. On
// another drive it is copied under a hidden name, read back and compared, and given its name;
// the original is deleted only then. A file in folder already stays as it is (it gives path).
func moveInto(ctx context.Context, path string, folder string, out Output) (string, error) {
	if SameFile(filepath.Dir(path), folder) {
		return path, nil
	}
	to := filepath.Join(folder, filepath.Base(path))
	// held for this file in the work: a new file of the work never takes its name
	if err := claim(ctx, path, to); err != nil {
		return "", err
	}
	if _, err := os.Lstat(to); err == nil {
		return "", ErrMovedThere
	}
	err := renameNew(path, to)
	switch {
	case err == nil:
		moveRecovery(path, to)
		return to, nil
	case errors.Is(err, fs.ErrExist):
		return "", ErrMovedThere
	case !otherDrive(err):
		return "", err
	}
	if err := copyAcross(ctx, path, to, out.Progress(path, "Moving")); err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil {
		// the original is whole where it was: the copy goes, so the file is not in two places
		os.Remove(to)
		return "", fmt.Errorf("it was copied, but the original could not be deleted: %w", err)
	}
	// its recovery data goes with it; a copy that fails leaves it with the original, gone
	if sidecar := par2.Path(path); exists(sidecar) && !exists(par2.Path(to)) {
		if copyAcross(ctx, sidecar, par2.Path(to), nil) == nil {
			os.Remove(sidecar)
		}
	}
	return to, nil
}

// copyAcross copies a file to another drive under a hidden name next to to, reads the copy back
// and compares it with what was read, and gives it its name; it never replaces a file. The
// progress goes over the copy, then over the reading back.
func copyAcross(ctx context.Context, from string, to string, progress func(done int64, total int64)) error {
	input, err := os.Open(from)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	size := info.Size()
	if progress == nil {
		progress = func(int64, int64) {}
	}
	hidden := temporary(to)
	output, err := os.OpenFile(hidden, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer os.Remove(hidden)
	read := sha256.New()
	err = durable.CopyWithProgress(ctx, io.MultiWriter(output, read), input, size, func(done int64, _ int64) { progress(done, 2*size) })
	if closeErr := output.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	copied, err := os.Open(hidden)
	if err != nil {
		return err
	}
	written := sha256.New()
	err = durable.CopyWithProgress(ctx, written, copied, size, func(done int64, _ int64) { progress(size+done, 2*size) })
	copied.Close()
	if err != nil {
		return err
	}
	if !bytes.Equal(read.Sum(nil), written.Sum(nil)) {
		return errors.New("its copy did not read back the same")
	}
	return durable.CommitNew(hidden, to)
}
