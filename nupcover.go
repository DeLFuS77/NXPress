package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/jobs"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// The cover of the NUP of a dump folder (Combine): the window asks for a picture, cuts and places
// it itself with what its engine shows (WebP, AVIF and the rest), and gives NXPress the JPEG of
// 256 × 256 pixels that results. NXPress keeps it for the folder until it is changed or taken away.

// maxCoverSource keeps the picture chosen to a size the page can take whole.
const maxCoverSource = 40 << 20

// coverTypes are the pictures a cover is made of, by their extension.
var coverTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".jpe": "image/jpeg", ".jfif": "image/jpeg",
	".webp": "image/webp", ".gif": "image/gif", ".bmp": "image/bmp", ".avif": "image/avif", ".ico": "image/x-icon",
}

// ChooseCoverPicture asks for the picture of the cover of a NUP.
func (a *App) ChooseCoverPicture() (string, error) {
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Filters: []wailsruntime.FileFilter{{DisplayName: "Pictures (*.jpg, *.png, *.bmp, *.webp, *.avif, *.gif, *.ico)",
			Pattern: "*.jpg;*.jpeg;*.jpe;*.jfif;*.png;*.bmp;*.webp;*.avif;*.gif;*.ico"}},
	})
}

// CoverPicture gives the picture chosen as a data URL the page can show and cut, since the page cannot
// open the files of the computer itself.
func (a *App) CoverPicture(path string) (string, error) {
	kind := coverTypes[strings.ToLower(filepath.Ext(path))]
	if kind == "" {
		return "", errors.New("not a picture NXPress makes a cover of")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("not a picture file")
	}
	if info.Size() > maxCoverSource {
		return "", fmt.Errorf("the picture is bigger than %d MB", maxCoverSource>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// SetNupCover keeps the cover (a JPEG of 256 × 256 pixels, in base64) for the NUP of a dump folder;
// an empty one takes the cover away.
func (a *App) SetNupCover(dir string, encoded string) error {
	var cover []byte
	if encoded != "" {
		var err error
		if cover, err = base64.StdEncoding.DecodeString(encoded); err != nil {
			return fmt.Errorf("the cover of %s cannot be read: %w", filepath.Base(dir), err)
		}
	}
	if err := jobs.SetCover(dir, cover); err != nil {
		return fmt.Errorf("the cover of %s: %w", filepath.Base(dir), err)
	}
	return nil
}
