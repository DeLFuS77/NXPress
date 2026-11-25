package jobs

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/titledb"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// .nx files (github.com/fab918/nxformat) hold decrypted games for emulators, like NUP files: a
// game, its updates and its DLC in one file. NXPress shows what they hold (Details), from their
// catalog, without keys; the other works leave them as they are, and say so.

// NxNotDone is why a .nx given to any other work than Details is left as it is.
const NxNotDone = "this work leaves .nx files as they are: Details (-i) shows what they hold"

// isNx tells a .nx file by the extension of its name.
func isNx(path string) bool {
	return switchfs.IsNxFile(path)
}

// nxKind is the kind of a version of a .nx as NXPress writes it: BASE, UPD or DLC.
func nxKind(snapshot switchfs.NxSnapshot) string {
	switch snapshot.Type {
	case switchfs.NxSnapshotBase:
		return "BASE"
	case switchfs.NxSnapshotUpdate, switchfs.NxSnapshotStandaloneUpdate:
		return "UPD"
	case switchfs.NxSnapshotDlc:
		return "DLC"
	}
	return "?"
}

// nxPlatform is the console a game of a .nx is for, as its catalog says it.
func nxPlatform(platform int) string {
	switch platform {
	case 1:
		return "Switch"
	case 2:
		return "Switch 2"
	}
	return "unknown"
}

// nxVersion is a version of a .nx as Details lists it: its title ID ("" for a DLC from an NSP,
// whose ID is in its CNMT), its kind, its version, its name and publisher, and the bytes of its
// files.
type nxVersion struct {
	id, kind, shown, name, publisher string
	version                          uint32
	size                             uint64
	platform                         int
}

func nxVersions(archive *switchfs.NxArchive) []nxVersion {
	result := []nxVersion{}
	for a := range archive.Applications {
		app := &archive.Applications[a]
		for _, snapshot := range app.Snapshots {
			v := nxVersion{id: strings.ToUpper(app.TitleId(snapshot)), kind: nxKind(snapshot), version: snapshot.Version,
				shown: snapshot.DisplayVersion, size: snapshot.LogicalSize, platform: app.Platform}
			if nacp := app.Nacp(snapshot); nacp != nil {
				v.name, v.publisher = reportName(nacp)
				if v.shown == "" {
					v.shown = strings.TrimSpace(nacp.DisplayVersion)
				}
			}
			if v.name == "" && snapshot.Type != switchfs.NxSnapshotDlc {
				v.name, v.publisher = app.Title, app.Publisher
			}
			result = append(result, v)
		}
	}
	return result
}

// infoNx says what a .nx holds: each version with its title ID, kind, version, name and size.
func infoNx(path string, out Output) (string, error) {
	archive, err := switchfs.ReadNx(path)
	if err != nil {
		return "", fmt.Errorf("cannot be read: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	lines := []string{}
	total := uint64(0)
	for _, v := range nxVersions(archive) {
		id := v.id
		if id == "" {
			id = "(ID in its CNMT)"
		}
		line := fmt.Sprintf("%s %s v%d", id, v.kind, v.version)
		if v.name != "" {
			line += " " + v.name
		}
		lines = append(lines, line)
		if v.shown != "" {
			lines = append(lines, "  version "+v.shown)
		}
		if v.publisher != "" {
			lines = append(lines, "  publisher: "+v.publisher)
		}
		lines = append(lines, "  "+size(int64(v.size))+" of files")
		total += v.size
	}
	lines = append(lines, fmt.Sprintf(".nx: %s, %s of files in its versions (the files they share are stored once)", size(info.Size()), size(int64(total))))
	lines = append(lines, "works with: emulators that read .nx files")
	out.Say("%s:\n  %s", filepath.Base(path), strings.Join(lines, "\n  "))
	return count(len(nxVersions(archive)), "title", "titles"), nil
}

// nxReport is the full report of a .nx (see Report).
func nxReport(path string, _ *titledb.Versions) (string, error) {
	archive, err := switchfs.ReadNx(path)
	if err != nil {
		return "", fmt.Errorf("cannot be read: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	versions := nxVersions(archive)
	if len(versions) == 0 {
		return "", errors.New("it holds no title")
	}
	var out strings.Builder
	fmt.Fprintf(&out, "\nTitles: %d\n", len(versions))
	total := uint64(0)
	for i, v := range versions {
		id := v.id
		if id == "" {
			id = "not in the catalog (it is in the CNMT of the DLC)"
		}
		fmt.Fprintf(&out, "\n%d. %-*s%s\n", i+1, reportLabelWidth, idLabel(v.kind), id)
		if v.name != "" {
			fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Name:", v.name)
		}
		if v.publisher != "" {
			fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Publisher:", v.publisher)
		}
		version := fmt.Sprintf("%d", v.version)
		if v.shown != "" {
			version += " (shown as " + v.shown + ")"
		}
		fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Version:", version)
		fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Console:", nxPlatform(v.platform))
		fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Files (Total):", bytesText(int64(v.size)))
		total += v.size
	}
	out.WriteString("\nTotals:\n")
	if icon, err := archive.Icon(); err == nil && len(icon) > 0 {
		fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Icon:", "Yes ("+size(int64(len(icon)))+")")
	} else {
		fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Icon:", "No")
	}
	fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Files of All Versions:", bytesText(int64(total)))
	fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, ".nx File Size:", bytesText(info.Size()))
	fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Format:", fmt.Sprintf("NX %d.%d", 0, archive.MinorVersion))
	if archive.Generation > 0 {
		fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Edits:", fmt.Sprintf("%d (updates or DLC added or taken out)", archive.Generation))
	}
	out.WriteString("\nWorks with:\n   Emulators that read .nx files. Installers do not take it.\n")
	return out.String(), nil
}

// NxDetails is what the card of a .nx in Details tells: the size of the file, the bytes of the
// files of its versions, its versions and its icon.
type NxDetails struct {
	Size         int64
	Uncompressed int64
	Packages     []NupPackage
	Icon         []byte
}

// NxCard tells what a .nx holds, for its card in Details. Its versions come as the packages of a
// NUP do, with the bytes of their files; what they take in the file is not told: they share it.
func NxCard(path string) (NxDetails, error) {
	archive, err := switchfs.ReadNx(path)
	if err != nil {
		return NxDetails{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return NxDetails{}, err
	}
	result := NxDetails{Size: info.Size()}
	if icon, err := archive.Icon(); err == nil {
		result.Icon = icon
	}
	for _, v := range nxVersions(archive) {
		result.Uncompressed += int64(v.size)
		result.Packages = append(result.Packages, NupPackage{TitleID: v.id, Type: v.kind, Version: int(v.version), Shown: v.shown,
			Name: v.name, Publisher: v.publisher, Size: int64(v.size)})
	}
	return result, nil
}

// nxTitle is the main title of a .nx, as ReadTitle tells it.
func nxTitle(path string, withIcon bool) (GameTitle, error) {
	metadata, err := switchfs.ReadNxMetadata(path)
	if err != nil {
		return GameTitle{}, err
	}
	if len(metadata) == 0 {
		return GameTitle{}, errors.New("it holds no title")
	}
	all := make([]*switchfs.ContentMetaAttributes, 0, len(metadata))
	for _, title := range metadata {
		all = append(all, title)
	}
	result := describeTitles(all)
	result.Container = "NX"
	if info, err := os.Stat(path); err == nil {
		result.Size = info.Size()
	}
	if withIcon {
		if icon, err := switchfs.ExtractIcon(path); err == nil && len(icon) > 0 {
			kind := "image/jpeg"
			if bytes.HasPrefix(icon, []byte("PNG")) {
				kind = "image/png"
			}
			result.Icon = "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(icon)
		}
	}
	return result, nil
}

// IsNx tells a .nx file by the extension of its name.
func IsNx(path string) bool {
	return isNx(path)
}
