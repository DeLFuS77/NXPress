#!/usr/bin/env bash
# Puts the program in a macOS app, NXPress.app, so it opens as a window from the Finder; the
# command line is NXPress.app/Contents/MacOS/nxpress. The app is signed ad hoc, as macOS needs
# for Apple chips: it is not notarized, so the first time it is opened with right click, Open.
#   macos_app.sh <program> <version> <folder of the app>
set -euo pipefail

program="$1"
version="$2"
app="$3/NXPress.app"

rm -rf "${app}"
mkdir -p "${app}/Contents/MacOS" "${app}/Contents/Resources"
cp "${program}" "${app}/Contents/MacOS/nxpress"
cat > "${app}/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>NXPress</string>
	<key>CFBundleDisplayName</key><string>NXPress</string>
	<key>CFBundleIdentifier</key><string>io.github.delfus77.nxpress</string>
	<key>CFBundleExecutable</key><string>nxpress</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleVersion</key><string>${version}</string>
	<key>CFBundleShortVersionString</key><string>${version}</string>
	<key>LSMinimumSystemVersion</key><string>11.0</string>
	<key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST
codesign --force --deep --sign - "${app}"
codesign --verify "${app}"
