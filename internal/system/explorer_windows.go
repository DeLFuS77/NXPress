//go:build windows

package system

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// The places of NXPress in the Explorer besides "Send to": the program that opens its files with
// a double click (#87) and a menu of its own on a right click (#88). Both are written for this
// user only, under HKEY_CURRENT_USER, so they need no administrator, and they name the copy of
// NXPress that wrote them: NXPress is portable, and after it is moved they are written again.

// FileClass is the kind of file the Explorer gives to NXPress: the files it opens.
const FileClass = "NXPress.SwitchFile"

// FileTypes are the files NXPress opens, and the ones its menu shows on.
var FileTypes = []string{".nsp", ".nsz", ".xci", ".xcz", ".nup"}

// compressed are the types the menu offers to decompress, not to compress.
var compressed = map[string]bool{".nsz": true, ".xcz": true, ".nup": true}

// explorerKeys are where the places are written. Tests write under a key of their own.
type explorerKeys struct {
	// under HKEY_CURRENT_USER: its Classes, RegisteredApplications and NXPress\Capabilities
	software string
	// where the classes of the computer and of this user are seen together, for the program a
	// type had before
	merged     registry.Key
	mergedPath string
	// where the Explorer keeps the program chosen for each type with Open with › Always
	fileExts string
}

var explorer = explorerKeys{
	software: "Software", merged: registry.CLASSES_ROOT,
	fileExts: `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts`,
}

func (k explorerKeys) classes() string { return k.software + `\Classes` }

func (k explorerKeys) mergedKey(path string) string {
	if k.mergedPath == "" {
		return path
	}
	return k.mergedPath + `\` + path
}

// notifyShell tells the Explorer that the programs of the files changed, so it shows them at
// once. Tests do without.
var notifyShell = func() {
	const assocChanged, idList = 0x08000000, 0
	syscall.NewLazyDLL("shell32.dll").NewProc("SHChangeNotify").Call(assocChanged, idList, 0, 0)
}

// The states of a place: not there, there for this copy of NXPress, or there for another copy.
const (
	PlaceMissing = ""
	PlaceHere    = "here"
	PlaceMoved   = "moved"
)

// ExplorerMenu is the text of each entry of the menu, in the language of the window.
type ExplorerMenu struct {
	Verify     string `json:"verify"`
	Info       string `json:"info"`
	Compress   string `json:"compress"`
	Decompress string `json:"decompress"`
	Pack       string `json:"pack"`
}

// a command of the Explorer for this copy of NXPress: the program, then its arguments
func command(executable string, args ...string) string {
	return `"` + executable + `" ` + strings.Join(args, " ") + ` "%1"`
}

// commandProgram is the program a command starts, "" for none.
func commandProgram(command string) string {
	if strings.HasPrefix(command, `"`) {
		if end := strings.Index(command[1:], `"`); end >= 0 {
			return command[1 : end+1]
		}
		return ""
	}
	program, _, _ := strings.Cut(command, " ")
	return program
}

func sameProgram(a, b string) bool {
	return a != "" && strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// placeOf tells whether a command was written for this copy of NXPress.
func placeOf(command, executable string) string {
	switch {
	case command == "":
		return PlaceMissing
	case sameProgram(commandProgram(command), executable):
		return PlaceHere
	default:
		return PlaceMoved
	}
}

func readString(root registry.Key, path, name string) string {
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	value, _, err := key.GetStringValue(name)
	if err != nil {
		return ""
	}
	return value
}

func writeString(path, name, value string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(name, value)
}

// deleteTree removes a key of this user and all it holds; a key not there is no error.
func deleteTree(path string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	names, err := key.ReadSubKeyNames(-1)
	key.Close()
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := deleteTree(path + `\` + name); err != nil {
			return err
		}
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// deleteValue removes a value of a key of this user, then the key, and the one holding it, when
// they are left empty; up to stop, which stays.
func deleteValue(path, name, stop string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.SET_VALUE|registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	err = key.DeleteValue(name)
	key.Close()
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	for path != stop && strings.HasPrefix(path, stop+`\`) {
		key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
		if err != nil {
			return nil
		}
		info, err := key.Stat()
		key.Close()
		if err != nil || info.SubKeyCount > 0 || info.ValueCount > 0 {
			return nil
		}
		if registry.DeleteKey(registry.CURRENT_USER, path) != nil {
			return nil
		}
		path = path[:strings.LastIndex(path, `\`)]
	}
	return nil
}

// FilesPlace tells whether NXPress opens its files with a double click: not, for this copy, or
// for another copy; and, when it does, the types another program still opens (one chosen with
// Open with › Always, or the one the type had before).
func FilesPlace(executable string) (string, []string) {
	place := placeOf(readString(registry.CURRENT_USER, explorer.classes()+`\`+FileClass+`\shell\open\command`, ""), executable)
	if place == PlaceMissing {
		return place, nil
	}
	others := []string{}
	for _, extension := range FileTypes {
		// Windows 11 24H2 keeps it in UserChoiceLatest, the ones before in UserChoice
		chosen := readString(registry.CURRENT_USER, explorer.fileExts+`\`+extension+`\UserChoiceLatest\ProgId`, "ProgId")
		if chosen == "" {
			chosen = readString(registry.CURRENT_USER, explorer.fileExts+`\`+extension+`\UserChoice`, "ProgId")
		}
		if chosen == "" {
			chosen = readString(explorer.merged, explorer.mergedKey(extension), "")
		}
		if chosen != FileClass && !strings.EqualFold(chosen, `Applications\`+filepath.Base(executable)) {
			others = append(others, extension)
		}
	}
	return place, others
}

// AddFiles makes NXPress a program of its files: it shows in Open with, and opens them with a
// double click where no other program does yet. The settings of Windows › Default apps list it
// too. A double click starts NXPress with --open and the file.
func AddFiles(executable string) error {
	classes := explorer.classes()
	class := classes + `\` + FileClass
	steps := []struct{ path, name, value string }{
		{class, "", "Switch file"},
		{class + `\DefaultIcon`, "", `"` + executable + `",0`},
		{class + `\shell\open\command`, "", command(executable, "--open")},
		{explorer.software + `\NXPress\Capabilities`, "ApplicationName", "NXPress"},
		{explorer.software + `\NXPress\Capabilities`, "ApplicationDescription", "Compress, check and organize Switch files"},
		{explorer.software + `\RegisteredApplications`, "NXPress", explorer.software + `\NXPress\Capabilities`},
	}
	for _, extension := range FileTypes {
		steps = append(steps,
			struct{ path, name, value string }{classes + `\` + extension + `\OpenWithProgids`, FileClass, ""},
			struct{ path, name, value string }{explorer.software + `\NXPress\Capabilities\FileAssociations`, extension, FileClass})
		// the type opens with NXPress only when no program had it: one chosen stays
		if readString(explorer.merged, explorer.mergedKey(extension), "") == "" {
			steps = append(steps, struct{ path, name, value string }{classes + `\` + extension, "", FileClass})
		}
	}
	for _, step := range steps {
		if err := writeString(step.path, step.name, step.value); err != nil {
			return err
		}
	}
	notifyShell()
	return nil
}

// RemoveFiles takes NXPress away from the programs of its files, and leaves the ones of other
// programs as they are.
func RemoveFiles() error {
	classes := explorer.classes()
	for _, extension := range FileTypes {
		if err := deleteValue(classes+`\`+extension+`\OpenWithProgids`, FileClass, classes); err != nil {
			return err
		}
		if readString(registry.CURRENT_USER, classes+`\`+extension, "") == FileClass {
			if err := deleteValue(classes+`\`+extension, "", classes); err != nil {
				return err
			}
		}
	}
	if err := deleteValue(explorer.software+`\RegisteredApplications`, "NXPress", explorer.software); err != nil {
		return err
	}
	for _, path := range []string{classes + `\` + FileClass, explorer.software + `\NXPress\Capabilities`} {
		if err := deleteTree(path); err != nil {
			return err
		}
	}
	// the key of NXPress, left empty
	deleteValue(explorer.software+`\NXPress`, "ApplicationName", explorer.software)
	notifyShell()
	return nil
}

// menuKeys are the keys of the menu of NXPress: one for each type, and one for folders.
func menuKeys() []string {
	keys := []string{}
	for _, extension := range FileTypes {
		keys = append(keys, explorer.classes()+`\SystemFileAssociations\`+extension+`\shell\NXPress`)
	}
	return append(keys, explorer.classes()+`\Directory\shell\NXPress`)
}

// MenuPlace tells whether the menu of NXPress is there: not, for this copy, or for another copy.
func MenuPlace(executable string) string {
	return placeOf(readString(registry.CURRENT_USER, menuKeys()[0]+`\shell\1verify\command`, ""), executable)
}

// AddMenu puts the menu of NXPress on a right click of its files and of folders: one entry,
// NXPress, with the sections that work on them. Each entry starts NXPress with --section, the
// section and the file; several files chosen start one each, which gather in one window. On
// Windows 11 it is under Show more options.
func AddMenu(executable string, texts ExplorerMenu) error {
	if err := RemoveMenu(); err != nil {
		return err
	}
	for i, key := range menuKeys() {
		folder := i == len(FileTypes)
		entries := [][2]string{{"verify", texts.Verify}, {"info", texts.Info}, {"compress", texts.Compress}, {"pack", texts.Pack}}
		if folder {
			// a folder holds files to compress, check, or pack: a dump folder makes a NUP
			entries = [][2]string{{"compress", texts.Compress}, {"verify", texts.Verify}, {"pack", texts.Pack}}
		} else if compressed[FileTypes[i]] {
			entries[2] = [2]string{"decompress", texts.Decompress}
		}
		values := [][3]string{
			{key, "MUIVerb", "NXPress"},
			{key, "Icon", `"` + executable + `",0`},
			{key, "SubCommands", ""},
			// Player: shown for any number of files chosen, not only up to 15
			{key, "MultiSelectModel", "Player"},
		}
		for n, entry := range entries {
			sub := key + `\shell\` + string(rune('1'+n)) + entry[0]
			values = append(values,
				[3]string{sub, "MUIVerb", entry[1]},
				[3]string{sub, "MultiSelectModel", "Player"},
				[3]string{sub + `\command`, "", command(executable, "--section", entry[0])})
		}
		for _, value := range values {
			if err := writeString(value[0], value[1], value[2]); err != nil {
				return err
			}
		}
	}
	notifyShell()
	return nil
}

// RelabelMenu writes the texts of the menu again, in the language of the window, when the menu
// is there for this copy of NXPress.
func RelabelMenu(executable string, texts ExplorerMenu) error {
	if MenuPlace(executable) != PlaceHere {
		return nil
	}
	return AddMenu(executable, texts)
}

// RemoveMenu takes the menu of NXPress away, and the keys of the types left empty.
func RemoveMenu() error {
	for _, key := range menuKeys() {
		if err := deleteTree(key); err != nil {
			return err
		}
		// SystemFileAssociations\.nsp\shell, then .nsp, when nothing else is there
		parent := key[:strings.LastIndex(key, `\`)]
		deleteValue(parent, "", explorer.classes())
	}
	return nil
}

// InstallerLanguage is the language chosen in the installer of Windows, which it keeps under
// Software\NXPress of the user; "" without one.
func InstallerLanguage() string {
	return readString(registry.CURRENT_USER, `Software\NXPress`, "Language")
}

// ExplorerPlaces says whether this system has them.
func ExplorerPlaces() bool { return true }

// Executable is this copy of NXPress, with its links followed.
func Executable() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	return executable, nil
}
