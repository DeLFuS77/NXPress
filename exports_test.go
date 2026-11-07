package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/export"
	"github.com/DeLFuS77/NXPress/internal/testkit"
	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// Details exports a row for each title of each file, in the order of the files, with the kind
// in the words of the page; a file that cannot be read gets one row that tells why (#89).
func TestDetailsRows(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	update := testkit.TitleNsp(t, filepath.Join(folder, "update.nsp"), testkit.UpdateID, 65536, switchfs.ContentMetaType_Patch, 2, nil)
	game := testkit.TitleNsp(t, filepath.Join(folder, "game.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	broken := filepath.Join(folder, "broken.nsz")
	if err := os.WriteFile(broken, []byte("not an NSZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows := detailsRows([]string{update, broken, game}, nil, map[string]string{"BASE": "Game", "UPD": "Update"})
	if len(rows) != 3 {
		t.Fatalf("%d rows: %v", len(rows), rows)
	}
	for i, row := range rows {
		if len(row) != 12 {
			t.Fatalf("row %d has %d cells", i, len(row))
		}
		if row[10] != folder {
			t.Errorf("row %d: folder %v", i, row[10])
		}
	}
	if rows[0][0] != "update.nsp" || rows[0][1] != "NSP" || rows[0][4] != "Update" || rows[0][5] != int64(65536) || rows[0][11] != nil {
		t.Errorf("update: %v", rows[0])
	}
	if size, ok := rows[0][2].(export.Bytes); !ok || size <= 0 {
		t.Errorf("update size: %v", rows[0][2])
	}
	if problem, _ := rows[1][11].(string); rows[1][0] != "broken.nsz" || rows[1][3] != nil || problem == "" {
		t.Errorf("broken: %v", rows[1])
	}
	if rows[2][4] != "Game" || rows[2][5] != int64(0) || !strings.EqualFold(rows[2][3].(string), "0100000000010000") {
		t.Errorf("game: %v", rows[2])
	}
}

// A name proposed for a file holds no character a system refuses.
func TestSafeName(t *testing.T) {
	for name, want := range map[string]string{
		"NXPress details 2026-11-07": "NXPress details 2026-11-07",
		`a/b\c:d*e?f"g<h>i|j`:        "a b c d e f g h i j",
		"  ":                         "NXPress",
	} {
		if got := safeName(name); got != want {
			t.Errorf("%q: %q", name, got)
		}
	}
}
