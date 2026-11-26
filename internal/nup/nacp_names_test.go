package nup

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testnup"
	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// compressedNacp is a NACP that keeps its names compressed, as the console does: each entry
// given (its index, then its name and publisher), and the version shown.
func compressedNacp(t *testing.T, names map[int][2]string, version string) []byte {
	t.Helper()
	entries := make([]byte, 32*nacpEntry)
	for i, entry := range names {
		copy(entries[i*nacpEntry:], entry[0])
		copy(entries[i*nacpEntry+0x200:], entry[1])
	}
	nacp := make([]byte, nacpSize)
	nacp[nacpCompressedFlag] = 1
	if err := packNacpTitles(nacp, entries); err != nil {
		t.Fatal(err)
	}
	copy(nacp[nacpVersionAt:], version)
	return nacp
}

// compressedDump is a dump of a game and an update whose NACPs keep their names compressed;
// broken gives the update a NACP whose compressed names do not decompress.
func compressedDump(t *testing.T, broken bool) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "dump")
	names := map[int][2]string{0: {"Made-up Quest", "Nobody Games"}, 2: {"Meido Kuesuto", "Nobadi"}, 16: {"Zmyslona Gra", "Nikt"}}
	update := compressedNacp(t, names, "1.0.1")
	if broken {
		// the name written over the compressed names, as a rename of before did
		clear(update[:0x200])
		copy(update, "Renamed Quest")
	}
	writeTree(t, root, map[string][]byte{
		"base/program/0/exefs/main":                       nso(1),
		"base/program/0/exefs/main.npdm":                  npdm(0x040074A01BF12000),
		"base/program/0/romfs/a.bin":                      []byte("a"),
		"base/program/0/control/control.nacp":             compressedNacp(t, names, "1.0.0"),
		"base/program/0/control/icon_AmericanEnglish.dat": testnup.Jpeg,
		"base/program/0/meta/x.cnmt":                      []byte("cnmt"),
		"updates/65536/program/0/exefs/main":              nso(2),
		"updates/65536/program/0/exefs/main.npdm":         npdm(0x040074A01BF12000),
		"updates/65536/program/0/romfs/a.bin":             []byte("b"),
		"updates/65536/program/0/control/control.nacp":    update,
	})
	return root
}

// nacpNames are the names and publishers of the entries of a NACP that have one, by index, as
// the library app reads them.
func nacpNames(t *testing.T, nacp []byte) map[int][2]string {
	t.Helper()
	entries, count, err := switchfs.NacpTitleEntries(nacp)
	if err != nil {
		t.Fatal(err)
	}
	result := map[int][2]string{}
	for i := 0; i < count; i++ {
		name, publisher := string(cString(entries[i*nacpEntry:][:0x200])), string(cString(entries[i*nacpEntry+0x200:][:0x100]))
		if name != "" || publisher != "" {
			result[i] = [2]string{name, publisher}
		}
	}
	return result
}

// The update of a game whose NACP keeps its names compressed gives its name and publisher, read
// decompressed, and carries them as records, for a reader that starts from the newest update.
func TestPackCompressedNacpUpdate(t *testing.T) {
	out, _ := pack(t, compressedDump(t, false), PackOptions{Level: 3, Workers: 2})
	container, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(container.Packages) != 2 {
		t.Fatalf("%d packages", len(container.Packages))
	}
	for i, pkg := range container.Packages {
		if pkg.Name() != "Made-up Quest" || pkg.Publisher() != "Nobody Games" || pkg.NamesUnreadable() {
			t.Errorf("package %d: %q by %q, unreadable %v", i, pkg.Name(), pkg.Publisher(), pkg.NamesUnreadable())
		}
	}
	update := container.Packages[1]
	if string(update.Meta[tlvTitleName]) != "Made-up Quest" || string(update.Meta[tlvPublisher]) != "Nobody Games" || string(update.Meta[tlvDisplayVersion]) != "1.0.1" {
		t.Errorf("update records: %q %q %q", update.Meta[tlvTitleName], update.Meta[tlvPublisher], update.Meta[tlvDisplayVersion])
	}
	if update.Meta[tlvLocalizedTitles] != nil || update.Meta[tlvIcon] != nil || update.Meta[tlvLocalizedIcons] != nil {
		t.Error("the update was given names by language or icons")
	}
	if emulators := container.Emulators(); emulators.NoStart != "" || len(emulators.Refused) != 0 || emulators.ZeroBased {
		t.Errorf("emulators: %+v", emulators)
	}
	if _, check, err := Verify(context.Background(), out, 1, 1, nil); err != nil || len(check.Damaged) != 0 {
		t.Errorf("verify: %v %+v", err, check.Damaged)
	}
}

// Renaming a NUP whose NACPs keep their names compressed keeps them compressed, the same way:
// they read back with the new name in each language that had one, their publishers and the
// rest of the NACP as they were, and the name record of the update renamed too.
func TestEditMetaCompressedNacp(t *testing.T) {
	out, _ := pack(t, compressedDump(t, false), PackOptions{Level: 3, Workers: 2})
	before, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, rename := range []string{"Renamed Quest", "Again"} {
		if err := EditMeta(out, rename, nil); err != nil {
			t.Fatal(err)
		}
		after, err := Open(out)
		if err != nil {
			t.Fatal(err)
		}
		want := map[int][2]string{0: {rename, "Nobody Games"}, 2: {rename, "Nobadi"}, 16: {rename, "Nikt"}}
		for i, pkg := range after.Packages {
			nacp := pkg.Meta[tlvNacp]
			if !nacpCompressed(nacp) {
				t.Fatalf("%q: the NACP of package %d no longer keeps its names compressed", rename, i)
			}
			got := nacpNames(t, nacp)
			if len(got) != len(want) {
				t.Errorf("%q: package %d has names %v", rename, i, got)
			}
			for index, entry := range want {
				if got[index] != entry {
					t.Errorf("%q: package %d entry %d = %q, want %q", rename, i, index, got[index], entry)
				}
			}
			if old := before.Packages[i].Meta[tlvNacp]; !bytes.Equal(nacp[nacpTitleData:], old[nacpTitleData:]) {
				t.Errorf("%q: the NACP of package %d changed past its names", rename, i)
			}
			if pkg.Name() != rename || pkg.Publisher() != "Nobody Games" {
				t.Errorf("%q: package %d reads %q by %q", rename, i, pkg.Name(), pkg.Publisher())
			}
		}
		if got := string(after.Packages[1].Meta[tlvTitleName]); got != rename {
			t.Errorf("%q: the name record of the update is %q", rename, got)
		}
	}
	if _, check, err := Verify(context.Background(), out, 1, 1, nil); err != nil || len(check.Damaged) != 0 {
		t.Errorf("verify: %v %+v", err, check.Damaged)
	}
}

// A NACP that keeps its names compressed but whose names do not decompress is told by Verify,
// and a rename leaves it as it was rather than break it further; the game is renamed.
func TestCompressedNacpUnreadable(t *testing.T) {
	out, _ := pack(t, compressedDump(t, true), PackOptions{Level: 3, Workers: 2})
	container, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	update := container.Packages[1]
	if !update.NamesUnreadable() || container.Packages[0].NamesUnreadable() {
		t.Fatalf("unreadable: game %v, update %v", container.Packages[0].NamesUnreadable(), update.NamesUnreadable())
	}
	// the broken names are not read as a name, and no name record is made of them
	if update.Name() != "" || update.Meta[tlvTitleName] != nil {
		t.Errorf("update name %q, record %q", update.Name(), update.Meta[tlvTitleName])
	}
	_, check, err := Verify(context.Background(), out, 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(check.Damaged) != 1 || check.Damaged[0].Package != "update v65536" || check.Damaged[0].Path != "metadata: NACP" ||
		!strings.Contains(check.Damaged[0].Reason, "compressed") {
		t.Errorf("damaged %+v", check.Damaged)
	}
	if err := EditMeta(out, "Renamed Again", nil); err != nil {
		t.Fatal(err)
	}
	after, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after.Packages[1].Meta[tlvNacp], update.Meta[tlvNacp]) {
		t.Error("the NACP whose names cannot be read changed")
	}
	if after.Packages[0].Name() != "Renamed Again" || nacpNames(t, after.Packages[0].Meta[tlvNacp])[0][0] != "Renamed Again" {
		t.Errorf("game %q", after.Packages[0].Name())
	}
}

// A rename of a NACP that keeps its names as they are writes them as they are.
func TestRenamedNacpPlain(t *testing.T) {
	nacp := make([]byte, nacpSize)
	copy(nacp, "Old")
	copy(nacp[2*nacpEntry:], "Furui")
	copy(nacp[nacpVersionAt:], "1.0.0")
	renamed, err := renamedNacp(nacp, "New")
	if err != nil {
		t.Fatal(err)
	}
	if string(cString(renamed[:0x200])) != "New" || string(cString(renamed[2*nacpEntry:][:0x200])) != "New" || string(cString(renamed[nacpEntry:][:0x200])) != "" {
		t.Errorf("%q %q %q", cString(renamed[:0x200]), cString(renamed[nacpEntry:][:0x200]), cString(renamed[2*nacpEntry:][:0x200]))
	}
	if !bytes.Equal(renamed[nacpTitleData:], nacp[nacpTitleData:]) || string(cString(nacp[:0x200])) != "Old" {
		t.Error("the rest of the NACP changed, or the NACP given")
	}
}
