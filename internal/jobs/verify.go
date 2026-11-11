package jobs

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/cache"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// verify checks a file at the level asked. The signature of Nintendo on every NCA is read from
// the headers only (#9); NCA files not signed by it make the file modified, not failed (#42):
// with both, its data is checked all the same, and only damage makes it fail.
func verify(ctx context.Context, path string, o Options, out Output) (string, string, error) {
	signed := ""
	var unsigned error
	if o.VerifyLevel == "signature" || o.VerifyLevel == "both" {
		note, err := checkSignatures(path)
		switch {
		case IsModified(err) && o.VerifyLevel == "both":
			unsigned = err
		case err != nil || o.VerifyLevel == "signature":
			return "", note, err
		default:
			signed = note
		}
	}
	note, err := verifyData(ctx, path, o, out, signed)
	if err == nil && unsigned != nil {
		return "", "", modified(joinNotes(unsigned.Error(), note))
	}
	// a CNMT that lists a content the file does not hold: the data can be sound, and installers
	// such as Sphaira still do not find it
	if missing := UnlistedContents(path); len(missing) > 0 && (err == nil || IsModified(err)) {
		before := note
		if err != nil {
			before = err.Error()
		}
		return "", "", modified(joinNotes(MissingContentsNote(missing), before))
	}
	return "", note, err
}

// UnlistedContents are the content IDs the CNMT of an NSP or NSZ lists that no file in it has as
// its name (.nca or .ncz), delta fragments left aside; none for an XCI or a file not read.
func UnlistedContents(path string) []string {
	if extension := strings.ToLower(filepath.Ext(path)); extension != ".nsp" && extension != ".nsz" {
		return nil
	}
	pfs0, err := switchfs.ReadPfs0File(path)
	if err != nil {
		return nil
	}
	titles, err := switchfs.ReadNspMetadata(path)
	if err != nil {
		return nil
	}
	names := map[string]bool{}
	for _, file := range pfs0.Files {
		name := strings.ToLower(file.Name)
		for _, suffix := range []string{".cnmt.nca", ".cnmt.ncz", ".nca", ".ncz"} {
			if strings.HasSuffix(name, suffix) {
				names[strings.TrimSuffix(name, suffix)] = true
				break
			}
		}
	}
	missing := []string{}
	for _, title := range titles {
		for kind, content := range title.Contents {
			if kind == "DeltaFragment" || kind == "Meta" || content.ID == "" {
				continue
			}
			if !names[strings.ToLower(content.ID)] {
				missing = append(missing, strings.ToLower(content.ID))
			}
		}
	}
	sort.Strings(missing)
	return missing
}

// MissingContentsNote tells the contents a CNMT lists that the file does not hold.
func MissingContentsNote(missing []string) string {
	return fmt.Sprintf("its CNMT lists %d content not in the file, so installers such as Sphaira do not find it; Rebuild (--rebuild) makes it right: %s", len(missing), strings.Join(missing, ", "))
}

// verifyData checks the data of a file by its hashes (and against the DAT given), or tells it
// was checked before; signed, what the check of the signatures found, is told with it.
func verifyData(ctx context.Context, path string, o Options, out Output, signed string) (string, error) {
	// a file checked before and not changed since is not read again, unless asked
	if entry, ok := cache.Checked.Find(path); ok && !o.NoCache && o.datFile == nil {
		note := "checked on " + entry.Checked.Format("2006-01-02") + ", not changed since"
		if entry.Note != "" {
			note += "; " + entry.Note
		}
		return note, nil
	}
	// with a DAT, a file it lists by a hash of the whole file is that very dump: the one reading
	// that compares it tells so, without reading it again to check its NCA files
	found := ""
	if o.datFile != nil {
		var whole bool
		var err error
		if found, whole, err = o.datFile.MatchDump(ctx, path, out.Progress(path, "Comparing")); err != nil {
			return "", err
		}
		if whole {
			cache.Checked.Keep(path, "")
			return joinNotes(signed, found), nil
		}
	}
	check, err := switchfs.VerifyFile(ctx, path, out.Progress(path, "Verifying"))
	note := modifiedNote(check)
	if err == nil {
		cache.Checked.Keep(path, note)
	}
	note = joinNotes(note, signed)
	if err != nil || o.datFile == nil {
		return note, err
	}
	return joinNotes(note, found), nil
}

// checkSignatures tells whether every NCA of a file is signed by Nintendo: the ones that are
// not, as a tool that rewrote them leaves them, make the file modified.
func checkSignatures(path string) (string, error) {
	check, err := switchfs.CheckSignatures(path)
	if err != nil {
		return "", err
	}
	if len(check.Unsigned) > 0 {
		return "", modified(fmt.Sprintf("%d of %d NCA not signed by Nintendo (rewritten by a tool): %s", len(check.Unsigned), check.Checked, strings.Join(check.Unsigned, ", ")))
	}
	note := fmt.Sprintf("%d NCA signed by Nintendo", check.Checked)
	if len(check.Unknown) > 0 {
		note += fmt.Sprintf("; %d signed with a key not known yet", len(check.Unknown))
	}
	return note, nil
}

// sparseSkipped is why a game in sparse storage is left as it is, and how to compress it: only
// the experimental format NCZSECT2 does, asked with --extra (or --sparse) or under Made for.
const sparseSkipped = "this game uses sparse storage: to compress it with the experimental format of NXPress (NCZSECT2), turn on Extra compression under Made for, or use --extra"
