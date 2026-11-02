package jobs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dtrunk90/switch-library-manager-web/settings"
)

// LoadKeys makes the keys active, and gives the file they were read from: the ones given, else
// a prod.keys or a keys.txt next to NXPress, in the working folder or in ~/.switch, the folder
// nsz and hactool use too. A file without header_key is no prod.keys, whatever else it holds.
func LoadKeys(given string) (string, error) {
	if given != "" {
		if err := settings.UseSwitchKeys(given); err != nil {
			return "", fmt.Errorf("the keys %s cannot be read: %w", given, err)
		}
		if !settings.IsKeysFileAvailable() {
			return "", fmt.Errorf("%s is not a prod.keys: it has no header_key", given)
		}
		return given, nil
	}
	folders := []string{"."}
	if executable, err := os.Executable(); err == nil {
		folders = append(folders, filepath.Dir(executable))
	}
	if home, err := os.UserHomeDir(); err == nil {
		folders = append(folders, filepath.Join(home, ".switch"))
	}
	for _, folder := range folders {
		for _, name := range []string{"prod.keys", "keys.txt"} {
			path := filepath.Join(folder, name)
			if _, err := os.Stat(path); err == nil && settings.UseSwitchKeys(path) == nil && settings.IsKeysFileAvailable() {
				if absolute, err := filepath.Abs(path); err == nil {
					path = absolute
				}
				return path, nil
			}
		}
	}
	return "", fmt.Errorf("no prod.keys or keys.txt found (looked in %s): give one with --keys", strings.Join(folders, ", "))
}
