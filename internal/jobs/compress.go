package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/durable"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// newFile is the name of the new file of path, in the output folder when there is one, held
// for it in the work and free to write.
func newFile(ctx context.Context, o Options, path string, name string) (string, error) {
	name, err := target(o, name)
	if err == nil {
		err = claim(ctx, path, name)
	}
	if err == nil {
		err = free(o, name)
	}
	return name, err
}

func compress(ctx context.Context, path string, o Options, threads int, out Output) (string, string, error) {
	name, err := newFile(ctx, o, path, switchfs.CompressedPath(path))
	if err != nil {
		return "", "", err
	}
	written := temporary(name)
	defer os.Remove(written)
	result, err := switchfs.CompressGame(ctx, path, written, switchfs.CompressOptions{
		Level: strconv.Itoa(o.Level), Workers: threads, Blocks: o.Block, BlockExponent: o.BlockExponent, Sparse: o.Sparse, Rebuild: o.Archive, Progress: out.Progress(path, "Compressing"),
	})
	// the most common reason, told plainly
	if errors.Is(err, switchfs.ErrSparseStorage) {
		return "", "", Skipped(sparseSkipped)
	}
	if errors.Is(err, switchfs.ErrNotCompressible) {
		// why: what the library tells after its sentence, when it tells more
		reason := "nothing in it can be compressed, it is left as it is"
		if detail := strings.TrimPrefix(err.Error(), switchfs.ErrNotCompressible.Error()+": "); detail != err.Error() {
			reason += ": " + detail
		}
		return "", "", Skipped(reason)
	}
	if err != nil {
		return "", "", err
	}
	// the original was hashed while it was read: an NCA that does not match its content ID is
	// checked by its hash trees, then every file of the compressed copy is checked against it
	check, err := switchfs.CheckHashes(ctx, path, result.Hashes)
	if err != nil {
		return "", "", err
	}
	if err := switchfs.VerifyCompressed(ctx, written, result.Hashes, out.Progress(path, "Verifying")); err != nil {
		return "", "", err
	}
	if err := finish(o, written, name); err != nil {
		return "", "", err
	}
	note := fmt.Sprintf("%s -> %s, %s smaller", size(result.InputSize), size(result.OutputSize), percent(result.InputSize-result.OutputSize, result.InputSize))
	if result.Sparse > 0 {
		note += fmt.Sprintf(", %d NCA in sparse storage in the experimental format NCZSECT2: decompress it with NXPress to install it", result.Sparse)
	}
	if result.Rebuilt > 0 {
		note += fmt.Sprintf(", %d LZ4 blocks of Nintendo kept decompressed in the experimental archive format NCZSECT3: decompress it with NXPress to install it", result.Rebuilt)
	}
	// a compressed storage that could not be read leaves its NCA a plain NCZ, with why
	if len(result.Lz4Unread) > 0 {
		note += fmt.Sprintf(", %d NCA with their LZ4 blocks left as they are: their compressed storage could not be read (%s)", len(result.Lz4Unread), strings.Join(result.Lz4Unread, "; "))
	}
	if modified := modifiedNote(check); modified != "" {
		note += ", " + modified
	}
	// the compressed copy matches the original, which was itself checked only in part when a
	// section of an NCA rewritten by a tool cannot be checked on its own
	return removeSource(o, path, check, name, note)
}

func decompress(ctx context.Context, path string, o Options, out Output) (string, string, error) {
	name, err := newFile(ctx, o, path, switchfs.DecompressedPath(path))
	if err != nil {
		return "", "", err
	}
	written := temporary(name)
	defer os.Remove(written)
	check, err := switchfs.DecompressAndCheck(ctx, path, written, out.Progress(path, "Decompressing"))
	if err == nil {
		err = finish(o, written, name)
	}
	if err != nil {
		return "", "", err
	}
	return removeSource(o, path, check, name, modifiedNote(check))
}

func convert(ctx context.Context, path string, o Options, out Output) (string, string, error) {
	name, err := newFile(ctx, o, path, switchfs.ConvertedPath(path))
	if err != nil {
		return "", "", err
	}
	written := temporary(name)
	defer os.Remove(written)
	check, err := switchfs.ConvertAndCheck(ctx, path, written, out.Progress(path, "Converting"))
	if err == nil {
		err = finish(o, written, name)
	}
	if err != nil {
		return "", "", err
	}
	return removeSource(o, path, check, name, modifiedNote(check))
}

// keptInPart is why an original stays: its new file could only be checked in part (an NCA
// rewritten by a tool, with a section that cannot be checked on its own).
const keptInPart = "the original is kept: the new file could only be checked in part"

// replaceChecked gives the new copy of a file worked on in place its name, once it is checked
// whole: a copy checked only in part does not replace it, and the file is left as it is.
func replaceChecked(written string, path string, check switchfs.FileCheck) error {
	if check.Partial {
		return Skipped(keptInPart)
	}
	return durable.CommitFile(written, path)
}

// sameOrNewFile is the name of the file an action that works in place writes for path: path
// itself (inPlace), or a new file in the output folder, held for it in the work and free to
// write. The output folder can be the folder of path written another way: in place is told by
// the file, not by how its name is written.
func sameOrNewFile(ctx context.Context, o Options, path string) (name string, inPlace bool, err error) {
	if name, err = target(o, path); err != nil {
		return "", false, err
	}
	if err = claim(ctx, path, name); err != nil {
		return "", false, err
	}
	if inPlace = SameFile(name, path); !inPlace {
		err = free(o, name)
	}
	return name, inPlace, err
}

// recompress writes an NSZ or XCZ again, solid or in blocks, at another level. Without an output
// folder the file is replaced, once its new copy is checked.
func recompress(ctx context.Context, path string, o Options, threads int, out Output) (string, string, error) {
	name, inPlace, err := sameOrNewFile(ctx, o, path)
	if err != nil {
		return "", "", err
	}
	written := temporary(name)
	defer os.Remove(written)
	result, err := switchfs.RecompressGame(ctx, path, written, switchfs.CompressOptions{
		Level: strconv.Itoa(o.Level), Workers: threads, Blocks: o.Block, BlockExponent: o.BlockExponent, Progress: out.Progress(path, "Recompressing"),
	})
	if err != nil {
		return "", "", err
	}
	check, err := switchfs.VerifyFile(ctx, written, out.Progress(path, "Verifying"))
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
	mode := "solid"
	if o.Block || strings.EqualFold(filepath.Ext(path), ".xcz") {
		mode = "in blocks"
	}
	note := fmt.Sprintf("%s, %s -> %s", mode, size(result.InputSize), size(result.OutputSize))
	if modified := modifiedNote(check); modified != "" {
		note += ", " + modified
	}
	if inPlace {
		return name, note, nil
	}
	return removeSource(o, path, check, name, note)
}
