package main

import (
	"context"
	"crypto/sha256"
	"path/filepath"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// The estimate of a game in sparse storage skips it with why, as compressing does, at one
// level and at several.
func TestEstimateOfASparseGameIsSkipped(t *testing.T) {
	safetyIsolate(t)
	program := safetyProgramNcaWith(safetyGameData(0x10000, 5), func(header []byte) {
		// the table of a sparse section has a size
		header[0x400+0x150] = 0x10
		fsHash := sha256.Sum256(header[0x400:0x600])
		copy(header[0x280:], fsHash[:])
	})
	path := safetyNspWith(t, filepath.Join(t.TempDir(), "sparse.nsp"), safetyBaseID, 0, switchfs.ContentMetaType_Application, program, safetyContentName(program, ".nca"), nil)
	for _, levels := range []bool{false, true} {
		_, note, err := estimate(context.Background(), path, options{estimate: true, levels: levels, level: 3}, 1, quiet{})
		if !isSkipped(err) || err.Error() != sparseSkipped {
			t.Errorf("levels %v: %q %v", levels, note, err)
		}
	}
}
