//go:build cgo

package optimize

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/settings"
	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// the made-up title key encryption key of these tests, besides the keys of testkit
const testTitleKek = "2f2e2d2c2b2a29282726252423222120"

// rebuildKeys are the keys of testkit with a titlekek too, for the NCA with a rights ID.
func rebuildKeys(t *testing.T) {
	t.Helper()
	testkit.Isolate(t)
	keys := "header_key = " + testkit.HeaderKey + "\nkey_area_key_application_00 = " + testkit.AreaKey +
		"\nkey_area_key_application_02 = " + testkit.AreaKey + "\ntitlekek_00 = " + testTitleKek + "\n"
	path := filepath.Join(t.TempDir(), "prod.keys")
	if err := os.WriteFile(path, []byte(keys), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := settings.UseSwitchKeys(path); err != nil {
		t.Fatal(err)
	}
}

// the key of the sections of the NCA of testkit
var testContentKey, _ = hex.DecodeString("00112233445566778899aabbccddeeff")

// testTicket is a common ticket for rights with the key of testkit as its title key.
func testTicket(t *testing.T, rights []byte) []byte {
	t.Helper()
	tik := make([]byte, 0x2C0)
	binary.LittleEndian.PutUint32(tik, 0x10004)
	kek, _ := hex.DecodeString(testTitleKek)
	block, err := aes.NewCipher(kek)
	if err != nil {
		t.Fatal(err)
	}
	block.Encrypt(tik[0x180:0x190], testContentKey)
	copy(tik[0x2A0:], rights)
	return tik
}

// rebuilt is what a test reads back of a rebuilt NSP: its program NCA, decrypted, and the
// content records of its CNMT.
type rebuilt struct {
	names   []string
	program []byte // section 0 of the program NCA, decrypted
	records []record
	header  []byte // of the program NCA, decrypted
}

type record struct {
	hash [32]byte
	id   string
	size uint64
	typ  byte
}

func readRebuilt(t *testing.T, path string) rebuilt {
	t.Helper()
	k, err := loadKeys()
	if err != nil {
		t.Fatal(err)
	}
	p, err := openNsp(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	g, err := readGame(k, []*pkgFile{p}, readCnmt)
	if err != nil {
		t.Fatal(err)
	}
	out := rebuilt{names: p.order}
	if len(g.metas) != 1 {
		t.Fatalf("%d CNMT", len(g.metas))
	}
	for id, n := range g.ncas {
		if strings.HasSuffix(n.name, ".cnmt.nca") {
			continue
		}
		if err := n.unlock(k, nil); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if out.program, err = n.readSection(0); err != nil {
			t.Fatal(err)
		}
		out.header = n.header
	}
	// the CNMT in the PFS0 of the meta section
	section := g.metas[0].section
	section = section[binary.LittleEndian.Uint64(g.metas[0].nca.fsHeader(0)[0x40:]):]
	count := int(binary.LittleEndian.Uint32(section[4:]))
	table := int(binary.LittleEndian.Uint32(section[8:]))
	data := 0x10 + count*0x18 + table
	offset := int(binary.LittleEndian.Uint64(section[0x10:]))
	cnmt := section[data+offset:]
	extended := int(binary.LittleEndian.Uint16(cnmt[0xE:]))
	contents := int(binary.LittleEndian.Uint16(cnmt[0x10:]))
	for i := range contents {
		r := cnmt[0x20+extended+i*0x38:]
		var rec record
		copy(rec.hash[:], r[:0x20])
		rec.id = hex.EncodeToString(r[0x20:0x30])
		size := make([]byte, 8)
		copy(size, r[0x30:0x36])
		rec.size = binary.LittleEndian.Uint64(size)
		rec.typ = r[0x36]
		out.records = append(out.records, rec)
	}
	return out
}

// Packages with the defects a tool can leave are rebuilt clean: each NCA named by its hash and
// listed so by the CNMT with its hash and size, standard crypto with no ticket, the hashes of
// the FS headers right, marked as downloaded, the files that are not NCA left out; the data the
// same; and the new NSP checked whole (no NCA MODIFIED, none in part).
func TestRebuildMakesPackagesClean(t *testing.T) {
	rebuildKeys(t)
	plain := testkit.GameData(0x10000, 1)
	rights := make([]byte, 16)
	binary.BigEndian.PutUint64(rights, testkit.BaseID)
	rights[15] = 0x00
	tests := []struct {
		name     string
		program  func(header []byte)
		wrongID  bool
		extra    func() map[string][]byte
		renamed  int
		standard int
		fsFixed  int
		download int
		dropped  int
	}{
		{name: "clean"},
		{name: "named by another ID", wrongID: true, renamed: 1},
		{name: "a wrong FS header hash", program: func(h []byte) { h[0x280] ^= 0xFF }, renamed: 1, fsFixed: 1},
		{name: "for a game card", program: func(h []byte) { h[0x204] = 1 }, renamed: 1, download: 1},
		{
			name:     "a rights ID and its ticket",
			program:  func(h []byte) { copy(h[0x230:], rights); clear(h[0x300:0x340]) },
			extra:    func() map[string][]byte { return map[string][]byte{hex.EncodeToString(rights) + ".tik": testTicket(t, rights), hex.EncodeToString(rights) + ".cert": make([]byte, 0x700)} },
			renamed:  1,
			standard: 1,
			dropped:  2,
		},
		{name: "files that are not NCA", extra: func() map[string][]byte { return map[string][]byte{"info.xml": []byte("<x/>"), "icon.jpg": {1, 2}} }, dropped: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			folder := t.TempDir()
			// its size in its header, as Nintendo writes it, then the defect
			program := testkit.ProgramNcaWith(plain, func(h []byte) {
				binary.LittleEndian.PutUint64(h[0x208:], uint64(0x4000+len(plain)))
				if test.program != nil {
					test.program(h)
				}
			})
			name := testkit.ContentName(program, ".nca")
			if test.wrongID {
				name = "0123456789abcdef0123456789abcdef.nca"
			}
			var extra map[string][]byte
			if test.extra != nil {
				extra = test.extra()
			}
			source := testkit.NspWith(t, filepath.Join(folder, "game.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, program, name, extra)
			target := filepath.Join(folder, "game (rebuilt).nsp")
			res, err := Rebuild(context.Background(), source, target, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if res.Titles != 1 || res.NCAs != 1 || res.Renamed != test.renamed || res.Standard != test.standard ||
				res.FsFixed != test.fsFixed || res.Download != test.download || res.Dropped != test.dropped {
				t.Fatalf("%+v", res)
			}
			got := readRebuilt(t, target)
			if !bytes.Equal(got.program, plain) {
				t.Fatal("the data of the program changed")
			}
			if len(got.names) != 2 {
				t.Fatalf("files: %v", got.names)
			}
			if !bytes.Equal(got.header[0x230:0x240], make([]byte, 16)) || got.header[0x204] != 0 {
				t.Fatal("the rights ID or the distribution type are left")
			}
			whole, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			for _, rec := range got.records {
				if rec.typ == 0 { // meta
					continue
				}
				p, err := openNsp(target)
				if err != nil {
					t.Fatal(err)
				}
				e, ok := p.entries[rec.id+".nca"]
				p.Close()
				if !ok {
					t.Fatalf("%s.nca, listed by the CNMT, is not there: %v", rec.id, got.names)
				}
				sum := sha256.Sum256(whole[e.offset : e.offset+e.size])
				if sum != rec.hash || uint64(e.size) != rec.size || hex.EncodeToString(sum[:16]) != rec.id {
					t.Fatalf("the CNMT lists %s with a wrong hash or size", rec.id)
				}
			}
			check, err := switchfs.VerifyFile(context.Background(), target, nil)
			if err != nil || len(check.Modified) > 0 || check.Partial {
				t.Fatalf("checked: %v %+v", err, check)
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatal("the original is gone")
			}
		})
	}
}

// What cannot be rebuilt is told, and no file is left: an NCA with a rights ID and no ticket,
// and a package without a CNMT.
func TestRebuildTellsWhatItCannot(t *testing.T) {
	rebuildKeys(t)
	folder := t.TempDir()
	rights := make([]byte, 16)
	rights[0] = 1
	program := testkit.ProgramNcaWith(testkit.GameData(0x10000, 2), func(h []byte) { copy(h[0x230:], rights) })
	source := testkit.NspWith(t, filepath.Join(folder, "game.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, program, testkit.ContentName(program, ".nca"), nil)
	target := filepath.Join(folder, "out.nsp")
	if _, err := Rebuild(context.Background(), source, target, Options{}); err == nil || !strings.Contains(err.Error(), "no ticket") {
		t.Fatalf("told %v", err)
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("a file was left")
	}
}
