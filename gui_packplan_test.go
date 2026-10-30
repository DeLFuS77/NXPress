package main

import (
	"context"
	"testing"
)

// The plan of Combine shows a game as combined already when its pack, made before, comes among
// the files given under the name it would get again, as the work then tells it.
func TestPackPlanTellsAPackAmongTheFiles(t *testing.T) {
	safetyIsolate(t)
	folder := t.TempDir()
	safetyGame(t, folder)
	if failed := packGames(context.Background(), options{packGames: true, files: []string{folder}}, quiet{}); failed != 0 {
		t.Fatalf("first run: %d failed", failed)
	}
	plan := (&App{}).PackPlan([]string{folder}, "", "")
	if len(plan) != 1 || plan[0].Packed == "" {
		t.Fatalf("plan %+v", plan)
	}
}
