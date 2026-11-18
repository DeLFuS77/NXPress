// Package edensaves keeps the saves of Eden (and Citron) on this computer in step with Switch
// Library Manager Web while NXPress is open: it finds the library on the network as NXDock does,
// pairs with it with a code, and runs the passes of the package edensync of the library, every
// minute, when NXPress starts and when the emulator closes. Nothing runs while NXPress is closed.
package edensaves

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/DeLFuS77/NXPress/internal/durable"
)

// the files of the folder of the settings of NXPress
const (
	// the library paired, its key and the choices of the user
	ConfigName = "eden-saves.json"
	// what the passes remember (edensync.State)
	StateName = "eden-saves-state.json"
)

// Config is what NXPress keeps of the saves of Eden. The key is read only by the passes: it is
// never shown, logged or given to the page, and its file is readable by the user only.
type Config struct {
	// the address of the library (http://192.168.1.2:3000), its id on the network and how it is
	// shown (192.168.1.2:3000)
	Address string `json:"address,omitempty"`
	Id      string `json:"id,omitempty"`
	Library string `json:"library,omitempty"`
	// the key the library gave this computer, its name there and the user it is of
	Key     string `json:"key,omitempty"`
	KeyName string `json:"keyName,omitempty"`
	User    string `json:"user,omitempty"`
	// the passes run: off until the user turns them on, once paired
	On bool `json:"on,omitempty"`
	// the data folder of the emulator chosen by hand, and which emulator it is; "" when found
	// by itself
	Folder   string `json:"folder,omitempty"`
	Emulator string `json:"emulator,omitempty"`
}

// Paired tells a library paired, with its key.
func (c Config) Paired() bool {
	return c.Address != "" && c.Key != ""
}

// LoadConfig reads the settings; a file that is missing is no setting.
func LoadConfig(dir string) (Config, error) {
	config := Config{}
	data, err := os.ReadFile(filepath.Join(dir, ConfigName))
	if errors.Is(err, os.ErrNotExist) {
		return config, nil
	}
	if err != nil {
		return config, err
	}
	err = json.Unmarshal(data, &config)
	return config, err
}

// SaveConfig writes the settings through a temporary file, readable by the user only, so a file
// cut halfway never takes the place of the one there.
func SaveConfig(dir string, config Config) error {
	data, err := json.MarshalIndent(config, "", "\t")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, "."+ConfigName+".*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	err = temporary.Chmod(0o600)
	if err == nil || errors.Is(err, errors.ErrUnsupported) {
		_, err = temporary.Write(data)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = durable.CommitFile(name, filepath.Join(dir, ConfigName))
	}
	if err != nil {
		os.Remove(name)
	}
	return err
}
