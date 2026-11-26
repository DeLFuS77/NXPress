package jobs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DeLFuS77/NXPress/internal/nup"
	"github.com/DeLFuS77/NXPress/internal/titledb"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// Report is everything Details can tell of a file (brought from the edition of QuickWick, PR
// #151), as plain text a person can read, copy or keep
// in a .txt: the file, then its titles (a game, its updates and DLC) with ID, version, name,
// publisher and what they hold, then how it is stored and what reads it. It is the text of the
// "Full details" of a file in Details and of the export to .txt. A file that cannot be read gives
// the error, as the other works do.
func Report(path string, versions *titledb.Versions) (string, error) {
	var out strings.Builder
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	line := func(format string, args ...any) { fmt.Fprintf(&out, format+"\n", args...) }
	line("File:    %s", filepath.Base(path))
	line("Size:    %s", bytesText(wholeSize(path, info)))
	line("Changed: %s", info.ModTime().Format("2006-01-02 15:04"))
	var body string
	if strings.EqualFold(filepath.Ext(path), ".nup") {
		body, err = nupReport(path, versions)
	} else if isNx(path) {
		body, err = nxReport(path, versions)
	} else {
		body, err = titlesReport(path, versions)
	}
	if err != nil {
		return "", err
	}
	out.WriteString(body)
	return out.String(), nil
}

// kindWord is the kind of a title as the report writes it: BASE, UPDATE or DLC.
func kindWord(kind string) string {
	if kind == "UPD" {
		return "UPDATE"
	}
	return kind
}

// reportLabelWidth is the width of the labels of a package in the report, so that what they tell
// starts in one column: the longest, "When Uncompressed (Total):", and two spaces.
const reportLabelWidth = 28

// idLabel is the label of the ID of a package in the report: "Base ID:", "Update ID:", "DLC ID:".
func idLabel(kind string) string {
	switch kind {
	case "BASE":
		return "Base ID:"
	case "UPD":
		return "Update ID:"
	}
	return kindWord(kind) + " ID:"
}

// cutPercent is part of whole as a percent with two decimals, the rest cut off and not rounded, so
// that it never says more than there is: 51.98796% is 51.98%. It uses whole numbers, not floats.
func cutPercent(part, whole int64) string {
	if whole <= 0 {
		return "0.00%"
	}
	hundredths := part * 10000 / whole
	sign := ""
	if hundredths < 0 {
		sign, hundredths = "-", -hundredths
	}
	return fmt.Sprintf("%s%d.%02d%%", sign, hundredths/100, hundredths%100)
}

// bytesText is a size as a person reads it, and its bytes: "14.1 GB (15,111,444,614 bytes)".
func bytesText(n int64) string {
	return fmt.Sprintf("%s (%s bytes)", size(n), grouped(n))
}

// grouped writes a number with its thousands apart: 15,111,444,614.
func grouped(n int64) string {
	text := strconv.FormatInt(n, 10)
	negative := strings.HasPrefix(text, "-")
	text = strings.TrimPrefix(text, "-")
	for i := len(text) - 3; i > 0; i -= 3 {
		text = text[:i] + "," + text[i:]
	}
	if negative {
		return "-" + text
	}
	return text
}

// reportName is the name and the publisher a game gives: in English when it has them, else in
// the first language that has a name.
func reportName(nacp *switchfs.Nacp) (string, string) {
	if nacp == nil {
		return "", ""
	}
	if title, ok := nacp.TitleName["AmericanEnglish"]; ok && strings.TrimSpace(title.Title) != "" {
		return strings.TrimSpace(title.Title), strings.TrimSpace(title.Publisher)
	}
	languages := make([]string, 0, len(nacp.TitleName))
	for language := range nacp.TitleName {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	for _, language := range languages {
		if title := nacp.TitleName[language]; strings.TrimSpace(title.Title) != "" {
			return strings.TrimSpace(title.Title), strings.TrimSpace(title.Publisher)
		}
	}
	return "", ""
}

// titlesReport tells the titles of an NSP, NSZ, XCI or XCZ, then how the file is stored.
func titlesReport(path string, versions *titledb.Versions) (string, error) {
	titles, err := ReadTitles(path)
	if err != nil {
		return "", fmt.Errorf("cannot be read: %w", err)
	}
	sort.Slice(titles, func(i, j int) bool { return titles[i].TitleId < titles[j].TitleId })
	// a DLC has no name of its own: it takes the one of its game when the file holds it
	games := map[uint64]string{}
	for _, title := range titles {
		name, _ := reportName(title.Ncap)
		if game, err := switchfs.GameTitleID(title.TitleId, title.Type); err == nil && name != "" && title.Type != "DLC" {
			games[game] = name
		}
	}
	var out strings.Builder
	fmt.Fprintf(&out, "\nTitles: %d\n", len(titles))
	for i, title := range titles {
		name, publisher := reportName(title.Ncap)
		if game, err := switchfs.GameTitleID(title.TitleId, title.Type); err == nil && name == "" {
			name = games[game]
		}
		fmt.Fprintf(&out, "\n%d. %s · %s\n", i+1, kindWord(title.Type), strings.ToUpper(title.TitleId))
		if name != "" {
			fmt.Fprintf(&out, "   Name:       %s\n", name)
		}
		if publisher != "" {
			fmt.Fprintf(&out, "   Publisher:  %s\n", publisher)
		}
		version := fmt.Sprintf("%d", title.Version)
		if title.Ncap != nil && strings.TrimSpace(title.Ncap.DisplayVersion) != "" {
			version += " (shown as " + strings.TrimSpace(title.Ncap.DisplayVersion) + ")"
		}
		fmt.Fprintf(&out, "   Version:    %s\n", version)
		if published := strings.TrimSpace(publishedLine(title, versions)); published != "" {
			fmt.Fprintf(&out, "   %s\n", published)
		}
		for _, detail := range titleDetails(title) {
			fmt.Fprintf(&out, "   %s\n", strings.TrimSpace(detail))
		}
	}
	if stored := describe(path); len(stored) > 0 {
		out.WriteString("\nStorage:\n")
		for _, detail := range stored {
			fmt.Fprintf(&out, "   %s\n", detail)
		}
	}
	if origin, ok := ReadOrigin(path); ok {
		out.WriteString("\nPackage:\n")
		fmt.Fprintf(&out, "   %s\n", OriginLine(origin))
	}
	if npdm, ok := ReadPermissions(path); ok {
		out.WriteString("\nPermissions (main.npdm):\n")
		for _, line := range permissionsReport(npdm) {
			fmt.Fprintf(&out, "   %s\n", line)
		}
	}
	return out.String(), nil
}

// nupReport tells what a NUP holds, package by package, with the totals of the file in the way
// NSP Box tells them: the files uncompressed, what is stored once, the file itself, and how
// many files are compressed and how many are raw.
func nupReport(path string, versions *titledb.Versions) (string, error) {
	container, err := nup.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot be read: %w", err)
	}
	if len(container.Packages) == 0 {
		return "", errors.New("it holds no package")
	}
	var out strings.Builder
	fmt.Fprintf(&out, "\nPackages: %d\n", len(container.Packages))
	for i, pkg := range container.Packages {
		fmt.Fprintf(&out, "\n%d. %-*s%s\n", i+1, reportLabelWidth, idLabel(pkg.KindName()), pkg.TitleIDText())
		if name := pkg.Name(); name != "" {
			fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Name:", name)
		}
		if publisher := pkg.Publisher(); publisher != "" {
			fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Publisher:", publisher)
		}
		version := fmt.Sprintf("%d", pkg.Version)
		if shown := pkg.DisplayVersion(); shown != "" {
			version += " (shown as " + shown + ")"
		}
		fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Version:", version)
		summary, err := container.Summarize(i)
		if err != nil {
			return "", fmt.Errorf("cannot be read: %w", err)
		}
		fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Files:", grouped(int64(summary.Files)))
		fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "When Uncompressed (Total):", bytesText(int64(summary.Size)))
		fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "When Compressed (Total):", bytesText(int64(summary.Stored)))
	}
	totals := container.Totals()
	compressed, raw, extents := 0, 0, map[uint64]bool{}
	for _, file := range container.Files {
		if file.Size == 0 {
			continue
		}
		if file.Codec == nup.CodecZstd {
			compressed++
		} else {
			raw++
		}
		extents[file.ExtentOffset] = true
	}
	out.WriteString("\nTotals:\n")
	fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Files:", grouped(int64(totals.Files)))
	if icon := container.Icon(); len(icon) > 0 {
		fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Icon:", "Yes ("+size(int64(len(icon)))+")")
	} else {
		fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Icon:", "No")
	}
	fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Original File Bytes:", bytesText(int64(totals.Size)))
	fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Unique Stored Data:", bytesText(int64(totals.Stored)))
	fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "NUP File Size:", bytesText(container.FileSize))
	fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Compressed Files:", grouped(int64(compressed)))
	fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Raw Files:", grouped(int64(raw)))
	fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Unique Extents:", grouped(int64(len(extents))))
	if totals.Size > 0 {
		fmt.Fprintf(&out, "   %-*s%s\n", reportLabelWidth, "Storage Saved:", cutPercent(int64(totals.Size)-container.FileSize, int64(totals.Size)))
	}
	// the section of the emulators is told only when something keeps them from the NUP
	if note := emulatorsNote(container); !strings.HasPrefix(note, "it meets what Eglantine and Lichii ask") {
		out.WriteString("\nCompatibility:\n")
		fmt.Fprintf(&out, "   %s\n", note)
	}
	return out.String(), nil
}

// ReportFiles joins the reports of the files given, in their order, each under a line with its
// number, the way the export to .txt writes them. A file that cannot be read gets its place and
// the reason; the others are not held back by it.
func ReportFiles(paths []string, versions *titledb.Versions) string {
	var out strings.Builder
	fmt.Fprintf(&out, "NXPress Details · %s · %d %s\n", time.Now().Format("2006-01-02 15:04"), len(paths), map[bool]string{true: "file", false: "files"}[len(paths) == 1])
	rule := strings.Repeat("=", 72)
	for i, path := range paths {
		fmt.Fprintf(&out, "\n%s\n[%d/%d]\n%s\n", rule, i+1, len(paths), rule)
		text, err := Report(path, versions)
		if err != nil {
			fmt.Fprintf(&out, "File:    %s\nProblem: %s\n", filepath.Base(path), err)
			continue
		}
		out.WriteString(text)
	}
	return out.String()
}
