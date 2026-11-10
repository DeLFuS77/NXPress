// Command installer makes the installers of NXPress, with the texts of
// build/windows/installer/texts.json in its 11 languages. For Windows it writes them as language
// strings of NSIS and runs makensis on build/windows/installer/nxpress.nsi with the program given;
// for macOS it makes a package of the app (see mac.go).
//
//	go run ./tools/installer -program out/nxpress.exe -version 1.7.8 -out dist/nxpress-windows-x64-setup.exe
//	go run ./tools/installer -mac -app out/NXPress.app -arch arm64 -version 1.7.8 -out dist/nxpress-macos-apple-silicon.pkg
//
// It runs from the root of the repository: on Windows with NSIS 3 installed (makensis in the
// path or in its usual folder), on a Mac with its own pkgbuild and productbuild.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// languages are the ones of the window, in the order of its menu: the first is the one of a
// Windows in a language NXPress has not
var languages = []string{"en", "es", "fr", "de", "it", "pt", "nl", "ru", "ja", "ko", "zh"}

// keys are the texts each language gives
var keys = []string{
	"WelcomeTitle", "WelcomeText", "WelcomeNext", "Language", "UpdateTitle", "UpdateText", "UpdateButton",
	"Notice", "LicenseBottom", "DirTop", "DirUser", "DirAdmin", "OptionsTitle", "OptionsSub",
	"OptStart", "OptDesktop", "OptFiles", "OptFilesHint", "OptMenu", "OptMenuHint", "OptSendTo", "OptSendToHint",
	"FinishTitle", "FinishText", "FinishRun", "CloseApp", "AdminDeclined", "AdminFailed",
	"UnTitle", "UnSub", "UnText", "UnKeep", "UnKeepHint", "UnClose",
}

// placeholders are the parts of a text NSIS fills in: the version made, the one installed, and
// the folder
var placeholders = map[string]string{"{VERSION}": "${VERSION}", "{OLD}": "$OldVersion", "{DIR}": "$INSTDIR"}

func main() {
	program := flag.String("program", "", "the nxpress.exe to install")
	version := flag.String("version", "dev", "the version of NXPress, as 1.7.8")
	out := flag.String("out", "", "the installer to write")
	mac := flag.Bool("mac", false, "make the package of macOS")
	app := flag.String("app", "", "the NXPress.app to install (macOS)")
	arch := flag.String("arch", "", "arm64 or x86_64 (macOS)")
	flag.Parse()
	var err error
	if *mac {
		var texts map[string]map[string]string
		if texts, err = readTexts(); err == nil {
			err = runMac(*app, *arch, *version, *out, texts)
		}
	} else {
		err = runWindows(*program, *version, *out)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "installer:", err)
		os.Exit(1)
	}
}

// readTexts reads the texts of the installers, by language.
func readTexts() (map[string]map[string]string, error) {
	data, err := os.ReadFile(filepath.Join("build", "windows", "installer", "texts.json"))
	if err != nil {
		return nil, err
	}
	texts := map[string]map[string]string{}
	if err := json.Unmarshal(data, &texts); err != nil {
		return nil, err
	}
	for _, code := range languages {
		for _, key := range append(append([]string{}, keys...), macKeys...) {
			if texts[code][key] == "" {
				return nil, fmt.Errorf("texts.json has no %s in %s", key, code)
			}
		}
	}
	return texts, nil
}

func runWindows(program, version, out string) error {
	if program == "" || out == "" {
		return errors.New("give -program and -out")
	}
	folder := filepath.Join("build", "windows", "installer")
	texts, err := readTexts()
	if err != nil {
		return err
	}
	langs, err := languageFile(texts)
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "nxpress-installer-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	// NSIS reads a script in UTF-8 when it starts with its mark
	if err := os.WriteFile(filepath.Join(work, "langs.nsh"), append([]byte{0xEF, 0xBB, 0xBF}, langs...), 0o644); err != nil {
		return err
	}
	license, err := os.ReadFile("LICENSE")
	if err != nil {
		return err
	}
	crlf := strings.ReplaceAll(strings.ReplaceAll(string(license), "\r\n", "\n"), "\n", "\r\n")
	if err := os.WriteFile(filepath.Join(work, "LICENSE.txt"), []byte(crlf), 0o644); err != nil {
		return err
	}
	info, err := os.Stat(program)
	if err != nil {
		return err
	}
	// the program, its license and the uninstaller, in KB
	size := (info.Size()+int64(len(crlf)))/1024 + 200
	makensis, err := findMakensis()
	if err != nil {
		return err
	}
	absolute := func(path string) string {
		whole, _ := filepath.Abs(path)
		return whole
	}
	if err := os.MkdirAll(filepath.Dir(absolute(out)), 0o755); err != nil {
		return err
	}
	command := exec.Command(makensis, "-V2", "-INPUTCHARSET", "UTF8",
		"-DVERSION="+version, "-DNUMBERS="+numbers(version), fmt.Sprintf("-DSIZEKB=%d", size),
		"-DPROGRAM="+absolute(program), "-DLICENSE="+filepath.Join(work, "LICENSE.txt"),
		"-DLANGS="+filepath.Join(work, "langs.nsh"), "-DIMAGES="+absolute(filepath.Join(folder, "images")),
		"-DICON="+absolute(filepath.Join("build", "winres", "icon.ico")), "-DOUTFILE="+absolute(out),
		filepath.Join(folder, "nxpress.nsi"))
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}

// numbers is the version as Windows writes it in the details of a file: 1.7.8.0, or 0.0.0.0 for
// a build that is not a version.
func numbers(version string) string {
	if regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(version) {
		return version + ".0"
	}
	return "0.0.0.0"
}

// findMakensis is makensis in the path, or where the installer of NSIS puts it.
func findMakensis() (string, error) {
	if path, err := exec.LookPath("makensis"); err == nil {
		return path, nil
	}
	for _, folder := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")} {
		path := filepath.Join(folder, "NSIS", "makensis.exe")
		if _, err := os.Stat(path); err == nil && folder != "" {
			return path, nil
		}
	}
	return "", errors.New("makensis was not found: install NSIS 3")
}

// languageFile writes the languages of NSIS and their texts: the languages of Modern UI, in the
// order of the window, a LangString for each text, and the functions that tell the language
// chosen by its code in the window and by its name in the list of the first page.
func languageFile(texts map[string]map[string]string) (string, error) {
	var b strings.Builder
	b.WriteString("; written by tools/installer from texts.json: do not change it here\r\n")
	for _, code := range languages {
		language, ok := texts[code]
		if !ok {
			return "", fmt.Errorf("texts.json has no %s", code)
		}
		fmt.Fprintf(&b, "!insertmacro MUI_LANGUAGE %q\r\n", languageName(language["nsis"]))
	}
	for _, code := range languages {
		language := texts[code]
		for _, key := range keys {
			text, ok := language[key]
			if !ok || text == "" {
				return "", fmt.Errorf("texts.json has no %s in %s", key, code)
			}
			fmt.Fprintf(&b, "LangString %s ${LANG_%s} \"%s\"\r\n", key, language["nsis"], nsisText(text))
		}
	}
	b.WriteString("Function LangCode\r\n  StrCpy $R9 en\r\n")
	for _, code := range languages[1:] {
		fmt.Fprintf(&b, "  ${If} $LANGUAGE == ${LANG_%s}\r\n    StrCpy $R9 %s\r\n  ${EndIf}\r\n", texts[code]["nsis"], code)
	}
	b.WriteString("FunctionEnd\r\n")
	b.WriteString("Function LangByName\r\n  StrCpy $R9 ${LANG_ENGLISH}\r\n")
	for _, code := range languages {
		fmt.Fprintf(&b, "  ${If} $R8 == \"%s\"\r\n    StrCpy $R9 ${LANG_%s}\r\n  ${EndIf}\r\n", nsisText(texts[code]["name"]), texts[code]["nsis"])
	}
	b.WriteString("FunctionEnd\r\n!macro FillLanguages COMBO\r\n")
	for _, code := range languages {
		fmt.Fprintf(&b, "  ${NSD_CB_AddString} ${COMBO} \"%s\"\r\n", nsisText(texts[code]["name"]))
	}
	b.WriteString("!macroend\r\nFunction LangName\r\n")
	for _, code := range languages {
		fmt.Fprintf(&b, "  ${If} $LANGUAGE == ${LANG_%s}\r\n    StrCpy $R9 \"%s\"\r\n  ${EndIf}\r\n", texts[code]["nsis"], nsisText(texts[code]["name"]))
	}
	b.WriteString("FunctionEnd\r\n")
	return b.String(), nil
}

// languageName is the name of a language file of NSIS: SIMPCHINESE is SimpChinese.
func languageName(nsis string) string {
	names := map[string]string{"SIMPCHINESE": "SimpChinese", "PORTUGUESEBR": "PortugueseBR"}
	if name, ok := names[nsis]; ok {
		return name
	}
	return nsis[:1] + strings.ToLower(nsis[1:])
}

// nsisText is a text as a string of NSIS: its $ and quotes kept, its lines as NSIS writes them,
// and its placeholders filled in.
func nsisText(text string) string {
	text = strings.ReplaceAll(text, "$", "$$")
	text = strings.ReplaceAll(text, `"`, `$\"`)
	text = strings.ReplaceAll(text, "\n", `$\r$\n`)
	for placeholder, value := range placeholders {
		text = strings.ReplaceAll(text, placeholder, value)
	}
	return text
}
