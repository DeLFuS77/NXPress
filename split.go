package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

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
