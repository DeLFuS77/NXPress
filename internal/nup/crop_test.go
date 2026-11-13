package nup

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// quarters is an image of 400×200 whose left half is red and right half blue.
func quarters() image.Image {
	picture := image.NewRGBA(image.Rect(0, 0, 400, 200))
	for y := range 200 {
		for x := range 400 {
			c := color.RGBA{255, 0, 0, 255}
			if x >= 200 {
				c = color.RGBA{0, 0, 255, 255}
			}
			picture.Set(x, y, c)
		}
	}
	return picture
}

func pixel(t *testing.T, icon []byte, x, y int) (uint32, uint32, uint32) {
	t.Helper()
	picture, err := jpeg.Decode(bytes.NewReader(icon))
	if err != nil {
		t.Fatal(err)
	}
	if picture.Bounds() != image.Rect(0, 0, IconSize, IconSize) {
		t.Fatalf("icon of %v", picture.Bounds())
	}
	r, g, b, _ := picture.At(x, y).RGBA()
	return r >> 8, g >> 8, b >> 8
}

// A crop takes the square chosen, kept inside the image; Fit takes all of it on white; none takes
// the square at the middle.
func TestMakeIconCrop(t *testing.T) {
	data := encodePng(t, quarters())
	red := func(r, g, b uint32) bool { return r > 200 && g < 60 && b < 60 }
	blue := func(r, g, b uint32) bool { return b > 200 && r < 60 && g < 60 }
	white := func(r, g, b uint32) bool { return r > 230 && g > 230 && b > 230 }

	left, err := MakeIconCrop(data, Crop{X: 0, Y: 0, Size: 200})
	if err != nil {
		t.Fatal(err)
	}
	if !red(pixel(t, left, 128, 128)) || !red(pixel(t, left, 250, 128)) {
		t.Error("the left square is not all red")
	}
	// a square past the right edge is moved back inside: all blue
	right, err := MakeIconCrop(data, Crop{X: 390, Y: -30, Size: 200})
	if err != nil {
		t.Fatal(err)
	}
	if !blue(pixel(t, right, 5, 5)) || !blue(pixel(t, right, 250, 250)) {
		t.Error("the right square is not all blue")
	}
	// the middle: red on the left, blue on the right
	middle, err := MakeIconCrop(data, Crop{})
	if err != nil {
		t.Fatal(err)
	}
	if !red(pixel(t, middle, 40, 128)) || !blue(pixel(t, middle, 216, 128)) {
		t.Error("the middle square is not half and half")
	}
	if plain, _ := MakeIcon(data); !bytes.Equal(plain, middle) {
		t.Error("MakeIcon is not the crop at the middle")
	}
	// the whole image: white above and below, red and blue across
	whole, err := MakeIconCrop(data, Crop{Fit: true})
	if err != nil {
		t.Fatal(err)
	}
	if !white(pixel(t, whole, 128, 10)) || !white(pixel(t, whole, 128, 245)) || !red(pixel(t, whole, 30, 128)) || !blue(pixel(t, whole, 226, 128)) {
		t.Error("the whole image is not fitted on white")
	}
	if (Crop{}).Chosen() || !(Crop{Fit: true}).Chosen() || !(Crop{Size: 10}).Chosen() {
		t.Error("Chosen")
	}
}

// The image the window crops on is made small, keeping its shape.
func TestCropSource(t *testing.T) {
	source, err := CropSource(encodePng(t, quarters()), 100)
	if err != nil {
		t.Fatal(err)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(source))
	if err != nil || config.Width != 100 || config.Height != 50 {
		t.Errorf("%+v %v", config, err)
	}
	// a small image is kept at its size
	source, _ = CropSource(encodePng(t, quarters()), 1024)
	if config, _ := jpeg.DecodeConfig(bytes.NewReader(source)); config.Width != 400 || config.Height != 200 {
		t.Errorf("%+v", config)
	}
	if _, err := CropSource([]byte("not an image"), 100); err == nil {
		t.Error("a file that is not an image was taken")
	}
}
