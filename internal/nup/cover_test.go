package nup

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"path/filepath"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testnup"
)

// coverJPEG makes a JPEG of the side given, red on its top half and blue on the bottom one.
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

func TestCheckCover(t *testing.T) {
	if err := CheckCover(coverJPEG(t, IconSize)); err != nil {
		t.Error(err)
	}
	for name, data := range map[string][]byte{"empty": nil, "fake": testnup.Jpeg, "size": coverJPEG(t, 100), "text": []byte("hello")} {
		if CheckCover(data) == nil {
			t.Errorf("%s cover was accepted", name)
		}
	}
	// a cover that is not one stops the packer before it reads anything
	if _, err := Pack(context.Background(), "", "", PackOptions{Cover: []byte("bad")}, nil); err == nil {
		t.Error("Pack took a cover that is not one")
	}
}

// the cover takes the place of the icon of the control data and of its icons by language, and is
// the one icon of the NUP, as of a NUP given an icon afterwards
func TestPackCover(t *testing.T) {
	cover := coverJPEG(t, IconSize)
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
	// without a cover the icons of the control data are kept
	plain, _ := pack(t, root, PackOptions{Level: 3, Workers: 2})
	before, err := Open(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !before.GameIcons() || !bytes.Equal(before.Icon(), testnup.Jpeg) {
		t.Fatalf("without a cover: game icons %v", before.GameIcons())
	}
	out, _ := pack(t, root, PackOptions{Level: 3, Workers: 2, Cover: cover})
	container, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(container.Icon(), cover) {
		t.Error("the icon of the NUP is not the cover")
	}
	if container.GameIcons() {
		t.Error("the icons by language of the control data are still there")
	}
}
