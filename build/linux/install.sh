#!/bin/sh
# Installs NXPress for this user: the program in ~/.local/bin, and its icon and its entry in
# the menu of applications. With --uninstall, takes them away again.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
data="${XDG_DATA_HOME:-$HOME/.local/share}"
bin="$HOME/.local/bin"
icon="$data/icons/hicolor/256x256/apps/nxpress.png"
entry="$data/applications/nxpress.desktop"

refresh() {
	command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$data/applications" >/dev/null 2>&1 || true
	command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -q -t "$data/icons/hicolor" >/dev/null 2>&1 || true
}

if [ "${1:-}" = "--uninstall" ]; then
	rm -f "$bin/nxpress" "$icon" "$entry"
	refresh
	echo "NXPress is uninstalled."
	exit 0
fi

mkdir -p "$bin" "$(dirname "$icon")" "$(dirname "$entry")"
install -m 755 "$here/nxpress" "$bin/nxpress"
install -m 644 "$here/nxpress.png" "$icon"
# the entry starts the program where it was installed, also when ~/.local/bin is not on the PATH
sed "s|^Exec=nxpress|Exec=$bin/nxpress|" "$here/nxpress.desktop" > "$entry"
chmod 644 "$entry"
refresh
echo "NXPress is installed: it is in the menu of applications, and $bin/nxpress on the command line."
case ":$PATH:" in
	*":$bin:"*) ;;
	*) echo "Add $bin to your PATH to call nxpress by its name." ;;
esac
