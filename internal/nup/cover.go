package nup

import (
	"bytes"
	"errors"
	"fmt"
	"image/jpeg"
)

// A cover is the icon the person chose for the NUP of a dump folder, in the window of Combine: a
// JPEG of IconSize × IconSize pixels, cut and placed there as the person wished. It takes the place
// of the icon of the game, of its control data or of titledb, in the metadata of the package that
// carries the game (PackOptions.Cover). As an icon given to a NUP afterwards, it is then the one icon
// the NUP carries, and Change image in its card can change it.

// CheckCover tells whether data is a cover a NUP can carry: a JPEG of IconSize × IconSize pixels
// that reads whole, no bigger than the icons of a control.
func CheckCover(data []byte) error {
	if len(data) == 0 {
		return errors.New("the cover is empty")
	}
	if len(data) > maxMetaSize/32 {
		return fmt.Errorf("the cover is %d KiB: the icon of a game is at most %d KiB", (len(data)+1023)>>10, maxMetaSize/32>>10)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("the cover is not a JPEG image: %w", err)
	}
	if size := img.Bounds().Size(); size.X != IconSize || size.Y != IconSize {
		return fmt.Errorf("the cover is %d × %d pixels, not %d × %d", size.X, size.Y, IconSize, IconSize)
	}
	return nil
}
