package jobs

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"path/filepath"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// coverJpeg is a small JPEG image of one shade, as an icon of a game.
func coverJpeg(t *testing.T, shade uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		img.Set(x, x, color.RGBA{shade, shade / 2, 0, 255})
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, nil); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

// The cover of a pack of a game and its update is the one of the game, or the one of its newest
// update when chosen in Settings > Appearance, which the console shows once it is installed
// (#158); the game's when its update has none.
func TestReadTitleCoverOfAPack(t *testing.T) {
	testkit.Isolate(t)
	game, update := coverJpeg(t, 40), coverJpeg(t, 220)
	dataURL := func(icon []byte) string { return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(icon) }
	pack := testkit.PackNsp(t, filepath.Join(t.TempDir(), "Pack Game [0100000000010000][v65536].nsp"), []testkit.PackTitle{
		{ID: testkit.BaseID, Kind: switchfs.ContentMetaType_Application, Icon: game},
		{ID: testkit.UpdateID, Version: 65536, Kind: switchfs.ContentMetaType_Patch, Icon: update},
	})
	for cover, want := range map[Cover]string{NoCover: "", GameCover: dataURL(game), UpdateCover: dataURL(update)} {
		title, err := ReadTitle(pack, cover)
		if err != nil {
			t.Fatal(err)
		}
		if title.Icon != want {
			t.Errorf("cover %d: the icon is not the one asked for (%d characters)", cover, len(title.Icon))
		}
		if title.Bases != 1 || title.Updates != 1 || title.Latest != 65536 {
			t.Errorf("cover %d: the titles of the pack: %+v", cover, title)
		}
	}

	// an update without an icon of its own: the one of the game
	pack = testkit.PackNsp(t, filepath.Join(t.TempDir(), "No Icon [0100000000010000][v65536].nsp"), []testkit.PackTitle{
		{ID: testkit.BaseID, Kind: switchfs.ContentMetaType_Application, Icon: game},
		{ID: testkit.UpdateID, Version: 65536, Kind: switchfs.ContentMetaType_Patch},
	})
	if title, err := ReadTitle(pack, UpdateCover); err != nil || title.Icon != dataURL(game) {
		t.Fatalf("the game's, as its update has none: %v", err)
	}
}
