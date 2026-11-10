package main

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The installer of macOS: a package (.pkg) the Installer of macOS opens, with a welcome page that
// carries the notice, the license, the place (for this user, in ~/Applications, or for all, in
// /Applications) and a last page, in the language of the Mac. It puts NXPress.app and Uninstall
// NXPress.app in Applications. It is made on a Mac, with pkgbuild and productbuild.
//
//	go run ./tools/installer -mac -app out/NXPress.app -arch arm64 -version 1.7.8 -out dist/nxpress-macos-apple-silicon.pkg

// macKeys are the texts the package and its uninstaller use besides the ones of Windows
var macKeys = []string{"MacFinish", "MacOpen", "MacUnAsk", "MacUnKeep", "MacKeep", "MacDelete", "MacCancel", "MacUninstall", "MacDone"}

// macFolders are the folders of the languages, as macOS names them
var macFolders = map[string]string{"pt": "pt_BR", "zh": "zh_CN"}

const bundleID = "io.github.delfus77.nxpress"

func runMac(app, arch, version, out string, texts map[string]map[string]string) error {
	if app == "" || out == "" || (arch != "arm64" && arch != "x86_64") {
		return fmt.Errorf("give -app, -arch (arm64 or x86_64) and -out")
	}
	work, err := os.MkdirTemp("", "nxpress-pkg-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	root := filepath.Join(work, "root")
	resources := filepath.Join(work, "resources")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	// ditto keeps the signature of the app
	if err := run("ditto", app, filepath.Join(root, "NXPress.app")); err != nil {
		return err
	}
	if err := writeUninstaller(filepath.Join(root, "Uninstall NXPress.app"), app, version, texts); err != nil {
		return err
	}
	if err := writeMacResources(resources, version, texts); err != nil {
		return err
	}
	// the apps are put where the package says, not where an older copy was moved to
	components := filepath.Join(work, "components.plist")
	if err := run("pkgbuild", "--analyze", "--root", root, components); err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		if err := run("plutil", "-replace", fmt.Sprintf("%d.BundleIsRelocatable", i), "-bool", "NO", components); err != nil {
			return err
		}
	}
	component := filepath.Join(work, "nxpress.pkg")
	if err := run("pkgbuild", "--root", root, "--component-plist", components, "--identifier", bundleID,
		"--version", version, "--install-location", "/Applications", component); err != nil {
		return err
	}
	distribution := filepath.Join(work, "distribution.xml")
	if err := os.WriteFile(distribution, []byte(distributionXML(arch, version)), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return run("productbuild", "--distribution", distribution, "--resources", resources, "--package-path", work, out)
}

func run(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// distributionXML tells the Installer of macOS its pages, its background, where NXPress can go
// (for this user without a password, or for all), and the processor of the package.
func distributionXML(arch, version string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<installer-gui-script minSpecVersion="2">
    <title>NXPress</title>
    <welcome file="welcome.html" mime-type="text/html"/>
    <license file="LICENSE.txt"/>
    <conclusion file="conclusion.html" mime-type="text/html"/>
    <background file="background.png" alignment="bottomleft" scaling="none"/>
    <background-darkAqua file="background.png" alignment="bottomleft" scaling="none"/>
    <domains enable_anywhere="false" enable_currentUserHome="true" enable_localSystem="true"/>
    <options customize="never" require-scripts="false" hostArchitectures="` + arch + `"/>
    <os-version min="13.0"/>
    <choices-outline>
        <line choice="nxpress"/>
    </choices-outline>
    <choice id="nxpress" visible="false" title="NXPress">
        <pkg-ref id="` + bundleID + `"/>
    </choice>
    <pkg-ref id="` + bundleID + `" version="` + version + `" onConclusion="none">nxpress.pkg</pkg-ref>
</installer-gui-script>
`
}

// macText is a text of texts.json for a page of the Installer: its placeholders filled in, in HTML.
func macText(text, version string) string {
	text = strings.ReplaceAll(text, "{VERSION}", version)
	return strings.ReplaceAll(html.EscapeString(text), "\n", "<br>")
}

// writeMacResources writes the pages of each language, the license and the background.
func writeMacResources(resources, version string, texts map[string]map[string]string) error {
	page := func(body string) string {
		return `<!doctype html><html><head><meta charset="utf-8"><style>
body { font: 13px -apple-system, "Helvetica Neue", sans-serif; }
h2 { font-size: 15px; }
.notice { border-left: 3px solid #0a6fb0; padding: 6px 10px; background: rgba(10, 111, 176, .08); }
</style></head><body>` + body + `</body></html>`
	}
	for _, code := range languages {
		t := texts[code]
		folder := filepath.Join(resources, macFolder(code)+".lproj")
		if err := os.MkdirAll(folder, 0o755); err != nil {
			return err
		}
		welcome := page("<h2>" + macText(t["WelcomeTitle"], version) + "</h2><p>" + macText(t["WelcomeText"], version) +
			"</p><p class=\"notice\">" + macText(t["Notice"], version) + "</p>")
		finish := page("<h2>" + macText(t["FinishTitle"], version) + "</h2><p>" + macText(t["MacFinish"], version) + "</p><p>" +
			macText(t["MacOpen"], version) + "</p><p>" + macText(t["FinishText"], version) + "</p>")
		for name, data := range map[string]string{"welcome.html": welcome, "conclusion.html": finish} {
			if err := os.WriteFile(filepath.Join(folder, name), []byte(data), 0o644); err != nil {
				return err
			}
		}
	}
	license, err := os.ReadFile("LICENSE")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(resources, "LICENSE.txt"), license, 0o644); err != nil {
		return err
	}
	background, err := os.ReadFile(filepath.Join("build", "macos", "installer", "background.png"))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(resources, "background.png"), background, 0o644)
}

func macFolder(code string) string {
	if folder, ok := macFolders[code]; ok {
		return folder
	}
	return code
}

// writeUninstaller makes Uninstall NXPress.app: a script that asks, in the language of the Mac,
// whether to uninstall and whether to keep the settings, then takes NXPress and itself away, as
// the administrator when they are in /Applications. With --yes [--keep] it asks nothing, for the
// tests.
func writeUninstaller(app, nxpress, version string, texts map[string]map[string]string) error {
	if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(app, "Contents", "Resources"), 0o755); err != nil {
		return err
	}
	icon, err := os.ReadFile(filepath.Join(nxpress, "Contents", "Resources", "icon.icns"))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Resources", "icon.icns"), icon, 0o644); err != nil {
		return err
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>Uninstall NXPress</string>
	<key>CFBundleIdentifier</key><string>` + bundleID + `.uninstall</string>
	<key>CFBundleExecutable</key><string>uninstall</string>
	<key>CFBundleIconFile</key><string>icon</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleVersion</key><string>` + version + `</string>
	<key>CFBundleShortVersionString</key><string>` + version + `</string>
	<key>LSMinimumSystemVersion</key><string>13.0</string>
</dict>
</plist>
`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(app, "Contents", "MacOS", "uninstall"), []byte(uninstallScript(texts)), 0o755)
}

// shellQuote is a text in single quotes for the shell, and inside it a string of AppleScript in
// double quotes.
func shellQuote(text string) string {
	text = strings.ReplaceAll(text, `\`, `\\`)
	text = strings.ReplaceAll(text, `"`, `\"`)
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

func uninstallScript(texts map[string]map[string]string) string {
	var b strings.Builder
	b.WriteString(`#!/bin/bash
# Uninstall NXPress: written by tools/installer. It takes away NXPress.app and itself from the
# folder they are in and, unless they are kept, the settings of NXPress.
set -u
language=$(defaults read -g AppleLanguages 2>/dev/null | sed -n 2p | tr -d ' ",' | cut -c1-2)
case "$language" in
`)
	for _, code := range languages[1:] {
		fmt.Fprintf(&b, "  %s) ;;\n", code)
	}
	b.WriteString("  *) language=en ;;\nesac\ncase \"$language\" in\n")
	for _, code := range languages {
		t := texts[code]
		fmt.Fprintf(&b, "  %s)\n", code)
		for _, key := range []string{"UnTitle", "MacUnAsk", "MacUnKeep", "MacKeep", "MacDelete", "MacCancel", "MacUninstall", "MacDone"} {
			fmt.Fprintf(&b, "    %s=%s\n", key, shellQuote(t[key]))
		}
		b.WriteString("    ;;\n")
	}
	b.WriteString(`esac
here=$(cd "$(dirname "$0")/../../.." && pwd)
this=$(cd "$(dirname "$0")/../.." && pwd)
keep=1
if [ "${1:-}" = "--yes" ]; then
  [ "${2:-}" = "--keep" ] || keep=0
else
  osascript -e "display dialog \"$MacUnAsk\" with title \"$UnTitle\" buttons {\"$MacCancel\", \"$MacUninstall\"} default button 2 cancel button 1 with icon caution" > /dev/null || exit 0
  answer=$(osascript -e "button returned of (display dialog \"$MacUnKeep\" with title \"$UnTitle\" buttons {\"$MacDelete\", \"$MacKeep\"} default button 2)") || exit 0
  [ "$answer" = "$MacKeep" ] || keep=0
fi
pkill -x nxpress 2> /dev/null
remove="rm -rf '$here/NXPress.app' '$this'"
if [ -w "$here" ]; then
  eval "$remove"
else
  osascript -e "do shell script \"$remove\" with administrator privileges" > /dev/null || exit 1
fi
pkgutil --forget ` + bundleID + ` > /dev/null 2>&1
pkgutil --volume "$HOME" --forget ` + bundleID + ` > /dev/null 2>&1
if [ "$keep" = 0 ]; then
  rm -rf "$HOME/Library/Application Support/NXPress" "$HOME/Library/WebKit/` + bundleID + `" "$HOME/Library/Caches/` + bundleID + `"
fi
[ "${1:-}" = "--yes" ] || osascript -e "display dialog \"$MacDone\" with title \"$UnTitle\" buttons {\"OK\"} default button 1" > /dev/null
exit 0
`)
	return b.String()
}
