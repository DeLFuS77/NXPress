package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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
	// the computer does not go to sleep while files are at work
	defer keepAwake()()
	action := o.action()
	if action == "pack" && o.packGames {
		return packGames(ctx, o, out)
	}
	if action == "pack" {
		if err := pack(ctx, o, out); err != nil {
			out.done(o.pack, "", "", err)
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
	// the measure tries its settings on the first game alone
	// the measure tries its settings on the first game that can be compressed
	if action == "measure" {
		path, err := measurableGame(ctx, files, o)
		output, note := "", ""
		if err == nil {
			output, note, err = measure(ctx, path, o, out)
		}
		out.done(path, output, note, err)
		if err != nil && !isSkipped(err) {
			return len(missing) + 1
		}
		return len(missing)
	}
	if o.dat != "" && o.datFile == nil {
		loaded, err := loadDat(o.dat)
		if err != nil {
			out.done(o.dat, "", "", err)
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
				output, note, err := doJob(ctx, action, path, o, threads, out)
				// every new file is checked before it gets its name: it need not be read again
				if err == nil && output != "" && action != "rename" {
					checked.keep(output, "")
				}
				out.done(path, output, note, err)
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
// extensions taken, each once and in order of name; missing are the paths not found. A folder
// of parts is one file, taken only with splitFiles.
func inputs(paths []string, extensions []string) (files []string, missing []string) {
	seen := map[string]bool{}
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
		if !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
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
		found := []string{}
		filepath.WalkDir(path, func(walked string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				// a folder of parts is one file: its parts are not looked into
				if _, ok := splitContainer(walked); ok && walked != path {
					if takes(walked) {
						found = append(found, walked)
					}
					return filepath.SkipDir
				}
				return nil
			}
			if takes(walked) {
				found = append(found, walked)
			}
			return nil
		})
		sort.Strings(found)
		for _, walked := range found {
			add(walked)
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

// finish gives the checked temporary file its name, replacing a file there with -w.
func finish(o options, written string, path string) error {
	if err := syncPath(written); err != nil {
		return err
	}
	if o.overwrite {
		os.Remove(path)
	}
	return commitFile(written, path)
}

// free tells whether a new file can be written at path.
func free(o options, path string) error {
	if _, err := os.Stat(path); err == nil && !o.overwrite {
		return fmt.Errorf("%s is already there (use -w to replace it)", filepath.Base(path))
	}
	return nil
}

func doJob(ctx context.Context, action string, path string, o options, threads int, out output) (string, string, error) {
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
		return "", info(path, out), nil
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
	return removeSource(o, path, switchfs.FileCheck{}, name, note)
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
		err = commitFile(written, path)
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
	written := path
	if !inPlace {
		written = temporary(name)
		defer os.Remove(written)
	}
	removed, err := switchfs.TrimXci(ctx, path, written, out.progress(path, "Trimming"))
	if errors.Is(err, switchfs.ErrNothingToTrim) {
		return "", "", skipped("nothing to trim, it ends where its data ends")
	}
	if err != nil {
		return "", "", err
	}
	check, err := switchfs.VerifyFile(ctx, written, out.progress(path, "Verifying"))
	if err != nil {
		return "", "", err
	}
	if !inPlace {
		if err := finish(o, written, name); err != nil {
			return "", "", err
		}
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
		err = commitFile(written, path)
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
		return "", "nothing in it can be compressed", nil
	}
	if err != nil {
		return "", "", err
	}
	return "", fmt.Sprintf("about %s -> %s, %s smaller, %s at level %d", size(result.InputSize), size(result.OutputSize),
		percent(result.InputSize-result.OutputSize, result.InputSize), duration(result.Duration), o.level), nil
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
			return "", "nothing in it can be compressed", nil
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
	names := []string{}
	var checked switchfs.FileCheck
	for i, part := range plan.Parts {
		name := parts[i]
		written := temporary(name)
		var check switchfs.FileCheck
		check, err = switchfs.WriteSplitPartAndCheck(ctx, path, part, written, out.progress(path, "Splitting "+part.Type))
		checked.Partial = checked.Partial || check.Partial
		if err == nil {
			err = finish(o, written, name)
		}
		os.Remove(written)
		if err != nil {
			return "", "", err
		}
		names = append(names, filepath.Base(name))
	}
	note := fmt.Sprintf("%d files: %s", len(names), strings.Join(names, ", "))
	if !plan.Complete() {
		// files of the original that no title lists: it is kept, whatever --rm-source says
		return "", note + "; some of its files are in no part, so it is kept", nil
	}
	_, note, err = removeSource(o, path, checked, "", note)
	return "", note, err
}

// pack merges the files given into one multi-title file (a SuperNSP), checked as it is
// written; the delta fragments of the updates are left out, as installers do not need them.
func pack(ctx context.Context, o options, out output) error {
	sources, missing := inputs(o.files, []string{".nsp", ".nsz", ".xci", ".xcz"})
	if len(missing) > 0 {
		return fmt.Errorf("not found: %s", strings.Join(missing, ", "))
	}
	if len(sources) < 2 {
		return errors.New("a pack needs at least two files")
	}
	name, note, check, err := packFiles(ctx, o, sources, o.pack, func(extension string, _ int64) (string, error) {
		return target(o, strings.TrimSuffix(o.pack, filepath.Ext(o.pack))+extension)
	}, out)
	if err != nil {
		return err
	}
	out.done(o.pack, name, keptNote(o, check, note), nil)
	for _, source := range sources {
		if _, _, err := removeSource(o, source, check, name, ""); err != nil {
			return err
		}
	}
	return nil
}

// defaultPackTemplate is the name of the pack of each game, as Switch Library Manager names a
// SuperNSP: Game [0100...][v65536] (1G+1U+2D)
const defaultPackTemplate = "{name} [{id}][v{version}] ({contents})"

// packGames packs the files given by game: each game with its updates and DLC in a SuperNSP of
// its own (into its game card, when it is one), named by o.packName, next to the files of the
// game or in -o. A game with one file only is skipped: there is nothing to pack it with. It
// gives how many files failed.
// gameGroups puts the files of each game together, in the order the games come and with the
// game itself first; the files that cannot be read are given apart, with why.
func gameGroups(sources []string) ([]uint64, map[uint64][]string, map[string]error) {
	groups := map[uint64][]string{}
	order := []uint64{}
	first := map[string]int{}
	unread := map[string]error{}
	for _, source := range sources {
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

func packGames(ctx context.Context, o options, out output) int {
	sources, missing := inputs(o.files, []string{".nsp", ".nsz", ".xci", ".xcz"})
	failed := len(missing)
	for _, path := range missing {
		out.done(path, "", "", errors.New("no such file or folder"))
	}
	ctx = withDestinations(ctx, sources)
	order, groups, unread := gameGroups(sources)
	for _, source := range sources {
		if err, ok := unread[source]; ok {
			out.done(source, "", "", err)
			failed++
		}
	}
	template := o.packName
	if template == "" {
		template = defaultPackTemplate
	}
	for _, id := range order {
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
		// the pack is named by the titles it holds, next to the file of the game
		name, note, check, err := packFiles(ctx, o, files, files[0], func(extension string, size int64) (string, error) {
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
			name, err := target(o, filepath.Join(filepath.Dir(files[0]), strings.TrimSuffix(newName(files[0], title, template), filepath.Ext(files[0]))+extension))
			if err == nil {
				err = claim(ctx, files[0], name)
			}
			return name, err
		}, out)
		out.done(files[0], name, keptNote(o, check, note), err)
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
		for _, file := range files {
			if _, _, err := removeSource(o, file, check, name, ""); err != nil {
				out.done(file, "", "", err)
				failed++
			}
		}
	}
	return failed
}

// packFiles writes the pack of the files given, checked, under the name nameOf gives for its
// extension and its size, and tells what it did. label names the work in its progress.
func packFiles(ctx context.Context, o options, sources []string, label string, nameOf func(extension string, size int64) (string, error), out output) (string, string, switchfs.FileCheck, error) {
	// a game card among them: its updates and DLC go into it, and the pack is a game card
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
		return "", "", switchfs.FileCheck{}, errors.New("only one game card can be packed: give its updates and DLC as NSP or NSZ")
	}
	// a pack of NSP files is an NSP, uncompressed; one with an NSZ among them holds NCZ, an NSZ
	compressed := false
	for _, source := range sources {
		if facts, err := switchfs.DescribeFile(source); err != nil {
			return "", "", switchfs.FileCheck{}, err
		} else if facts.Solid+facts.Blocks > 0 {
			compressed = true
		}
	}
	extension := ".nsp"
	switch {
	case len(cards) == 1 && compressed:
		extension = ".xcz"
	case len(cards) == 1:
		extension = ".xci"
	case compressed:
		extension = ".nsz"
	}
	// written under a hidden name of its own first: the name of a pack per game tells its size,
	// known once it is written
	folder := filepath.Dir(sources[0])
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
	// of a title given twice (an update in a SuperNSP, and a newer one), the newest is kept
	var (
		skipped  int64
		replaced []string
		check    switchfs.FileCheck
	)
	var err error
	if len(cards) == 1 {
		replaced, check, err = switchfs.PackIntoCard(ctx, cards[0], others, written, out.progress(label, "Packing"))
	} else {
		skipped, replaced, check, err = switchfs.MergeNewestAndCheck(ctx, sources, written, true, out.progress(label, "Packing"))
	}
	name := ""
	if err == nil {
		var info os.FileInfo
		if info, err = os.Stat(written); err == nil {
			name, err = nameOf(extension, info.Size())
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
	return name, note, check, nil
}

// info tells the titles a file holds.
func info(path string, out output) string {
	read := switchfs.ReadNspMetadata
	if extension := strings.ToLower(filepath.Ext(path)); extension == ".xci" || extension == ".xcz" {
		read = switchfs.ReadXciMetadata
	}
	titles, err := read(path)
	if err != nil {
		return "cannot be read: " + err.Error()
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
		return "1 title"
	}
	return fmt.Sprintf("%d titles", len(titles))
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
	kept := "the original is kept: the new file could only be checked in part"
	if note == "" {
		return kept
	}
	return note + "; " + kept
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
