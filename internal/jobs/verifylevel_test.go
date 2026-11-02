package jobs

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// --verify-level takes hash, signature or both, and nothing else; full, the name of hash before
// 1.4.0, is still taken (#42).
func TestVerifyLevelOption(t *testing.T) {
	for given, level := range map[string]string{"hash": "full", "full": "full", "signature": "signature", "both": "both"} {
		o, err := ParseOptions([]string{"-V", "--verify-level", given, "game.nsp"})
		if err != nil || o.VerifyLevel != level {
			t.Errorf("%s: %q %v", given, o.VerifyLevel, err)
		}
	}
	if _, err := ParseOptions([]string{"-V", "--verify-level", "quick", "game.nsp"}); err == nil {
		t.Error("an unknown level is taken")
	}
}

// verifyGames writes a game whose NCA files are whole but not signed by Nintendo, as every
// synthetic one, and a game rewritten by a tool and then damaged.
func verifyGames(t *testing.T) (string, string) {
	t.Helper()
	folder := t.TempDir()
	unsigned := testkit.TitleNsp(t, filepath.Join(folder, "unsigned.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	nca, name := testkit.RewrittenNca()
	// a byte of the data its hash tree checks
	nca[len(nca)-0x100] ^= 0xFF
	damaged := testkit.NspWith(t, filepath.Join(folder, "damaged.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, nca, name, nil)
	return unsigned, damaged
}

// A file whose NCA files are not signed by Nintendo while its data is whole is modified, not
// failed, by the signature alone or by both; one whose data is damaged fails by its hashes, and
// by both, which reads its data even after the signature (#42).
func TestVerifyTellsModifiedFromDamaged(t *testing.T) {
	testkit.Isolate(t)
	unsigned, damaged := verifyGames(t)
	verify := func(path string, level string) (string, error) {
		_, note, err := doJob(context.Background(), "verify", path, Options{VerifyLevel: level, NoCache: true}, 1, testkit.Quiet{})
		return note, err
	}
	for _, level := range []string{"signature", "both"} {
		if _, err := verify(unsigned, level); !IsModified(err) || isFailure(err) || !strings.HasPrefix(err.Error(), "2 of 2 NCA not signed by Nintendo") {
			t.Errorf("%s, a file only unsigned: %v", level, err)
		}
	}
	if note, err := verify(unsigned, "full"); err != nil {
		t.Errorf("hash, a file only unsigned: %q %v", note, err)
	}
	// the signature alone does not read the data
	if _, err := verify(damaged, "signature"); !IsModified(err) {
		t.Errorf("signature, a damaged file: %v", err)
	}
	for _, level := range []string{"full", "both"} {
		if _, err := verify(damaged, level); !isFailure(err) || strings.Contains(err.Error(), "not signed") {
			t.Errorf("%s, a damaged file: %v", level, err)
		}
	}
}

// The command line tells a modified file as MODIFIED, and so to other programs, and only the
// damaged one counts as failed.
func TestVerifyModifiedOnTheCommandLine(t *testing.T) {
	testkit.Isolate(t)
	unsigned, damaged := verifyGames(t)
	o := Options{Verify: true, VerifyLevel: "both", NoCache: true, Files: []string{unsigned, damaged}, Multi: 1, Threads: 1}
	var text bytes.Buffer
	if failed := RunJobs(context.Background(), o, &Reporter{Out: &text}); failed != 1 {
		t.Fatalf("%d failed: %s", failed, text.String())
	}
	if !strings.Contains(text.String(), "MODIFIED unsigned.nsp: 2 of 2 NCA not signed by Nintendo") || !strings.Contains(text.String(), "FAILED damaged.nsp: ") {
		t.Fatal(text.String())
	}
	text.Reset()
	o.Files = []string{unsigned}
	if failed := RunJobs(context.Background(), o, &Reporter{Out: &text, Machine: true}); failed != 0 || !strings.Contains(text.String(), `"result": "modified"`) {
		t.Fatalf("%d failed: %s", failed, text.String())
	}
}
