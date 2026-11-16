package main

import (
	"encoding/base64"
	"fmt"
	"path/filepath"

	"github.com/DeLFuS77/NXPress/internal/jobs"
	"github.com/DeLFuS77/NXPress/internal/nup"
)

// SetSplitParts tells Split what to take out of one NUP: its game, its updates, its DLC and its icon.
func (a *App) SetSplitParts(path string, base bool, update bool, dlc bool, icon bool) {
	jobs.SetSplitParts(path, base, update, dlc, icon)
}

// EditNup changes the title name and/or the icon in the metadata of a NUP, where it is (Rename): an
// empty title leaves the name as it is, and an empty icon (a JPEG of 256 × 256 pixels, in base64)
// leaves the icon.
func (a *App) EditNup(path string, title string, icon string) error {
	var data []byte
	if icon != "" {
		var err error
		if data, err = base64.StdEncoding.DecodeString(icon); err != nil {
			return fmt.Errorf("the icon of %s cannot be read: %w", filepath.Base(path), err)
		}
	}
	return nup.EditMeta(path, title, data)
}
