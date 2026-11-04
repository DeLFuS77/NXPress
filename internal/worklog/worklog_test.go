package worklog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func work(when int64, text string) Work {
	return Work{When: when, Mode: "compress", Status: "1 of 1 done", Rows: []Row{{Name: "game.nsp", Status: "ok"}}, Text: text}
}

func TestAddKeepsTheWorksInBothFiles(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "NXPress")
	if _, err := Add(folder, work(1, "first work\n"), 0); err != nil {
		t.Fatal(err)
	}
	works, err := Add(folder, work(2, "second work"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 2 || works[0].When != 1 || works[1].When != 2 {
		t.Fatalf("the works must be kept the oldest first: %+v", works)
	}
	if read := Read(folder); len(read) != 2 || read[1].Rows[0].Name != "game.nsp" {
		t.Fatalf("the list must be read back: %+v", read)
	}
	text, err := os.ReadFile(filepath.Join(folder, TextName))
	if err != nil {
		t.Fatal(err)
	}
	if string(text) != "first work\n\nsecond work\n" {
		t.Fatalf("the text file must hold each work, a line between them: %q", text)
	}
	if names, _ := filepath.Glob(filepath.Join(folder, ".*")); len(names) != 0 {
		t.Fatalf("no hidden file must be left: %v", names)
	}
}

func TestAddLeavesOutTheOldest(t *testing.T) {
	folder := t.TempDir()
	for when := int64(1); when <= 5; when++ {
		if _, err := Add(folder, work(when, "work"), 3); err != nil {
			t.Fatal(err)
		}
	}
	works := Read(folder)
	if len(works) != 3 || works[0].When != 3 || works[2].When != 5 {
		t.Fatalf("only the 3 newest must be kept: %+v", works)
	}
	text, _ := os.ReadFile(filepath.Join(folder, TextName))
	if strings.Count(string(text), "work") != 3 {
		t.Fatalf("the text file must keep as many works: %q", text)
	}
}

func TestTrim(t *testing.T) {
	works := []Work{work(1, ""), work(2, ""), work(3, "")}
	if kept := Trim(works, 0); len(kept) != 3 {
		t.Fatal("0 keeps them all")
	}
	if kept := Trim(works, 5); len(kept) != 3 {
		t.Fatal("fewer than the most are all kept")
	}
	if kept := Trim(works, 1); len(kept) != 1 || kept[0].When != 3 {
		t.Fatalf("the newest must be kept: %+v", kept)
	}
}

func TestClearAndWriteBack(t *testing.T) {
	folder := t.TempDir()
	works, _ := Add(folder, work(1, "work"), 0)
	if err := Clear(folder); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{ListName, TextName} {
		if _, err := os.Stat(filepath.Join(folder, name)); !os.IsNotExist(err) {
			t.Fatalf("%s must be gone", name)
		}
	}
	if err := Clear(folder); err != nil {
		t.Fatal("a log cleared twice is no error")
	}
	if len(Read(folder)) != 0 {
		t.Fatal("a cleared log has no works")
	}
	// what Undo does: the works the window had are written back
	if err := Write(folder, works); err != nil {
		t.Fatal(err)
	}
	if read := Read(folder); len(read) != 1 {
		t.Fatalf("the works must be back: %+v", read)
	}
	if err := Write(folder, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(folder, TextName)); !os.IsNotExist(err) {
		t.Fatal("writing no works removes the log")
	}
}

func TestASpoiltLogIsEmpty(t *testing.T) {
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, ListName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if works := Read(folder); works == nil || len(works) != 0 {
		t.Fatalf("a spoilt log must read as empty: %+v", works)
	}
	// the next work starts it again
	if works, err := Add(folder, work(1, "work"), 0); err != nil || len(works) != 1 {
		t.Fatalf("a work must start a spoilt log again: %v %+v", err, works)
	}
}
