package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"
	"github.com/DeLFuS77/NXPress/internal/worklog"
)

// The log of the works the page keeps (#69): in the folder of the settings, the oldest left
// out past the most, the setting lowered, cleared and put back.
func TestLogOfTheWorks(t *testing.T) {
	testkit.Isolate(t)
	app := &App{}
	if works := app.Log(); works == nil || len(works) != 0 {
		t.Fatalf("there is no log yet: %+v", works)
	}
	if err := app.OpenLog(); err == nil {
		t.Fatal("an empty log cannot be opened")
	}
	for when := int64(1); when <= 3; when++ {
		if _, err := app.AddToLog(worklog.Work{When: when, Mode: "compress", Rows: []worklog.Row{{Name: "game.nsp", Status: "ok"}}, Text: "work"}, 2); err != nil {
			t.Fatal(err)
		}
	}
	works := app.Log()
	if len(works) != 2 || works[0].When != 2 || works[1].When != 3 {
		t.Fatalf("the 2 newest must be kept: %+v", works)
	}
	if filepath.Base(app.LogFile()) != "log.txt" || filepath.Base(filepath.Dir(app.LogFile())) != "NXPress" {
		t.Fatalf("the text file must be log.txt in the folder of the settings: %s", app.LogFile())
	}
	if _, err := os.Stat(app.LogFile()); err != nil {
		t.Fatal(err)
	}
	if kept, err := app.KeepLog(1); err != nil || len(kept) != 1 || kept[0].When != 3 {
		t.Fatalf("lowering the most keeps the newest: %v %+v", err, kept)
	}
	if kept, err := app.KeepLog(0); err != nil || len(kept) != 1 {
		t.Fatalf("no limit leaves the log as it is: %v %+v", err, kept)
	}
	if err := app.ClearLog(); err != nil {
		t.Fatal(err)
	}
	if len(app.Log()) != 0 {
		t.Fatal("a cleared log has no works")
	}
	if err := app.WriteLog(works); err != nil || len(app.Log()) != 2 {
		t.Fatalf("Undo must put the works back: %v", err)
	}
}
