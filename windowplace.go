package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// windowPlace is where the window was left and how big: it opens there again.
type windowPlace struct {
	X, Y          int
	Width, Height int
	Maximised     bool
}

// the size the window opens with the first time, and the least it takes
const (
	windowWidth     = 1120
	windowHeight    = 720
	windowMinWidth  = 900
	windowMinHeight = 520
)

func windowPlacePath() string {
	folder, err := dataFolder()
	if err != nil {
		return ""
	}
	return filepath.Join(folder, "window.json")
}

// loadWindowPlace gives the place kept, or the size of the first time.
func loadWindowPlace() (windowPlace, bool) {
	place := windowPlace{Width: windowWidth, Height: windowHeight}
	data, err := os.ReadFile(windowPlacePath())
	if err != nil || json.Unmarshal(data, &place) != nil {
		return windowPlace{Width: windowWidth, Height: windowHeight}, false
	}
	place.Width = max(place.Width, windowMinWidth)
	place.Height = max(place.Height, windowMinHeight)
	return place, true
}

// placeWindow puts the window where it was left, then shows it: it opens hidden, so it does not
// show in the middle first. A maximised one is put in its place before, so it goes back there.
// A place no screen holds any longer (a screen taken away) is left out, and the window stays
// in the middle.
func placeWindow(ctx context.Context) {
	defer wailsruntime.WindowShow(ctx)
	place, ok := loadWindowPlace()
	if !ok {
		return
	}
	if place.Maximised {
		defer wailsruntime.WindowMaximise(ctx)
	}
	screens, err := wailsruntime.ScreenGetAll(ctx)
	if err != nil || len(screens) == 0 {
		return
	}
	// the screens give their sizes, not where they are: the place must fall within all of
	// them side by side, on either side of the main one
	width, height := 0, 0
	for _, screen := range screens {
		width += screen.Size.Width
		height = max(height, screen.Size.Height)
	}
	if place.X <= -width+100 || place.X >= width-100 || place.Y < -50 || place.Y >= height-100 {
		return
	}
	wailsruntime.WindowSetPosition(ctx, place.X, place.Y)
}

// keepWindowPlace writes where the window is and how big, as it closes. A maximised window keeps
// the size and place it had before, to go back to them.
func keepWindowPlace(ctx context.Context) {
	path := windowPlacePath()
	if path == "" || wailsruntime.WindowIsMinimised(ctx) {
		return
	}
	place, _ := loadWindowPlace()
	place.Maximised = wailsruntime.WindowIsMaximised(ctx)
	if !place.Maximised {
		place.Width, place.Height = wailsruntime.WindowGetSize(ctx)
		place.X, place.Y = wailsruntime.WindowGetPosition(ctx)
	}
	data, err := json.Marshal(place)
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, data, 0o644)
}
