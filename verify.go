package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// verify checks a file at the level asked. The signature of Nintendo on every NCA is read from
// the headers only (#9); NCA files not signed by it make the file modified, not failed (#42):
// with both, its data is checked all the same, and only damage makes it fail.
func verify(ctx context.Context, path string, o options, out output) (string, string, error) {
	signed := ""
	var unsigned error
	if o.verifyLevel == "signature" || o.verifyLevel == "both" {
		note, err := checkSignatures(path)
		switch {
		case isModified(err) && o.verifyLevel == "both":
			unsigned = err
		case err != nil || o.verifyLevel == "signature":
			return "", note, err
		default:
			signed = note
		}
	}
	note, err := verifyData(ctx, path, o, out, signed)
	if err == nil && unsigned != nil {
		return "", "", modified(joinNotes(unsigned.Error(), note))
	}
	return "", note, err
}

// verifyData checks the data of a file by its hashes (and against the DAT given), or tells it
// was checked before; signed, what the check of the signatures found, is told with it.
func verifyData(ctx context.Context, path string, o options, out output, signed string) (string, error) {
	// a file checked before and not changed since is not read again, unless asked
	if entry, ok := checked.find(path); ok && !o.noCache && o.datFile == nil {
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
		if found, whole, err = o.datFile.matchDump(ctx, path, out.progress(path, "Comparing")); err != nil {
			return "", err
		}
		if whole {
			checked.keep(path, "")
			return joinNotes(signed, found), nil
		}
	}
	check, err := switchfs.VerifyFile(ctx, path, out.progress(path, "Verifying"))
	note := modifiedNote(check)
	if err == nil {
		checked.keep(path, note)
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

// sparseSkipped is why a game in sparse storage is left as it is: it installs as it is, and only
// the experimental format NCZSECT2, asked with --sparse or under Advanced, compresses it.
const sparseSkipped = "this game uses sparse storage: it installs as it is, and only the experimental format NCZSECT2, an option under Advanced, can compress it"
