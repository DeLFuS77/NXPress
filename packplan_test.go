package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The packs by game, before they are made: one game for each base game of the folder, none
// packed yet. It needs games of the user, so it runs only where they are.
func TestPackPlanOfRealGames(t *testing.T) {
	folder := os.Getenv("NXPRESS_TEST_GAMES")
	if folder == "" {
		t.Skip("NXPRESS_TEST_GAMES is not set")
	}
	if _, err := loadKeys(""); err != nil {
		t.Skip("no prod.keys")
	}
	files, _ := filepath.Glob(filepath.Join(folder, "*.ns*"))
	plan := (&App{}).PackPlan(files, t.TempDir())
	if len(plan) == 0 {
		t.Fatal("no game")
	}
	for _, game := range plan {
		t.Logf("%s %s %d files packed=%q", game.ID, game.Name, len(game.Files), game.Packed)
		if game.Packed != "" {
			t.Errorf("%s is told packed in an empty folder", game.Name)
		}
	}
}
