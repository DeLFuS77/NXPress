package nup

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testnup"
)

// a dump with control data: a name in its NACP and an icon
func editableDump(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "dump")
	nacp := make([]byte, 0x4000)
	copy(nacp[0:], "Made-up Quest")
	writeTree(t, root, map[string][]byte{
		"base/program/0/exefs/main":                       nso(1),
		"base/program/0/exefs/main.npdm":                  npdm(0x040074A01BF12000),
		"base/program/0/romfs/a.bin":                      []byte("a"),
		"base/program/0/control/control.nacp":             nacp,
		"base/program/0/control/icon_AmericanEnglish.dat": testnup.Jpeg,
		"base/program/0/meta/x.cnmt":                      []byte("cnmt"),
	})
	return root
}

func TestEditMeta(t *testing.T) {
	out, _ := pack(t, editableDump(t), PackOptions{Level: 3, Workers: 2})
	before, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	cover := coverJPEG(t, IconSize)
	if err := EditMeta(out, "  Renamed Quest  ", cover); err != nil {
		t.Fatal(err)
	}
	after, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := after.Packages[0].Name(); got != "Renamed Quest" {
		t.Errorf("name = %q", got)
	}
	if !bytes.Equal(after.Icon(), cover) || after.GameIcons() {
		t.Error("the icon is not the one icon of the NUP")
	}
	if len(after.Packages) != len(before.Packages) || len(after.Files) != len(before.Files) {
		t.Error("packages or files changed")
	}
	for i, file := range before.Files {
		if after.Files[i] != file {
			t.Errorf("file %q changed", file.Name)
		}
	}
	if _, err := os.Stat(IconBackup(out)); !os.IsNotExist(err) {
		t.Error("the copy of the table of contents was left beside the NUP")
	}
	// a name alone keeps the icon
	if err := EditMeta(out, "Again", nil); err != nil {
		t.Fatal(err)
	}
	again, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if again.Packages[0].Name() != "Again" || !bytes.Equal(again.Icon(), cover) {
		t.Error("a name alone changed the icon, or did not change the name")
	}
	// what cannot be written is refused, and the NUP stays as it is
	for name, args := range map[string]struct {
		title string
		icon  []byte
	}{
		"nothing":  {"", nil},
		"too long": {strings.Repeat("a", 600), nil},
		"control":  {"a\x00b", nil},
		"bad icon": {"x", []byte("not an image")},
	} {
		if EditMeta(out, args.title, args.icon) == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	last, err := Open(out)
	if err != nil || last.Packages[0].Name() != "Again" {
		t.Error("a refused edit changed the NUP")
	}
}

func TestUnpackSelected(t *testing.T) {
	out, _ := pack(t, editableDump(t), PackOptions{Level: 3, Workers: 2})
	ctx := context.Background()
	all, err := Unpack(ctx, out, filepath.Join(t.TempDir(), "all"), 2, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	every, err := UnpackSelected(ctx, out, filepath.Join(t.TempDir(), "every"), 2, 1, func(int) bool { return true }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if every.Files != all.Files || every.Size != all.Size {
		t.Errorf("all packages chosen: %d files, %d bytes; Unpack: %d, %d", every.Files, every.Size, all.Files, all.Size)
	}
	none, err := UnpackSelected(ctx, out, filepath.Join(t.TempDir(), "none"), 2, 1, func(int) bool { return false }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if none.Files != 0 || none.Size != 0 {
		t.Errorf("no package chosen: %d files, %d bytes", none.Files, none.Size)
	}
}
func coverJPEG(t *testing.T, side int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			if y < side/2 {
				img.Set(x, y, color.RGBA{R: 255, A: 255})
			} else {
				img.Set(x, y, color.RGBA{B: 255, A: 255})
			}
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 92}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
