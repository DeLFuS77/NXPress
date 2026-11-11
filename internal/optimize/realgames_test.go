//go:build cgo

package optimize

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dtrunk90/switch-library-manager-web/settings"
)

// TestRealGames optimizes real games, when given: NXPRESS_OPTIMIZE_GAMES lists games separated by
// "|", each its files separated by ";" (a base and its update, NSP, NSZ, XCI or XCZ), and
// NXPRESS_KEYS the prod.keys (default ~/.switch/prod.keys). Each NSP made is checked whole by Run
// (CheckMerged), then removed; the sizes are logged. NXPRESS_OPTIMIZE_KEEP keeps them in that
// folder instead.
func TestRealGames(t *testing.T) {
	games := os.Getenv("NXPRESS_OPTIMIZE_GAMES")
	if games == "" {
		t.Skip("no real games given (NXPRESS_OPTIMIZE_GAMES)")
	}
	keys := os.Getenv("NXPRESS_KEYS")
	if keys == "" {
		home, _ := os.UserHomeDir()
		keys = filepath.Join(home, ".switch", "prod.keys")
	}
	if err := settings.UseSwitchKeys(keys); err != nil {
		t.Fatalf("keys: %v", err)
	}
	folder := os.Getenv("NXPRESS_OPTIMIZE_KEEP")
	if folder == "" {
		folder = t.TempDir()
	}
	for _, game := range strings.Split(games, "|") {
		files := strings.Split(game, ";")
		name := filepath.Base(files[0])
		t.Run(name, func(t *testing.T) {
			target := filepath.Join(folder, strings.TrimSuffix(name, filepath.Ext(name))+" (optimized).nsp")
			os.Remove(target)
			start := time.Now()
			last := ""
			res, err := Run(context.Background(), files, target, Options{Threads: 8, Progress: func(step string, done, total int64) {
				if step != last {
					t.Logf("%6.0f s  %s", time.Since(start).Seconds(), step)
					last = step
				}
			}})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%016X v%d: program %.1f MB (merged NCA %d bytes, sha256 %s), base and update installed %.1f MB, NSP %.1f MB; RomFS %.1f MB, runs again %.1f MB, blocks again %.1f MB; %.0f s",
				res.TitleID, res.Version, float64(res.ProgramAfter)/1e6, res.ProgramAfter, res.ProgramHash, float64(res.InputSize)/1e6, float64(res.OutputSize)/1e6,
				float64(res.RomfsSize)/1e6, float64(res.RunsAgain)/1e6, float64(res.BlocksAgain)/1e6, time.Since(start).Seconds())
			if os.Getenv("NXPRESS_OPTIMIZE_KEEP") == "" {
				os.Remove(target)
			}
		})
	}
}
