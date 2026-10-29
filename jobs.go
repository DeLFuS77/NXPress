package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// the files each action takes, by extension
var takes = map[string][]string{
	"compress":   {".nsp", ".xci"},
	"decompress": {".nsz", ".xcz"},
	"verify":     {".nsp", ".nsz", ".xci", ".xcz"},
	"convert":    {".xci", ".xcz"},
	"recompress": {".nsz", ".xcz"},
	"trim":       {".xci", ".nsp", ".nsz"},
	"estimate":   {".nsp", ".xci"},
	"measure":    {".nsp", ".xci"},
	"rename":     {".nsp", ".nsz", ".xci", ".xcz"},
	// with the files in parts, to make them whole
	"fat32": {".nsp", ".nsz", ".xci", ".xcz", splitFiles},
	"split": {".nsp", ".nsz", ".xci", ".xcz"},
	"info":  {".nsp", ".nsz", ".xci", ".xcz"},
}

// checksOutput tells, by action, whether the new file it gives is checked as a game (every NCA
// by its hashes) before it gets its name, so the cache of the files checked keeps it. A file
// made whole from its parts is only compared with them, and a file renamed is not read.
var checksOutput = map[string]bool{
	"compress":   true,
	"decompress": true,
	"convert":    true,
	"recompress": true,
	"trim":       true,
	"fat32":      false,
	"rename":     false,
	"split":      false,
	"estimate":   false,
	"measure":    false,
	"info":       false,
	"verify":     false,
}

func (o options) action() string {
	switch {
	case o.compress:
		return "compress"
	case o.decompress:
		return "decompress"
	case o.convert:
		return "convert"
	case o.recompress:
		return "recompress"
	case o.trim:
		return "trim"
	case o.estimate:
		return "estimate"
	case o.measure:
		return "measure"
	case o.fat32:
		return "fat32"
	case o.template != "":
		return "rename"
	case o.split:
		return "split"
	case o.info:
		return "info"
	case o.pack != "" || o.packGames:
		return "pack"
	}
	return "verify"
}

// runJobs does the action on every file, several at once with -m, and gives how many failed.
func runJobs(ctx context.Context, o options, out output) int {
	defer checked.flush()
	// the computer does not go to sleep while files are at work
	defer keepAwake()()
	action := o.action()
	if action == "pack" && o.packGames {
		return packGames(ctx, o, out)
	}
	if action == "pack" {
		if err := pack(ctx, o, out); err != nil {
			out.done(o.pack, "", "", shown(err))
			if isSkipped(err) {
				return 0
			}
			return 1
		}
		return 0
	}
	files, missing := inputs(o.files, takes[action])
	for _, path := range missing {
		out.done(path, "", "", errors.New("no such file or folder"))
	}
	if len(files) == 0 {
		out.say("no file to %s among the ones given", action)
		return len(missing)
	}
	// the measure tries its settings on the biggest game that can be compressed
	if action == "measure" {
		path, err := measurableGame(ctx, files, o)
		output, note := "", ""
		if err == nil {
			output, note, err = measure(ctx, path, o, out)
		}
		out.done(path, output, note, shown(err))
		if err != nil && !isSkipped(err) {
			return len(missing) + 1
		}
		return len(missing)
	}
	if o.dat != "" && o.datFile == nil {
		loaded, err := loadDat(o.dat)
		if err != nil {
			out.done(o.dat, "", "", shown(err))
			return len(files) + len(missing)
		}
		o.datFile = loaded
	}
	ctx = withDestinations(ctx, files)
	threads := switchfs.LimitThreads(strconv.Itoa(o.level), o.threads, o.multi)
	var (
		mutex  sync.Mutex
		failed = len(missing)
		wait   sync.WaitGroup
	)
	work := make(chan string)
	for worker := 0; worker < min(o.multi, len(files)); worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for path := range work {
				// a file is taken only while the work is not paused: renaming, -i and the files
				// checked before have no progress of their own to pause in
				waitWhilePaused(out)
				if ctx.Err() != nil {
					continue
				}
				output, note, err := doJob(ctx, action, path, o, threads, out)
				// a new file checked as a game before it got its name need not be read again
				if err == nil && output != "" && checksOutput[action] {
					checked.keep(output, "")
				}
				out.done(path, output, note, shown(err))
				if err != nil && !isSkipped(err) {
					mutex.Lock()
					failed++
					mutex.Unlock()
				}
			}
		}()
	}
	for _, path := range files {
		if ctx.Err() != nil {
			break
		}
		work <- path
	}
	close(work)
	wait.Wait()
	return failed
}

// waitWhilePaused waits while the work is paused, in the window; the console has no pause.
func waitWhilePaused(out output) {
	if p, ok := out.(interface{ waitWhilePaused() }); ok {
		p.waitWhilePaused()
	}
}

// skippedError is a file left as it is, with why: not done, and not failed either, so it
// is told apart from both (SKIPPED, and in yellow in the window).
type skippedError struct {
	reason string
}

func (e skippedError) Error() string {
	return e.reason
}

func skipped(reason string) error {
	return skippedError{reason: reason}
}

func isSkipped(err error) bool {
	var s skippedError
	return errors.As(err, &s)
}

// splitFiles, among the extensions taken, takes the files in parts too: a folder of parts, or
// the first part of name.ns0, name.ns1...
const splitFiles = "parts"

// inputs are the files given and the ones in the folders given (and in their folders), of the
// extensions taken, each once and in order of name; missing are the paths not found, and the
// folders that could not be read. A folder of parts is one file, taken only with splitFiles.
func inputs(paths []string, extensions []string) (files []string, missing []string) {
	seen := map[string]bool{}
	// the files taken by their size, to find one given twice under two names
	bySize := map[int64][]os.FileInfo{}
	withParts := false
	for _, taken := range extensions {
		withParts = withParts || taken == splitFiles
	}
	takes := func(path string) bool {
		if strings.HasPrefix(filepath.Base(path), ".") {
			return false
		}
		if _, ok := splitContainer(path); ok {
			return withParts
		}
		extension := strings.ToLower(filepath.Ext(path))
		for _, taken := range extensions {
			if extension == taken {
				return true
			}
		}
		return false
	}
	add := func(path string) {
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
		if seen[path] {
			return
		}
		seen[path] = true
		// the same file by another name is done once: a.nsp and A.NSP on Windows and macOS, or a
		// link to it
		if info, err := os.Stat(path); err == nil {
			for _, other := range bySize[info.Size()] {
				if os.SameFile(info, other) {
					return
				}
			}
			bySize[info.Size()] = append(bySize[info.Size()], info)
		}
		files = append(files, path)
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			missing = append(missing, path)
			continue
		}
		if _, ok := splitContainer(path); !info.IsDir() || ok {
			if takes(path) {
				add(path)
			}
			continue
		}
		found, unread, _ := folderFiles(path, true)
		for _, folder := range unread {
			missing = append(missing, folder.path)
		}
		for _, walked := range found {
			if takes(walked) {
				add(walked)
			}
		}
	}
	return files, missing
}

// target is where the new file of path goes: next to it, or in the output folder.
func target(o options, name string) (string, error) {
	if o.output == "" {
		return name, nil
	}
	folder, err := outputFolder(o)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(folder, filepath.Base(name)), nil
}

// outputFolder is the folder of -o as an absolute path: "." or a folder given relative names the
// folder it is at the start, not wherever the work is when it is used.
func outputFolder(o options) (string, error) {
	if o.output == "" {
		return "", nil
	}
	return filepath.Abs(o.output)
}

// sameFile tells whether two paths name the same file or folder, however they are written: in
// another case, through a link or a junction, by a short name, or on a mapped drive.
func sameFile(a string, b string) bool {
	first, err := os.Stat(a)
	if err != nil {
		return false
	}
	second, err := os.Stat(b)
	return err == nil && os.SameFile(first, second)
}

// destinations are the names the files of a work take, so that no two of them get the same one
// (nor the hidden name a new file is written under), and none takes the name of a file the
// work has to read: each file given holds its own name from the start, and each new name is
// held by the file it comes from. Names that differ only in case are one on Windows and macOS.
type destinations struct {
	mutex   sync.Mutex
	owners  map[string]string
	folders map[string]string
}

type destinationsKey struct{}

// withDestinations gives ctx the destinations of a work on files.
func withDestinations(ctx context.Context, files []string) context.Context {
	d := &destinations{owners: map[string]string{}, folders: map[string]string{}}
	for _, file := range files {
		d.owners[d.key(file)] = file
	}
	return context.WithValue(ctx, destinationsKey{}, d)
}

// claim holds name for the new file of path, or tells which file of the work has it.
func claim(ctx context.Context, path string, name string) error {
	d, ok := ctx.Value(destinationsKey{}).(*destinations)
	if !ok {
		return nil
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	key := d.key(name)
	owner, ok := d.owners[key]
	switch {
	case !ok || owner == path:
		d.owners[key] = path
		return nil
	case d.key(owner) == key:
		return fmt.Errorf("it would get the same name as %s, another file of this work", owner)
	}
	return fmt.Errorf("it would get the same name as the new file of %s: %s", owner, filepath.Base(name))
}

// key is a name as the file system tells it apart: absolute, with the links of its folder
// followed (and on Windows its short names made long), in lower case where case is not told.
func (d *destinations) key(name string) string {
	if absolute, err := filepath.Abs(name); err == nil {
		name = absolute
	}
	folder := filepath.Dir(name)
	real, ok := d.folders[folder]
	if !ok {
		real = folder
		if followed, err := filepath.EvalSymlinks(folder); err == nil {
			real = followed
		}
		d.folders[folder] = real
	}
	key := filepath.Join(real, filepath.Base(name))
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		key = strings.ToLower(key)
	}
	return key
}

// temporary is the hidden name a new file is written under until it is checked.
func temporary(path string) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".nxpress")
}

// finish gives the checked temporary file its name. With -w the rename replaces a file there,
// and nothing is deleted before it; without -w the rename fails when a file took the name
// meanwhile.
func finish(o options, written string, path string) error {
	if o.overwrite {
		return commitFile(written, path)
	}
	err := commitNew(written, path)
	if errors.Is(err, fs.ErrExist) {
		if there := free(o, path); there != nil {
			return there
		}
	}
	return err
}

// free tells whether a new file can be written at path: a file there is replaced only with -w,
// and without it the file worked on is skipped, as nsz skips it (a script run again over a
// library does not fail for the files done before).
func free(o options, path string) error {
	if _, err := os.Stat(path); err == nil && !o.overwrite {
		return skipped(fmt.Sprintf("%s is already there (use -w to replace it)", filepath.Base(path)))
	}
	return nil
}

// errEmpty is a file of 0 bytes, a copy that never happened: told plainly, before it is read.
var errEmpty = errors.New("the file is empty")

// isEmpty tells whether path is a file of 0 bytes (a folder of parts is not).
func isEmpty(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() == 0
}

func doJob(ctx context.Context, action string, path string, o options, threads int, out output) (string, string, error) {
	if isEmpty(path) {
		return "", "", errEmpty
	}
	switch action {
	case "compress":
		return compress(ctx, path, o, threads, out)
	case "decompress":
		return decompress(ctx, path, o, out)
	case "convert":
		return convert(ctx, path, o, out)
	case "recompress":
		return recompress(ctx, path, o, threads, out)
	case "trim":
		return trim(ctx, path, o, out)
	case "estimate":
		return estimate(ctx, path, o, threads, out)
	case "fat32":
		return fat32(ctx, path, o, out)
	case "rename":
		return rename(ctx, path, o, out)
	case "split":
		return split(ctx, path, o, out)
	case "info":
		note, err := info(path, out)
		return "", note, err
	}
	// the signature of Nintendo on every NCA, read from the headers only (#9)
	signed := ""
	if o.verifyLevel == "signature" || o.verifyLevel == "both" {
		note, err := checkSignatures(path)
		if err != nil || o.verifyLevel == "signature" {
			return "", note, err
		}
		signed = note
	}
	// a file checked before and not changed since is not read again, unless asked
	if entry, ok := checked.find(path); ok && !o.noCache && o.datFile == nil {
		note := "checked on " + entry.Checked.Format("2006-01-02") + ", not changed since"
		if entry.Note != "" {
			note += "; " + entry.Note
		}
		return "", note, nil
	}
	check, err := switchfs.VerifyFile(ctx, path, out.progress(path, "Verifying"))
	note := modifiedNote(check)
	if err == nil {
		checked.keep(path, note)
	}
	if signed != "" {
		note = strings.TrimPrefix(note+"; "+signed, "; ")
	}
	if err != nil || o.datFile == nil {
		return "", note, err
	}
	found, err := o.datFile.match(ctx, path, out.progress(path, "Comparing"))
	if note != "" {
		found = note + "; " + found
	}
	return "", found, err
}

// checkSignatures tells whether every NCA of a file is signed by Nintendo: an error names the
// ones that are not, as a tool that rewrote them leaves them.
func checkSignatures(path string) (string, error) {
	check, err := switchfs.CheckSignatures(path)
	if err != nil {
		return "", err
	}
	if len(check.Unsigned) > 0 {
		return "", fmt.Errorf("%d of %d NCA not signed by Nintendo (rewritten by a tool): %s", len(check.Unsigned), check.Checked, strings.Join(check.Unsigned, ", "))
	}
	note := fmt.Sprintf("%d NCA signed by Nintendo", check.Checked)
	if len(check.Unknown) > 0 {
		note += fmt.Sprintf("; %d signed with a key not known yet", len(check.Unknown))
	}
	return note, nil
}

// sparseSkipped is why a game in sparse storage is left as it is: no compressor reads that
// layout yet, nsz included, and the game installs as it is.
const sparseSkipped = "this game uses sparse storage, which no compressor can read yet (nsz included): it installs as it is"

// newFile is the name of the new file of path, in the output folder when there is one, held
// for it in the work and free to write.
func newFile(ctx context.Context, o options, path string, name string) (string, error) {
	name, err := target(o, name)
	if err == nil {
		err = claim(ctx, path, name)
	}
	if err == nil {
		err = free(o, name)
	}
	return name, err
}

func compress(ctx context.Context, path string, o options, threads int, out output) (string, string, error) {
	name, err := newFile(ctx, o, path, switchfs.CompressedPath(path))
	if err != nil {
		return "", "", err
	}
	written := temporary(name)
	defer os.Remove(written)
	result, err := switchfs.CompressGame(ctx, path, written, switchfs.CompressOptions{
		Level: strconv.Itoa(o.level), Workers: threads, Blocks: o.block, Progress: out.progress(path, "Compressing"),
	})
	// the most common reason, told plainly
	if errors.Is(err, switchfs.ErrSparseStorage) {
		return "", "", skipped(sparseSkipped)
	}
	if errors.Is(err, switchfs.ErrNotCompressible) {
		// why: what the library tells after its sentence, when it tells more
		reason := "nothing in it can be compressed, it is left as it is"
		if detail := strings.TrimPrefix(err.Error(), switchfs.ErrNotCompressible.Error()+": "); detail != err.Error() {
			reason += ": " + detail
		}
		return "", "", skipped(reason)
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
	if err := switchfs.VerifyCompressed(ctx, written, result.Hashes, out.progress(path, "Verifying")); err != nil {
		return "", "", err
	}
	if err := finish(o, written, name); err != nil {
		return "", "", err
	}
	note := fmt.Sprintf("%s -> %s, %s smaller", size(result.InputSize), size(result.OutputSize), percent(result.InputSize-result.OutputSize, result.InputSize))
	if modified := modifiedNote(check); modified != "" {
		note += ", " + modified
	}
	// the compressed copy matches the original, which was itself checked only in part when a
	// section of an NCA rewritten by a tool cannot be checked on its own
	return removeSource(o, path, check, name, note)
}

func decompress(ctx context.Context, path string, o options, out output) (string, string, error) {
	name, err := newFile(ctx, o, path, switchfs.DecompressedPath(path))
	if err != nil {
		return "", "", err
	}
	written := temporary(name)
	defer os.Remove(written)
	check, err := switchfs.DecompressAndCheck(ctx, path, written, out.progress(path, "Decompressing"))
	if err == nil {
		err = finish(o, written, name)
	}
	if err != nil {
		return "", "", err
	}
	return removeSource(o, path, check, name, modifiedNote(check))
}

func convert(ctx context.Context, path string, o options, out output) (string, string, error) {
	name, err := newFile(ctx, o, path, switchfs.ConvertedPath(path))
	if err != nil {
		return "", "", err
	}
	written := temporary(name)
	defer os.Remove(written)
	check, err := switchfs.ConvertAndCheck(ctx, path, written, out.progress(path, "Converting"))
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
		return skipped(keptInPart)
	}
	return commitFile(written, path)
}

// sameOrNewFile is the name of the file an action that works in place writes for path: path
// itself (inPlace), or a new file in the output folder, held for it in the work and free to
// write. The output folder can be the folder of path written another way: in place is told by
// the file, not by how its name is written.
func sameOrNewFile(ctx context.Context, o options, path string) (name string, inPlace bool, err error) {
	if name, err = target(o, path); err != nil {
		return "", false, err
	}
	if err = claim(ctx, path, name); err != nil {
		return "", false, err
	}
	if inPlace = sameFile(name, path); !inPlace {
		err = free(o, name)
	}
	return name, inPlace, err
}

// recompress writes an NSZ or XCZ again, solid or in blocks, at another level. Without an output
// folder the file is replaced, once its new copy is checked.
func recompress(ctx context.Context, path string, o options, threads int, out output) (string, string, error) {
	name, inPlace, err := sameOrNewFile(ctx, o, path)
	if err != nil {
		return "", "", err
	}
	written := temporary(name)
	defer os.Remove(written)
	result, err := switchfs.RecompressGame(ctx, path, written, switchfs.CompressOptions{
		Level: strconv.Itoa(o.level), Workers: threads, Blocks: o.block, Progress: out.progress(path, "Recompressing"),
	})
	if err != nil {
		return "", "", err
	}
	check, err := switchfs.VerifyFile(ctx, written, out.progress(path, "Verifying"))
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
	if o.block || strings.EqualFold(filepath.Ext(path), ".xcz") {
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

// trim takes away what installers do not use: the padding after the data of an XCI, or the
// delta fragments of the updates of an NSP or NSZ. In place, or into the output folder.
func trim(ctx context.Context, path string, o options, out output) (string, string, error) {
	if extension := strings.ToLower(filepath.Ext(path)); extension == ".nsp" || extension == ".nsz" {
		return stripDeltas(ctx, path, o, out)
	}
	name, inPlace, err := sameOrNewFile(ctx, o, path)
	if err != nil {
		return "", "", err
	}
	nothingToTrim := skipped("nothing to trim, it ends where its data ends")
	var (
		removed int64
		check   switchfs.FileCheck
	)
	if inPlace {
		// in place the file is cut: it is checked first, and cut only when checked whole
		if facts, err := switchfs.DescribeFile(path); err == nil && facts.Padding == 0 {
			return "", "", nothingToTrim
		}
		if check, err = switchfs.VerifyFile(ctx, path, out.progress(path, "Verifying")); err != nil {
			return "", "", err
		}
		if check.Partial {
			return "", "", skipped("it is left as it is: it could only be checked in part")
		}
		removed, err = switchfs.TrimXci(ctx, path, path, out.progress(path, "Trimming"))
	} else {
		written := temporary(name)
		defer os.Remove(written)
		removed, err = switchfs.TrimXci(ctx, path, written, out.progress(path, "Trimming"))
		if err == nil {
			check, err = switchfs.VerifyFile(ctx, written, out.progress(path, "Verifying"))
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
func stripDeltas(ctx context.Context, path string, o options, out output) (string, string, error) {
	deltas, err := switchfs.DeltaFragmentsSize(path)
	if err != nil {
		return "", "", err
	}
	if deltas == 0 {
		return "", "", skipped("nothing to trim, it holds no delta fragments: only updates have them")
	}
	name, inPlace, err := sameOrNewFile(ctx, o, path)
	if err != nil {
		return "", "", err
	}
	written := temporary(name)
	defer os.Remove(written)
	removed, check, err := switchfs.StripDeltasAndCheck(ctx, path, written, out.progress(path, "Trimming"))
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

// how much of a game is compressed to tell its estimate
const (
	estimateSamples    = 8
	estimateSampleSize = 16 << 20
)

// estimate tells the size and the time a compression would give, from samples of the game; with
// --levels, at several levels, to choose one.
func estimate(ctx context.Context, path string, o options, threads int, out output) (string, string, error) {
	if o.levels {
		return compareLevels(ctx, path, o, threads, out)
	}
	result, err := switchfs.EstimateCompression(ctx, path, switchfs.CompressOptions{
		Level: strconv.Itoa(o.level), Workers: threads, Blocks: o.block, Progress: out.progress(path, "Estimating"),
	}, estimateSamples, estimateSampleSize)
	if errors.Is(err, switchfs.ErrNothingToMeasure) {
		return nothingToEstimate(path)
	}
	if err != nil {
		return "", "", err
	}
	return "", fmt.Sprintf("about %s -> %s, %s smaller, %s at level %d", size(result.InputSize), size(result.OutputSize),
		percent(result.InputSize-result.OutputSize, result.InputSize), duration(result.Duration), o.level), nil
}

// nothingToEstimate ends the estimate of a game with nothing to compress: a game in sparse
// storage is skipped with why, as compressing skips it.
func nothingToEstimate(path string) (string, string, error) {
	if errors.Is(switchfs.NotCompressible(path), switchfs.ErrSparseStorage) {
		return "", "", skipped(sparseSkipped)
	}
	return "", "nothing in it can be compressed", nil
}

// the levels compared, from light to maximum
var comparedLevels = []int{3, 9, 12, 15, 18, 19}

// compareLevels tells the size and the time of a compression at several levels.
func compareLevels(ctx context.Context, path string, o options, threads int, out output) (string, string, error) {
	lines := []string{}
	progress := out.progress(path, "Estimating")
	for i, level := range comparedLevels {
		result, err := switchfs.EstimateCompression(ctx, path, switchfs.CompressOptions{
			Level: strconv.Itoa(level), Workers: threads, Blocks: o.block,
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
	out.say("%s:\n  %s", filepath.Base(path), strings.Join(lines, "\n  "))
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

// split writes each title of a multi-title file (each game of a game card with several) to a
// file of its own, named like the original with the title ID and version of the part.
func split(ctx context.Context, path string, o options, out output) (string, string, error) {
	plan, err := switchfs.PlanSplit(path)
	if err != nil {
		return "", "", err
	}
	if len(plan.Parts) < 2 {
		return "", "it holds one title, nothing to split", nil
	}
	base := strings.TrimSpace(strings.SplitN(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), "[", 2)[0])
	// every name is known before a part is written: none may be the file itself (a SuperNSP
	// named as its game is), nor the name of another part
	parts := make([]string, len(plan.Parts))
	for i, part := range plan.Parts {
		extension := ".nsp"
		switch {
		case part.Cartridge && part.Compressed:
			extension = ".xcz"
		case part.Cartridge:
			extension = ".xci"
		case part.Compressed:
			extension = ".nsz"
		}
		name, err := target(o, filepath.Join(filepath.Dir(path), fmt.Sprintf("%s [%s][v%d]%s", base, strings.ToUpper(part.TitleId), part.Version, extension)))
		if err != nil {
			return "", "", err
		}
		if sameFile(name, path) {
			return "", "", fmt.Errorf("its part %s would take the place of the file itself: rename the file first, or split it into another folder", filepath.Base(name))
		}
		for _, other := range parts[:i] {
			if strings.EqualFold(other, name) {
				return "", "", fmt.Errorf("two of its parts would get the same name: %s", filepath.Base(name))
			}
		}
		if err := claim(ctx, path, name); err != nil {
			return "", "", err
		}
		if err := free(o, name); err != nil {
			return "", "", err
		}
		parts[i] = name
	}
	// every part is written and checked under its hidden name, and only then do they all get
	// their names: the original is read whole before any file there is replaced
	var whole switchfs.FileCheck
	for i, part := range plan.Parts {
		written := temporary(parts[i])
		defer os.Remove(written)
		check, err := switchfs.WriteSplitPartAndCheck(ctx, path, part, written, out.progress(path, "Splitting "+part.Type))
		if err != nil {
			return "", "", err
		}
		whole.Partial = whole.Partial || check.Partial
	}
	names := []string{}
	for _, name := range parts {
		if err := finish(o, temporary(name), name); err != nil {
			return "", "", err
		}
		names = append(names, filepath.Base(name))
	}
	note := fmt.Sprintf("%d files: %s", len(names), strings.Join(names, ", "))
	if !plan.Complete() {
		// files of the original that no title lists: it is kept, whatever --rm-source says
		return "", note + "; some of its files are in no part, so it is kept", nil
	}
	_, note, err = removeSource(o, path, whole, "", note)
	return "", note, err
}

// pack merges the files given into one multi-title file (a SuperNSP), checked as it is
// written; the delta fragments of the updates are left out, as installers do not need them.
func pack(ctx context.Context, o options, out output) error {
	sources, missing := inputs(o.files, []string{".nsp", ".nsz", ".xci", ".xcz"})
	if len(missing) > 0 {
		return fmt.Errorf("not found: %s", strings.Join(missing, ", "))
	}
	// the file chosen for the pack is never one of its files: it would be written over while it
	// is read, then deleted. Given again when it is a pack made before of the others (the window
	// keeps the result in its list), it is left out; else it holds what the pack would lose
	chosen, err := filepath.Abs(o.pack)
	if err != nil {
		return err
	}
	given := sources
	sources = nil
	for _, source := range given {
		if source != chosen && !sameFile(source, chosen) {
			sources = append(sources, source)
		}
	}
	if len(sources) < len(given) && !coveredBy(chosen, sources) {
		return fmt.Errorf("the pack would take the name of %s, one of the files to combine", filepath.Base(chosen))
	}
	if len(sources) < 2 {
		return errors.New("a pack needs at least two files")
	}
	// the name is known before anything is written, where it was chosen (the output folder is
	// for the new files of the other actions): taken, the work does not start
	extension, err := packKind(sources)
	if err != nil {
		return err
	}
	name := strings.TrimSuffix(chosen, filepath.Ext(chosen)) + extension
	for _, source := range sources {
		if sameFile(source, name) {
			return fmt.Errorf("the pack would take the name of %s, one of the files to combine", filepath.Base(source))
		}
	}
	if err := free(o, name); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	// written in its folder, so its name is a rename on the same drive
	written := filepath.Join(filepath.Dir(name), fmt.Sprintf(".nxpress-pack-%d%s", time.Now().UnixNano(), extension))
	defer os.Remove(written)
	note, check, err := writePack(ctx, sources, written, o.pack, out)
	if err == nil {
		err = finish(o, written, name)
	}
	if err != nil {
		return err
	}
	kept := incompleteNote(o, check, name, sources)
	out.done(o.pack, name, joinNotes(keptNote(o, check, note), kept), nil)
	if kept != "" {
		return nil
	}
	for _, source := range sources {
		if _, _, err := removeSource(o, source, check, name, ""); err != nil {
			return err
		}
	}
	return nil
}

// coveredBy tells whether the others hold every title of a file, in its version or a newer one:
// a pack made of them before.
func coveredBy(file string, others []string) bool {
	titles, err := readTitles(file)
	if err != nil {
		return false
	}
	newest := map[string]int{}
	for _, other := range others {
		read, err := readTitles(other)
		if err != nil {
			return false
		}
		for _, title := range read {
			id := strings.ToLower(title.TitleId)
			if version, ok := newest[id]; !ok || title.Version > version {
				newest[id] = title.Version
			}
		}
	}
	for _, title := range titles {
		if version, ok := newest[strings.ToLower(title.TitleId)]; !ok || version < title.Version {
			return false
		}
	}
	return true
}

// incompleteNote is why the originals of a pack stay with --rm-source although it is checked:
// it lacks some of the titles it keeps. "" when it holds them whole, or nothing is deleted.
func incompleteNote(o options, check switchfs.FileCheck, pack string, sources []string) string {
	if !o.rmSource || check.Partial {
		return ""
	}
	lacks, err := packLacks(pack, sources)
	switch {
	case err != nil:
		return "the originals are kept: the pack cannot be read again: " + printable(err.Error())
	case len(lacks) > 0:
		return "the originals are kept: the pack lacks " + strings.Join(lacks, ", ")
	}
	return ""
}

func joinNotes(notes ...string) string {
	kept := []string{}
	for _, note := range notes {
		if note != "" {
			kept = append(kept, note)
		}
	}
	return strings.Join(kept, "; ")
}

// packLacks tells what a pack lacks of the titles it keeps: of each, the NCA files its CNMT
// names (but its delta fragments, left out on purpose), and a ticket when an original has one
// for it. The check of a pack compares each file it holds with the one it comes from, not
// what should be there: a file merged in and lost on the way (the ticket of an update kept,
// when a SuperNSP holds two versions of it under one name) would go with its originals.
func packLacks(pack string, sources []string) ([]string, error) {
	files, err := packageFiles(pack)
	if err != nil {
		return nil, err
	}
	held := map[string]bool{}
	tickets := map[string]bool{}
	for _, file := range files {
		name := strings.ToLower(file)
		held[name] = true
		// a ticket is named by its rights ID, the title ID first
		if strings.HasSuffix(name, ".tik") && len(name) > 16 {
			tickets[name[:16]] = true
		}
	}
	wanted := map[string]bool{}
	for _, source := range sources {
		files, err := packageFiles(source)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			if name := strings.ToLower(file); strings.HasSuffix(name, ".tik") && len(name) > 16 {
				wanted[name[:16]] = true
			}
		}
	}
	titles, err := readTitles(pack)
	if err != nil {
		return nil, err
	}
	sort.Slice(titles, func(i, j int) bool { return titles[i].TitleId < titles[j].TitleId })
	lacks := []string{}
	for _, title := range titles {
		id := strings.ToLower(title.TitleId)
		for _, kind := range contentOrder {
			content, ok := title.Contents[kind]
			if !ok || content.ID == "" || kind == "DeltaFragment" {
				continue
			}
			nca := strings.ToLower(content.ID)
			if !held[nca+".nca"] && !held[nca+".ncz"] && !held[nca+".cnmt.nca"] && !held[nca+".cnmt.ncz"] {
				lacks = append(lacks, fmt.Sprintf("the %s NCA of %s v%d", kind, strings.ToUpper(id), title.Version))
			}
		}
		if wanted[id] && !tickets[id] {
			lacks = append(lacks, "the ticket of "+strings.ToUpper(id))
		}
	}
	return lacks, nil
}

// packageFiles are the names of the files a game file holds: in its PFS0, or in the secure
// partition of a game card.
func packageFiles(path string) ([]string, error) {
	if extension := strings.ToLower(filepath.Ext(path)); extension != ".xci" && extension != ".xcz" {
		pfs0, err := switchfs.ReadPfs0File(path)
		if err != nil {
			return nil, err
		}
		names := make([]string, len(pfs0.Files))
		for i, file := range pfs0.Files {
			names[i] = file.Name
		}
		return names, nil
	}
	reader, err := switchfs.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	header := make([]byte, 0x200)
	if err := readAt(reader, header, 0); err != nil {
		return nil, err
	}
	if string(header[0x100:0x104]) != "HEAD" {
		return nil, errors.New("it is not a game card")
	}
	root := int64(binary.LittleEndian.Uint64(header[0x130:0x138]))
	partitions, err := hfs0Files(reader, root)
	if err != nil {
		return nil, err
	}
	for _, partition := range partitions {
		if partition.name != "secure" {
			continue
		}
		files, err := hfs0Files(reader, root+partition.offset)
		if err != nil {
			return nil, err
		}
		names := make([]string, len(files))
		for i, file := range files {
			names[i] = file.name
		}
		return names, nil
	}
	return nil, errors.New("the game card has no secure partition")
}

// hfs0File is a file of an HFS0, the file system of a game card, and where it starts.
type hfs0File struct {
	name   string
	offset int64
}

// hfs0Files reads the table of the HFS0 at offset: its files, each from the start of the HFS0.
func hfs0Files(reader io.ReaderAt, offset int64) ([]hfs0File, error) {
	head := make([]byte, 0x10)
	if err := readAt(reader, head, offset); err != nil {
		return nil, err
	}
	if string(head[:4]) != "HFS0" {
		return nil, errors.New("the game card has no HFS0 where its header says")
	}
	count := int64(binary.LittleEndian.Uint32(head[4:8]))
	names := int64(binary.LittleEndian.Uint32(head[8:12]))
	if count > 1<<16 || names > 4<<20 {
		return nil, errors.New("the HFS0 of the game card is damaged")
	}
	table := make([]byte, count*0x40+names)
	if err := readAt(reader, table, offset+0x10); err != nil {
		return nil, err
	}
	data := 0x10 + int64(len(table))
	files := make([]hfs0File, 0, count)
	for i := int64(0); i < count; i++ {
		entry := table[i*0x40:]
		at := int64(binary.LittleEndian.Uint32(entry[16:20]))
		if at >= names {
			return nil, errors.New("the HFS0 of the game card is damaged")
		}
		name := table[count*0x40+at:]
		if end := bytes.IndexByte(name, 0); end >= 0 {
			name = name[:end]
		}
		files = append(files, hfs0File{name: string(name), offset: data + int64(binary.LittleEndian.Uint64(entry[0:8]))})
	}
	return files, nil
}

// readAt reads all of p at offset: a read that ends at the end of the file is whole too.
func readAt(reader io.ReaderAt, p []byte, offset int64) error {
	n, err := reader.ReadAt(p, offset)
	if n == len(p) {
		return nil
	}
	if err == nil || err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return err
}

// defaultPackTemplate is the name of the pack of each game, as Switch Library Manager names a
// SuperNSP: Game [0100...][v65536] (1G+1U+2D)
const defaultPackTemplate = "{name} [{id}][v{version}] ({contents})"

// gameGroups puts the files of each game together, in the order the games come and with the
// game itself first; the files that cannot be read are given apart, with why.
func gameGroups(sources []string) ([]uint64, map[uint64][]string, map[string]error) {
	return groupGames(context.Background(), sources)
}

// groupGames is gameGroups that stops reading the files once ctx ends.
func groupGames(ctx context.Context, sources []string) ([]uint64, map[uint64][]string, map[string]error) {
	groups := map[uint64][]string{}
	order := []uint64{}
	first := map[string]int{}
	unread := map[string]error{}
	for _, source := range sources {
		if ctx.Err() != nil {
			break
		}
		titles, err := readTitles(source)
		if err != nil {
			unread[source] = err
			continue
		}
		main := describeTitles(titles)
		id := uint64(0)
		for _, title := range titles {
			if strings.EqualFold(title.TitleId, main.TitleID) {
				id, _ = switchfs.GameTitleID(title.TitleId, title.Type)
			}
		}
		if _, ok := groups[id]; !ok {
			order = append(order, id)
		}
		groups[id] = append(groups[id], source)
		first[source] = map[string]int{"BASE": 0, "UPD": 1, "DLC": 2}[main.Type]
	}
	for _, files := range groups {
		sort.SliceStable(files, func(i, j int) bool { return first[files[i]] < first[files[j]] })
	}
	return order, groups, unread
}

// packedAlready is a pack of the game id found where its pack would go: a file not given whose
// name has the title ID of the game and that holds more than one title. "" when there is none.
func packedAlready(o options, id uint64, files []string) string {
	folder, err := outputFolder(o)
	if err != nil {
		return ""
	}
	if folder == "" {
		folder = filepath.Dir(files[0])
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		return ""
	}
	given := map[string]bool{}
	for _, file := range files {
		given[strings.ToLower(filepath.Clean(file))] = true
	}
	tag := fmt.Sprintf("[%016X]", id)
	for _, entry := range entries {
		name := entry.Name()
		extension := strings.ToLower(filepath.Ext(name))
		if entry.IsDir() || !strings.Contains(strings.ToUpper(name), tag) || (extension != ".nsp" && extension != ".nsz" && extension != ".xci" && extension != ".xcz") {
			continue
		}
		path := filepath.Join(folder, name)
		if given[strings.ToLower(filepath.Clean(path))] {
			continue
		}
		if titles, err := readTitles(path); err == nil && len(titles) > 1 {
			return path
		}
	}
	return ""
}

// packGames packs the files given by game: each game with its updates and DLC in a SuperNSP of
// its own (into its game card, when it is one), named by o.packName, next to the files of the
// game or in -o. A game with one file only is skipped: there is nothing to pack it with. It
// gives how many files failed.
func packGames(ctx context.Context, o options, out output) int {
	sources, missing := inputs(o.files, []string{".nsp", ".nsz", ".xci", ".xcz"})
	failed := len(missing)
	for _, path := range missing {
		out.done(path, "", "", errors.New("no such file or folder"))
	}
	ctx = withDestinations(ctx, sources)
	order, groups, unread := groupGames(ctx, sources)
	for _, source := range sources {
		if err, ok := unread[source]; ok {
			out.done(source, "", "", shown(err))
			failed++
		}
	}
	template := o.packName
	if template == "" {
		template = defaultPackTemplate
	}
	for _, id := range order {
		waitWhilePaused(out)
		if ctx.Err() != nil {
			break
		}
		files := groups[id]
		// a game left out in the window, and a game packed before, unless asked to write again
		if o.skipGames[fmt.Sprintf("%016X", id)] {
			for _, file := range files {
				out.done(file, "", "", skipped("left out of the games to combine"))
			}
			continue
		}
		if packed := packedAlready(o, id, files); packed != "" && !o.overwrite {
			for _, file := range files {
				out.done(file, "", "", skipped("already combined in "+filepath.Base(packed)+" (-w writes it again)"))
			}
			continue
		}
		if len(files) == 1 {
			out.done(files[0], "", "", skipped("it is the only file of its game here: there is nothing to combine it with"))
			continue
		}
		name, note, check, err := packGame(ctx, o, files, template, out)
		err = shown(err)
		if isSkipped(err) {
			for _, file := range files {
				out.done(file, "", "", err)
			}
			continue
		}
		kept := ""
		if err == nil {
			kept = incompleteNote(o, check, name, files)
		}
		out.done(files[0], name, joinNotes(keptNote(o, check, note), kept), err)
		for _, file := range files[1:] {
			if err != nil {
				out.done(file, "", "", err)
				continue
			}
			out.done(file, "", "combined into "+filepath.Base(name), nil)
		}
		if err != nil {
			failed += len(files)
			continue
		}
		if kept != "" {
			continue
		}
		for _, file := range files {
			if _, _, err := removeSource(o, file, check, name, ""); err != nil {
				out.done(file, "", "", shown(err))
				failed++
			}
		}
	}
	return failed
}

// packGame writes the pack of the files of a game, named by the titles it holds and its size,
// next to the file of the game or in the output folder, and tells what it did. Its own pack,
// made before and given among its files, is told (skipped) unless -w writes it again; then
// the new pack takes its place only when it holds every title it keeps whole.
func packGame(ctx context.Context, o options, files []string, template string, out output) (string, string, switchfs.FileCheck, error) {
	extension, err := packKind(files)
	if err != nil {
		return "", "", switchfs.FileCheck{}, err
	}
	if again := packedAmong(o, files, template, extension); again != "" && !o.overwrite {
		return "", "", switchfs.FileCheck{}, skipped("already combined in " + filepath.Base(again) + " (-w writes it again)")
	}
	// written under a hidden name of its own first, in the folder of the pack: its name tells
	// its size, known once it is written
	folder := filepath.Dir(files[0])
	if o.output != "" {
		output, err := outputFolder(o)
		if err == nil {
			err = os.MkdirAll(output, 0o755)
		}
		if err != nil {
			return "", "", switchfs.FileCheck{}, err
		}
		folder = output
	}
	written := filepath.Join(folder, fmt.Sprintf(".nxpress-pack-%d%s", time.Now().UnixNano(), extension))
	defer os.Remove(written)
	note, check, err := writePack(ctx, files, written, files[0], out)
	if err != nil {
		return "", "", switchfs.FileCheck{}, err
	}
	info, err := os.Stat(written)
	if err != nil {
		return "", "", switchfs.FileCheck{}, err
	}
	name, err := packName(o, files, template, extension, info.Size())
	if err != nil {
		return "", "", switchfs.FileCheck{}, err
	}
	again := ""
	for _, file := range files {
		if sameFile(file, name) {
			again = file
		}
	}
	if again == "" {
		err = claim(ctx, files[0], name)
	} else if lacks, lackErr := packLacks(written, files); lackErr != nil || len(lacks) > 0 {
		// its own pack is written again only with all it holds
		err = lackErr
		if err == nil {
			err = fmt.Errorf("the new pack lacks %s: %s is kept as it is", strings.Join(lacks, ", "), filepath.Base(again))
		}
	}
	if err == nil {
		err = free(o, name)
	}
	if err == nil {
		err = finish(o, written, name)
	}
	if err != nil {
		return "", "", switchfs.FileCheck{}, err
	}
	return name, note, check, nil
}

// packName is the name of the pack of the files of a game: by the newest version of each
// title they hold and its size, next to the file of the game or in the output folder.
func packName(o options, files []string, template string, extension string, size int64) (string, error) {
	titles := []*switchfs.ContentMetaAttributes{}
	newest := map[string]*switchfs.ContentMetaAttributes{}
	for _, file := range files {
		read, err := readTitles(file)
		if err != nil {
			return "", err
		}
		for _, title := range read {
			if kept, ok := newest[strings.ToLower(title.TitleId)]; !ok || title.Version > kept.Version {
				newest[strings.ToLower(title.TitleId)] = title
			}
		}
	}
	for _, title := range newest {
		titles = append(titles, title)
	}
	title := describeTitles(titles)
	title.Size = size
	return target(o, filepath.Join(filepath.Dir(files[0]), strings.TrimSuffix(newName(files[0], title, template), filepath.Ext(files[0]))+extension))
}

// packedAmong is a file of a game that is its pack already, given with the others: one that
// holds more than one title, under the name the pack of them would get. "" when there is none.
func packedAmong(o options, files []string, template string, extension string) string {
	for _, file := range files {
		titles, err := readTitles(file)
		info, statErr := os.Stat(file)
		if err != nil || statErr != nil || len(titles) < 2 {
			continue
		}
		// with what it holds, its own size is the size of the pack
		if name, err := packName(o, files, template, extension, info.Size()); err == nil && sameFile(name, file) {
			return file
		}
	}
	return ""
}

// packKind is the extension of the pack of the files given: an NSP, an NSZ when one of them
// holds NCZ files, and with a game card among them an XCI (or an XCZ).
func packKind(sources []string) (string, error) {
	cards, _, err := splitCards(sources)
	if err != nil {
		return "", err
	}
	compressed := false
	for _, source := range sources {
		if isEmpty(source) {
			return "", fmt.Errorf("%s: %w", filepath.Base(source), errEmpty)
		}
		if facts, err := switchfs.DescribeFile(source); err != nil {
			return "", err
		} else if facts.Solid+facts.Blocks > 0 {
			compressed = true
		}
	}
	switch {
	case len(cards) == 1 && compressed:
		return ".xcz", nil
	case len(cards) == 1:
		return ".xci", nil
	case compressed:
		return ".nsz", nil
	}
	return ".nsp", nil
}

// splitCards puts the game card among the files apart: its updates and DLC go into it, and the
// pack is a game card.
func splitCards(sources []string) ([]string, []string, error) {
	cards := []string{}
	others := []string{}
	for _, source := range sources {
		if extension := strings.ToLower(filepath.Ext(source)); extension == ".xci" || extension == ".xcz" {
			cards = append(cards, source)
		} else {
			others = append(others, source)
		}
	}
	if len(cards) > 1 {
		return nil, nil, errors.New("only one game card can be packed: give its updates and DLC as NSP or NSZ")
	}
	return cards, others, nil
}

// writePack writes the pack of the files given to written, checked as it is written, and tells
// what it did. label names the work in its progress.
func writePack(ctx context.Context, sources []string, written string, label string, out output) (string, switchfs.FileCheck, error) {
	cards, others, err := splitCards(sources)
	if err != nil {
		return "", switchfs.FileCheck{}, err
	}
	// of a title given twice (an update in a SuperNSP, and a newer one), the newest is kept
	var (
		skipped  int64
		replaced []string
		check    switchfs.FileCheck
	)
	if len(cards) == 1 {
		replaced, check, err = switchfs.PackIntoCard(ctx, cards[0], others, written, out.progress(label, "Packing"))
	} else {
		skipped, replaced, check, err = switchfs.MergeNewestAndCheck(ctx, sources, written, true, out.progress(label, "Packing"))
	}
	if err != nil {
		return "", switchfs.FileCheck{}, err
	}
	note := fmt.Sprintf("%d files merged", len(sources))
	if skipped > 0 {
		note += fmt.Sprintf(", %s of delta fragments left out", size(skipped))
	}
	if len(replaced) > 0 {
		note += ", newer versions in place of " + strings.Join(replaced, ", ")
	}
	if modified := modifiedNote(check); modified != "" {
		note += ", " + modified
	}
	return note, check, nil
}

// info tells the titles a file holds; a file that cannot be read fails.
func info(path string, out output) (string, error) {
	read := switchfs.ReadNspMetadata
	if extension := strings.ToLower(filepath.Ext(path)); extension == ".xci" || extension == ".xcz" {
		read = switchfs.ReadXciMetadata
	}
	titles, err := read(path)
	if err != nil {
		return "", fmt.Errorf("cannot be read: %w", err)
	}
	ids := make([]string, 0, len(titles))
	for id := range titles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	lines := []string{}
	for _, id := range ids {
		title := titles[id]
		line := fmt.Sprintf("%s %s v%d", strings.ToUpper(title.TitleId), title.Type, title.Version)
		if title.Ncap != nil {
			if name := strings.TrimSpace(title.Ncap.TitleName["AmericanEnglish"].Title); name != "" {
				line += " " + name
			}
		}
		lines = append(lines, line)
		lines = append(lines, titleDetails(title)...)
	}
	lines = append(lines, describe(path)...)
	out.say("%s:\n  %s", filepath.Base(path), strings.Join(lines, "\n  "))
	if len(titles) == 1 {
		return "1 title", nil
	}
	return fmt.Sprintf("%d titles", len(titles)), nil
}

// shown is an error as people see it: the bytes of a damaged file quoted in it (what was read
// where a header should be) are shown as ?, not as the characters they happen to make.
func shown(err error) error {
	if err == nil {
		return nil
	}
	text := printable(err.Error())
	switch {
	case text == err.Error():
		return err
	case isSkipped(err):
		return skipped(text)
	}
	return errors.New(text)
}

// printable is text with each byte that is not a printable character as ?.
func printable(text string) string {
	return strings.Map(func(r rune) rune {
		if r == utf8.RuneError || !unicode.IsPrint(r) {
			return '?'
		}
		return r
	}, text)
}

// contentOrder is the order the NCA files of a title are told in.
var contentOrder = []string{"Meta", "Program", "Control", "Data", "HtmlDocument", "LegalInformation", "DeltaFragment"}

// titleDetails tells what a title needs, the firmware of the console for a game or an update
// and the version of its game for a DLC, and the NCA files it is made of (#20).
func titleDetails(title *switchfs.ContentMetaAttributes) []string {
	lines := []string{}
	switch {
	case title.RequiredTitleVersion > 0 && title.Type == "DLC":
		lines = append(lines, fmt.Sprintf("  needs version %d of its game", title.RequiredTitleVersion))
	case title.RequiredTitleVersion > 0:
		lines = append(lines, "  needs firmware "+firmware(uint32(title.RequiredTitleVersion)))
	}
	for _, kind := range contentOrder {
		if content, ok := title.Contents[kind]; ok && content.ID != "" {
			lines = append(lines, fmt.Sprintf("  %s NCA %s", kind, content.ID))
		}
	}
	return lines
}

// firmware is a version of the system as the console shows it: 12.1.0 for 0x30100000.
func firmware(version uint32) string {
	return fmt.Sprintf("%d.%d.%d", version>>26, version>>20&0x3F, version>>16&0xF)
}

// describe tells how a file is stored, what it works with, and what trimming takes away.
func describe(path string) []string {
	facts, err := switchfs.DescribeFile(path)
	if err != nil {
		return nil
	}
	lines := []string{}
	switch {
	case facts.Solid > 0 && facts.Blocks > 0:
		lines = append(lines, fmt.Sprintf("%s, solid and in blocks: %s, %s uncompressed (%s smaller)", facts.Kind, size(facts.Size), size(facts.DecompressedSize), percent(facts.DecompressedSize-facts.Size, facts.DecompressedSize)))
	case facts.Solid > 0:
		lines = append(lines, fmt.Sprintf("%s, solid: %s, %s uncompressed (%s smaller)", facts.Kind, size(facts.Size), size(facts.DecompressedSize), percent(facts.DecompressedSize-facts.Size, facts.DecompressedSize)))
	case facts.Blocks > 0:
		lines = append(lines, fmt.Sprintf("%s, in blocks: %s, %s uncompressed (%s smaller)", facts.Kind, size(facts.Size), size(facts.DecompressedSize), percent(facts.DecompressedSize-facts.Size, facts.DecompressedSize)))
	default:
		lines = append(lines, fmt.Sprintf("%s, uncompressed: %s", facts.Kind, size(facts.Size)))
	}
	switch {
	case facts.Solid > 0:
		lines = append(lines, "works with: every installer (DBI, Tinfoil, Awoo, CyberFoil, Sphaira); emulators need it decompressed")
	case facts.Blocks > 0:
		lines = append(lines, "works with: every installer but Awoo; emulators that read NSZ run it as it is")
	default:
		lines = append(lines, "works with: every installer and emulator")
	}
	if facts.Padding > 0 {
		lines = append(lines, fmt.Sprintf("padding: %s that trimming takes away", size(facts.Padding)))
	}
	if facts.DeltaFragments > 0 {
		lines = append(lines, fmt.Sprintf("delta fragments: %s that trimming takes away", size(facts.DeltaFragments)))
	}
	return lines
}

// removeSource deletes the original with --rm-source once its new file is checked whole, and
// gives back name and note. A file checked only in part (an NCA rewritten by a tool with a
// section that cannot be checked on its own) does not replace it: the original is kept, and the
// note says why. An original that is the new file itself, however its name is written, is
// never deleted.
func removeSource(o options, path string, check switchfs.FileCheck, name string, note string) (string, string, error) {
	if !o.rmSource {
		return name, note, nil
	}
	if check.Partial {
		return name, keptNote(o, check, note), nil
	}
	if name != "" && sameFile(path, name) {
		return name, note, nil
	}
	if err := os.Remove(path); err != nil {
		return name, note, fmt.Errorf("the new file is checked, but the original could not be deleted: %w", err)
	}
	return name, note, nil
}

// keptNote is note, with why the original stays when --rm-source cannot delete it.
func keptNote(o options, check switchfs.FileCheck, note string) string {
	if !o.rmSource || !check.Partial {
		return note
	}
	return joinNotes(note, keptInPart)
}

func modifiedNote(check switchfs.FileCheck) string {
	if len(check.Modified) == 0 {
		return ""
	}
	return fmt.Sprintf("%d NCA rewritten by a tool, their data checked", len(check.Modified))
}

func size(bytes int64) string {
	value, unit := float64(bytes), "B"
	for _, next := range []string{"KB", "MB", "GB", "TB"} {
		if value < 1024 {
			break
		}
		value, unit = value/1024, next
	}
	return fmt.Sprintf("%.1f %s", value, unit)
}

func percent(part int64, whole int64) string {
	if whole <= 0 {
		return "0%"
	}
	return fmt.Sprintf("%.1f%%", float64(part)*100/float64(whole))
}
