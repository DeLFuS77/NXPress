package nup

import (
	"bytes"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"testing"
)

// A GIF chosen for an icon makes one, as PNG, JPEG, WEBP and BMP do (brought from the edition of
// QuickWick, PR #151).
func TestMakeIconTakesAGif(t *testing.T) {
	picture := image.NewPaletted(image.Rect(0, 0, 300, 300), palette.Plan9)
	for y := 0; y < 300; y++ {
		for x := 0; x < 300; x++ {
			picture.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var data bytes.Buffer
	if err := gif.Encode(&data, picture, nil); err != nil {
		t.Fatal(err)
	}
	icon, err := MakeIcon(data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckCover(icon); err != nil {
		t.Fatal(err)
	}
}
