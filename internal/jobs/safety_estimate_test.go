package jobs

import (
	"context"
	"crypto/sha256"
	"path/filepath"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// The estimate of a game in sparse storage skips it with why, as compressing does, at one
// level and at several.
func TestEstimateOfASparseGameIsSkipped(t *testing.T) {
	testkit.Isolate(t)
	program := testkit.ProgramNcaWith(testkit.GameData(0x10000, 5), func(header []byte) {
		// the table of a sparse section has a size
		header[0x400+0x150] = 0x10
		fsHash := sha256.Sum256(header[0x400:0x600])
		copy(header[0x280:], fsHash[:])
	})
	path := testkit.NspWith(t, filepath.Join(t.TempDir(), "sparse.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, program, testkit.ContentName(program, ".nca"), nil)
	for _, levels := range []bool{false, true} {
		_, note, err := estimate(context.Background(), path, Options{Estimate: true, Levels: levels, Level: 3}, 1, testkit.Quiet{})
		if !IsSkipped(err) || err.Error() != sparseSkipped {
			t.Errorf("levels %v: %q %v", levels, note, err)
		}
	}
}
