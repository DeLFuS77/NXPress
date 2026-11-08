//go:build !windows

package system

import (
	"errors"
	"os"
)

// Only the Explorer of Windows opens files with NXPress or shows its menu (#87, #88).

const (
	PlaceMissing = ""
	PlaceHere    = "here"
	PlaceMoved   = "moved"
)

type ExplorerMenu struct {
	Verify     string `json:"verify"`
	Info       string `json:"info"`
	Compress   string `json:"compress"`
	Decompress string `json:"decompress"`
	Pack       string `json:"pack"`
}

var errNoExplorer = errors.New("only the Explorer of Windows has these")

func FilesPlace(executable string) (string, []string)         { return PlaceMissing, nil }
func AddFiles(executable string) error                        { return errNoExplorer }
func RemoveFiles() error                                      { return nil }
func MenuPlace(executable string) string                      { return PlaceMissing }
func AddMenu(executable string, texts ExplorerMenu) error     { return errNoExplorer }
func RelabelMenu(executable string, texts ExplorerMenu) error { return nil }
func RemoveMenu() error                                       { return nil }
func ExplorerPlaces() bool                                    { return false }
func Executable() (string, error)                             { return os.Executable() }
