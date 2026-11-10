package jobs

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/cache"
	"github.com/DeLFuS77/NXPress/internal/nup"
	"github.com/DeLFuS77/NXPress/internal/system"
	"github.com/DeLFuS77/NXPress/internal/titledb"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// NUP files (.nup) hold decrypted games of the Switch and the Switch 2, a game with its updates
// and its DLC, as plain files: NXPress shows what they hold and verifies them, makes them of dump
// folders, turns them back into folders and renames them by what they hold, without keys. The
// other works leave them as they are, and say so.

// NupNotDone is why a NUP given to any other work is left as it is.
const NupNotDone = "this work leaves NUP files as they are: Combine (--nup) makes them of dump folders, compressed with zstd, Decompress (-D) turns them back into folders, Details (-i) shows what they hold, Verify (-V) checks them and Rename (--rename) renames them"

// nupActions are the works that take NUP files.
var nupActions = map[string]bool{"verify": true, "info": true, "decompress": true, "rename": true, "fixmeta": true, "fixicon": true}

// TakesNup tells whether a work takes NUP files.
func TakesNup(action string) bool {
	return nupActions[action]
}

// isNup tells a NUP by the extension of its name.
func isNup(path string) bool {
	return nup.IsNup(path)
}

// NupFiles are the NUP files among the paths given and in the folders given (and in theirs).
func NupFiles(paths []string) []string {
	files, _ := Inputs(paths, []string{".nup"})
	return files
}

// NeedsKeys tells whether a work needs the keys of the console: not to recompress, to split for
// FAT32 or to join, nor for NUP files and dump folders, which hold their games decrypted. A work
// given NUP files or dump folders only needs none: it packs them, unpacks, verifies or shows
// them, or tells it leaves them as they are.
func NeedsKeys(o Options) bool {
	// with the extra compression, a file is recompressed through a decompressed copy (#92)
	if o.Recompress {
		return o.Sparse || o.Archive
	}
	if o.Fat32 || o.Nup {
		return false
	}
	if o.Watch {
		return true
	}
	taken := Takes[o.Action()]
	if o.Action() == "pack" {
		taken = []string{".nsp", ".nsz", ".xci", ".xcz"}
	}
	files, _ := Inputs(o.Files, append(append([]string{}, taken...), ".nup", dumpFolders))
	if len(files) == 0 {
		return true
	}
	for _, path := range files {
		if !isNup(path) && !IsDumpFolder(path) {
			return true
		}
	}
	return false
}

// nupWorkers are the pieces of data of one NUP hashed and decompressed at once: its share of the
// processors among the files worked on at once.
func nupWorkers(o Options) int {
	return max(1, switchfs.UsableProcessors()/max(1, o.Multi))
}

// nupReaders are the readers that go through a NUP, or the files of a dump folder, at once: one
// on a hard disk, which reads fastest in order, and on a drive that does not tell what it is;
// several on an SSD, which reads faster so.
func nupReaders(path string, workers int) int {
	if penalty, known := system.SeekPenalty(path); known && !penalty {
		return max(1, min(4, workers))
	}
	return 1
}

// nupChecker is the version of the check of a NUP the cache keeps: 1 reads the code modules of
// its ExeFS by their headers too. A NUP checked by an older version is read again.
const nupChecker = 1

// what to do with a game whose code is damaged: its hashes, made of the dump as it was, do not
// tell it, and the game stops while it launches
const (
	codeDamagedNup  = "the code of the game is damaged in the dump this NUP was made of: dump the game again"
	codeDamagedDump = "the code of the game is damaged in this dump folder: dump the game again"
	// a dump folder has nothing else to check it by
	dumpNoHashes = "the other files of a dump folder have no hashes to be checked by"
	// a NUP made of a dump with its code damaged is kept, as the dump is
	nupMadeAnyway = "the NUP was made all the same, as the dump is"
)

// codeOnly tells damaged files that are all code modules whose data is as it was packed.
func codeOnly(damaged []nup.Damaged) bool {
	for _, d := range damaged {
		if !d.Code {
			return false
		}
	}
	return len(damaged) > 0
}

// codeDamaged is the error of damaged code modules: what to do, then the first of them by their
// package and path, and why.
func codeDamaged(head string, damaged []nup.Damaged) error {
	parts := []string{}
	for i, d := range damaged {
		if i == damagedInError {
			parts = append(parts, fmt.Sprintf("and %d more", len(damaged)-damagedInError))
			break
		}
		parts = append(parts, d.Package+": "+d.Path+": "+d.Reason)
	}
	return fmt.Errorf("%s (%s)", head, strings.Join(parts, "; "))
}

// modulesNote tells the code modules read by their headers.
func modulesNote(modules int) string {
	if modules == 1 {
		return "1 code module (NSO) checked by its header"
	}
	return fmt.Sprintf("%d code modules (NSO) checked by their headers", modules)
}

// verifyNup checks a NUP: the hash of its table of contents, then the SHA-256 of every file,
// decompressed when it is stored with zstd, and the code modules of each ExeFS by their headers;
// each damaged file is told by its path. A NUP holds no signatures of Nintendo, and is in no
// DAT: only its own hashes are checked.
func verifyNup(ctx context.Context, path string, o Options, out Output) (string, string, error) {
	extra := []string{}
	if o.VerifyLevel == "signature" || o.VerifyLevel == "both" {
		extra = append(extra, "a NUP holds no signatures of Nintendo")
	}
	if o.datFile != nil {
		extra = append(extra, "not compared with the DAT, which lists no NUP files")
	}
	if entry, ok := cache.Checked.Find(path); ok && !o.NoCache && entry.Checker >= nupChecker {
		note := "checked on " + entry.Checked.Format("2006-01-02") + ", not changed since"
		if entry.Note != "" {
			note += "; " + entry.Note
		}
		if container, err := nup.Open(path); err == nil {
			extra = append(extra, emulatorsNote(container))
		}
		return "", joinNotes(append([]string{note}, extra...)...), nil
	}
	workers := nupWorkers(o)
	container, check, err := nup.Verify(ctx, path, workers, nupReaders(path, workers), out.Progress(path, "Verifying"))
	if err != nil {
		if nup.EmulatorsRefuse(err) {
			err = fmt.Errorf("%w; %s", err, emulatorsRefuseToo)
		}
		return "", "", err
	}
	if codeOnly(check.Damaged) {
		return "", "", codeDamaged(codeDamagedNup, check.Damaged)
	}
	if len(check.Damaged) > 0 {
		return "", "", damagedNup(path, check, out)
	}
	note := fmt.Sprintf("%d files checked by their SHA-256", check.Files)
	if check.Files == 1 {
		note = "1 file checked by its SHA-256"
	}
	if check.Unplaced > 0 {
		note += fmt.Sprintf(" (%d in no folder of any package)", check.Unplaced)
	}
	notes := []string{note}
	if check.Modules > 0 {
		notes = append(notes, modulesNote(check.Modules))
	}
	note = joinNotes(append(notes, check.Notes...)...)
	cache.Checked.KeepChecked(path, note, nupChecker)
	return "", joinNotes(append(append([]string{note}, extra...), emulatorsNote(container))...), nil
}

// FixNupMeta writes again the metadata of a NUP whose types are counted from 0 with them where the
// format puts them, as Eglantine and Lichii read them; a NUP counted from 1 is skipped.
func FixNupMeta(path string) (string, error) {
	fixed, err := nup.FixZeroBased(path)
	switch {
	case err != nil:
		return "", err
	case fixed == 0:
		return "", Skipped(countedFromOne)
	}
	return fmt.Sprintf("its metadata is counted from 1 now, as Eglantine and Lichii read it: %s written again, the files of the game as they were", count(fixed, "package", "packages")), nil
}

// countedFromOne is why --fix-nup-meta leaves a NUP as it is.
const countedFromOne = "its metadata is counted from 1 already: nothing to write again"

// emulatorsRefuseToo is said of a NUP NXPress does not read for a reason Eglantine and Lichii
// refuse it too.
const emulatorsRefuseToo = "Eglantine and Lichii do not open it either"

// emulatorsNote tells, in Verify and Details, whether Eglantine and Lichii open a NUP and can
// start its game, and why not: they are stricter than the format in a few things.
func emulatorsNote(container *nup.Container) string {
	emulators := container.Emulators()
	notes := []string{}
	switch {
	case len(emulators.Refused) > 0:
		notes = append(notes, "Eglantine and Lichii do not open it: "+strings.Join(emulators.Refused, "; "))
	case emulators.NoStart != "":
		notes = append(notes, "Eglantine and Lichii cannot start it: "+emulators.NoStart)
	}
	if emulators.ZeroBased {
		notes = append(notes, zeroBasedNote)
	}
	if len(notes) == 0 {
		return "it meets what Eglantine and Lichii ask of a NUP"
	}
	return joinNotes(notes...)
}

// zeroBasedNote is said of a NUP whose metadata has its types counted from 0, as a build of the
// packer of the format wrote them: NXPress reads them where the format puts them, the emulators
// do not.
const zeroBasedNote = "Eglantine and Lichii show its name, version and icon wrong: the types of its metadata are counted from 0, as a build of the packer of the format wrote them"

// verifyDump checks the code of a dump folder: every code module of the ExeFS of each package by
// its header, as its loader reads it. The other files of a dump have no hashes to be checked by,
// and a dump holds no signatures of Nintendo and is in no DAT.
func verifyDump(ctx context.Context, path string, o Options, out Output) (string, string, error) {
	extra := []string{dumpNoHashes}
	if o.VerifyLevel == "signature" || o.VerifyLevel == "both" {
		extra = append(extra, "a dump folder holds no signatures of Nintendo")
	}
	if o.datFile != nil {
		extra = append(extra, "not compared with the DAT, which lists no dump folders")
	}
	check, err := nup.CheckDumpCode(ctx, path)
	if err != nil {
		return "", "", err
	}
	if len(check.Damaged) > 0 {
		return "", "", codeDamaged(codeDamagedDump, check.Damaged)
	}
	if check.Modules == 0 {
		return "", "", Skipped("it holds no code module (NSO) to check, and " + dumpNoHashes)
	}
	return "", joinNotes(append([]string{modulesNote(check.Modules)}, extra...)...), nil
}

// the damaged files a failed NUP tells in its error, and the ones told on lines of their own
const (
	damagedInError = 5
	damagedTold    = 200
)

// damagedNup is the error of a NUP with damaged files: how many, and the first of them by their
// path; every damaged file (up to damagedTold) is told on a line of its own before it.
func damagedNup(path string, check nup.Check, out Output) error {
	where := func(d nup.Damaged) string {
		if d.Package == "" {
			return d.Path + " (in no folder)"
		}
		return d.Package + ": " + d.Path
	}
	if len(check.Damaged) > 1 {
		lines := []string{}
		for i, d := range check.Damaged {
			if i == damagedTold {
				lines = append(lines, fmt.Sprintf("and %d more", len(check.Damaged)-damagedTold))
				break
			}
			lines = append(lines, where(d)+": "+d.Reason)
		}
		out.Say("%s: damaged files:\n  %s", filepath.Base(path), strings.Join(lines, "\n  "))
	}
	first := []string{}
	for i, d := range check.Damaged {
		if i == damagedInError {
			first = append(first, fmt.Sprintf("and %d more", len(check.Damaged)-damagedInError))
			break
		}
		first = append(first, where(d)+" ("+d.Reason+")")
	}
	return fmt.Errorf("%d of %d files are damaged: %s", len(check.Damaged), check.Files, strings.Join(first, "; "))
}

// count is a number of things, as "1 file" or "12 files".
func count(n int, one string, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// infoNup tells what a NUP holds: each package (the game, an update, a DLC) with its title ID,
// its version, the name and the publisher it gives, its files and their size as stored and
// uncompressed, then the whole file.
func infoNup(path string, versions *titledb.Versions, out Output) (string, error) {
	container, err := nup.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot be read: %w", err)
	}
	lines := []string{}
	for i, pkg := range container.Packages {
		line := fmt.Sprintf("%s %s v%d", pkg.TitleIDText(), pkg.KindName(), pkg.Version)
		if name := pkg.Name(); name != "" {
			line += " " + name
		}
		lines = append(lines, line)
		if shown := pkg.DisplayVersion(); shown != "" {
			lines = append(lines, "  version "+shown)
		}
		if publisher := pkg.Publisher(); publisher != "" {
			lines = append(lines, "  publisher: "+publisher)
		}
		if published := nupPublished(pkg, versions); published != "" {
			lines = append(lines, published)
		}
		summary, err := container.Summarize(i)
		if err != nil {
			return "", fmt.Errorf("cannot be read: %w", err)
		}
		lines = append(lines, "  "+nupSizes(summary))
	}
	totals := container.Totals()
	lines = append(lines, fmt.Sprintf("NUP: %s, %s of files uncompressed, %s as stored", size(container.FileSize),
		size(int64(totals.Size)), size(int64(totals.Stored))))
	lines = append(lines, emulatorsNote(container))
	lines = append(lines, "works with: programs that read NUP files; NXPress makes it of a dump folder (Combine, --nup) and turns it back into one (Decompress, -D)")
	out.Say("%s:\n  %s", filepath.Base(path), strings.Join(lines, "\n  "))
	return count(len(container.Packages), "title", "titles"), nil
}

// nupSizes tells the files of a package and their size: "7 files: 3.2 MB, 1.1 MB as stored".
func nupSizes(summary nup.Summary) string {
	return fmt.Sprintf("%s: %s, %s as stored", count(summary.Files, "file", "files"), size(int64(summary.Size)), size(int64(summary.Stored)))
}

// nupPublished tells the newest version published of a package, when the versions kept know it.
func nupPublished(pkg nup.Package, versions *titledb.Versions) string {
	id, err := strconv.ParseUint(pkg.TitleIDText(), 16, 64)
	if err != nil || versions == nil {
		return ""
	}
	what := "update"
	switch pkg.Kind {
	case nup.KindUpdate:
		id &^= 0xFFF
	case nup.KindAddon:
		what = "version"
	}
	published, ok := versions.Newest(id)
	if !ok {
		return ""
	}
	line := fmt.Sprintf("  newest %s published: v%d", what, published.Version)
	if published.Date != "" {
		line += " (" + published.Date + ")"
	}
	return line
}

// nupTitle is the main title of a NUP, as ReadTitle gives the one of a game file: the game, else
// its newest update, else a DLC, with its name and its icon when the file carries them.
func nupTitle(path string, withIcon bool) (GameTitle, error) {
	container, err := nup.Open(path)
	if err != nil {
		return GameTitle{}, err
	}
	main := -1
	for _, kind := range []byte{nup.KindBase, nup.KindUpdate, nup.KindAddon} {
		for i, pkg := range container.Packages {
			if pkg.Kind == kind && (main < 0 || pkg.Version > container.Packages[main].Version) {
				main = i
			}
		}
		if main >= 0 {
			break
		}
	}
	if main < 0 {
		return GameTitle{}, fmt.Errorf("it holds no title")
	}
	first := container.Packages[main]
	result := GameTitle{TitleID: first.TitleIDText(), Version: int(first.Version), Type: first.KindName(),
		Titles: len(container.Packages), Latest: int(first.Version), Size: container.FileSize, Container: "NUP"}
	shown := first
	for _, pkg := range container.Packages {
		switch pkg.Kind {
		case nup.KindBase:
			result.Bases++
		case nup.KindUpdate:
			result.Updates++
			if first.Kind == nup.KindBase && int(pkg.Version) > result.Latest {
				result.Latest, shown = int(pkg.Version), pkg
			}
		case nup.KindAddon:
			result.Dlcs++
		}
	}
	result.VersionTxt = shown.DisplayVersion()
	// the name and the publisher of the game are in its package, an update may give its own
	for _, pkg := range []nup.Package{first, shown} {
		if result.Name == "" {
			result.Name = pkg.Name()
		}
		if result.Publisher == "" {
			result.Publisher = pkg.Publisher()
		}
	}
	// a NUP made of a dump without its control data gives no name: the list of the games of the
	// Switch 2 names it, else the label of its dump made readable, as Combine names it now
	// (NintendoSwitchSportsResort_v1.1.0 is Nintendo Switch Sports Resort), so Rename and Details
	// do not take the name of its file as it is
	if result.Name == "" && first.Kind != nup.KindAddon {
		result.Name = listName(nup.Source{TitleID: first.TitleID, Kind: first.Kind})
	}
	if result.Name == "" {
		for _, pkg := range []nup.Package{first, shown} {
			if name := dumpNameOf(pkg.SourceDump()); name != "" {
				result.Name = name
				break
			}
		}
	}
	if first.Kind != nup.KindBase {
		kind := map[byte]string{nup.KindUpdate: "UPD", nup.KindAddon: "DLC"}[first.Kind]
		if game, err := switchfs.GameTitleID(first.TitleIDText(), kind); err == nil {
			result.GameID = fmt.Sprintf("%016X", game)
		}
	}
	if withIcon {
		if icon := container.Icon(); icon != nil {
			kind := "image/jpeg"
			if bytes.HasPrefix(icon, []byte("\x89PNG")) {
				kind = "image/png"
			}
			result.Icon = "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(icon)
		}
	}
	return result, nil
}

// NupPackage is what the card of a NUP in Details tells of one of its packages.
type NupPackage struct {
	TitleID   string `json:"titleId"`
	Type      string `json:"type"`
	Version   int    `json:"version"`
	Shown     string `json:"shown,omitempty"`
	Name      string `json:"name,omitempty"`
	Publisher string `json:"publisher,omitempty"`
	Files     int    `json:"files"`
	Size      int64  `json:"size"`
	Stored    int64  `json:"stored"`
}

// NupDetails is what the card of a NUP in Details tells: the size of the file, the size of its
// files uncompressed, its packages, and whether Eglantine and Lichii open it (a note).
type NupDetails struct {
	Size         int64
	Uncompressed int64
	Packages     []NupPackage
	Emulators    string
	// Eglantine and Lichii open it, start its game and show it right; its metadata is counted
	// from 0, which --fix-nup-meta writes again
	EmulatorsOK bool
	ZeroBased   bool
	// the icon it carries for its game, nil when it carries none (--fix-nup-icon gives it one)
	Icon []byte
	// it carries the icons of its game in each language, which are kept: no other is given it
	GameIcons bool
}

// NupCard tells what a NUP holds, for its card in Details.
func NupCard(path string) (NupDetails, error) {
	container, err := nup.Open(path)
	if err != nil {
		return NupDetails{}, err
	}
	emulators := container.Emulators()
	result := NupDetails{Size: container.FileSize, Uncompressed: int64(container.Totals().Size), Emulators: emulatorsNote(container),
		EmulatorsOK: len(emulators.Refused) == 0 && emulators.NoStart == "" && !emulators.ZeroBased, ZeroBased: emulators.ZeroBased,
		Icon: container.Icon(), GameIcons: container.GameIcons()}
	for i, pkg := range container.Packages {
		summary, err := container.Summarize(i)
		if err != nil {
			return NupDetails{}, err
		}
		result.Packages = append(result.Packages, NupPackage{TitleID: pkg.TitleIDText(), Type: pkg.KindName(), Version: int(pkg.Version),
			Shown: pkg.DisplayVersion(), Name: pkg.Name(), Publisher: pkg.Publisher(), Files: summary.Files,
			Size: int64(summary.Size), Stored: int64(summary.Stored)})
	}
	return result, nil
}

// tellNupsLeft tells, for a work that does not take NUP files, each one given or in a folder
// given: left as it is, and why. A watched folder tells nothing, round after round.
func tellNupsLeft(o Options, out Output) int {
	if TakesNup(o.Action()) || o.Watch || o.Action() == "nup" {
		return 0
	}
	files := NupFiles(o.Files)
	for _, path := range files {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			out.Done(path, "", "", Skipped(NupNotDone))
		}
	}
	return len(files)
}
