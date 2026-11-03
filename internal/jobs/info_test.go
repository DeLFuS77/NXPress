package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/titledb"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// Info tells the firmware a game needs, the version of its game a DLC needs, and its NCA files.
func TestTitleDetails(t *testing.T) {
	if got := firmware(0x0C100000); got != "3.1.0" {
		t.Errorf("firmware %s", got)
	}
	if got := firmware(805306368); got != "12.0.0" {
		t.Errorf("firmware %s", got)
	}
	game := &switchfs.ContentMetaAttributes{Type: "BASE", RequiredTitleVersion: 805306368, Contents: map[string]switchfs.Content{"Program": {ID: "aa"}, "Meta": {ID: "bb"}}}
	if got := strings.Join(titleDetails(game), "|"); got != "  needs firmware 12.0.0|  Meta NCA bb|  Program NCA aa" {
		t.Errorf("game: %q", got)
	}
	dlc := &switchfs.ContentMetaAttributes{Type: "DLC", RequiredTitleVersion: 65536}
	if got := strings.Join(titleDetails(dlc), "|"); got != "  needs version 65536 of its game" {
		t.Errorf("DLC: %q", got)
	}
}

// Info tells the newest version published beside each title, when the versions are kept: the
// newest update of its game for a game or an update, its own for a DLC; nothing for a title the
// list does not know, nor with no versions.
func TestPublishedLine(t *testing.T) {
	folder := t.TempDir()
	list := `{"0100a1b2c3d40000":{"65536":"2026-01-10","393216":"2026-10-21"},"0100a1b2c3d41001":{"65536":"2026-08-12"}}`
	if err := os.WriteFile(filepath.Join(folder, titledb.FileName), []byte(list), 0o644); err != nil {
		t.Fatal(err)
	}
	versions, err := titledb.Load(folder)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		id, kind, want string
	}{
		{"0100A1B2C3D40000", "BASE", "  newest update published: v393216 (2026-10-21)"},
		{"0100a1b2c3d40800", "UPD", "  newest update published: v393216 (2026-10-21)"},
		{"0100A1B2C3D41001", "DLC", "  newest version published: v65536 (2026-08-12)"},
		{"0100A1B2C3D41002", "DLC", ""},
		{"0100E6F5D4C30000", "BASE", ""},
	} {
		title := &switchfs.ContentMetaAttributes{TitleId: c.id, Type: c.kind}
		if got := publishedLine(title, versions); got != c.want {
			t.Errorf("%s %s: %q, want %q", c.id, c.kind, got, c.want)
		}
		if got := publishedLine(title, nil); got != "" {
			t.Errorf("%s %s with no versions: %q", c.id, c.kind, got)
		}
	}
}
