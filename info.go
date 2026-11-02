package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// info tells the titles a file holds; a file that cannot be read fails.
func info(path string, out output) (string, error) {
	read := switchfs.ReadNspMetadata
	if extension := strings.ToLower(filepath.Ext(path)); extension == ".xci" || extension == ".xcz" {
		read = switchfs.ReadXciMetadata
	}
	titles, err := read(path)
	if err != nil {
		return "", fmt.Errorf("cannot be read: %w", err)
	}
	ids := make([]string, 0, len(titles))
	for id := range titles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	lines := []string{}
	for _, id := range ids {
		title := titles[id]
		line := fmt.Sprintf("%s %s v%d", strings.ToUpper(title.TitleId), title.Type, title.Version)
		if title.Ncap != nil {
			if name := strings.TrimSpace(title.Ncap.TitleName["AmericanEnglish"].Title); name != "" {
				line += " " + name
			}
		}
		lines = append(lines, line)
		lines = append(lines, titleDetails(title)...)
	}
	lines = append(lines, describe(path)...)
	out.say("%s:\n  %s", filepath.Base(path), strings.Join(lines, "\n  "))
	if len(titles) == 1 {
		return "1 title", nil
	}
	return fmt.Sprintf("%d titles", len(titles)), nil
}

// shown is an error as people see it: the bytes of a damaged file quoted in it (what was read
// where a header should be) are shown as ?, not as the characters they happen to make.
func shown(err error) error {
	if err == nil {
		return nil
	}
	text := printable(err.Error())
	switch {
	case text == err.Error():
		return err
	case isSkipped(err):
		return skipped(text)
	case isModified(err):
		return modified(text)
	}
	return errors.New(text)
}

// printable is text with each byte that is not a printable character as ?.
func printable(text string) string {
	return strings.Map(func(r rune) rune {
		if r == utf8.RuneError || !unicode.IsPrint(r) {
			return '?'
		}
		return r
	}, text)
}

// contentOrder is the order the NCA files of a title are told in.
var contentOrder = []string{"Meta", "Program", "Control", "Data", "HtmlDocument", "LegalInformation", "DeltaFragment"}

// titleDetails tells what a title needs, the firmware of the console for a game or an update
// and the version of its game for a DLC, and the NCA files it is made of (#20).
func titleDetails(title *switchfs.ContentMetaAttributes) []string {
	lines := []string{}
	switch {
	case title.RequiredTitleVersion > 0 && title.Type == "DLC":
		lines = append(lines, fmt.Sprintf("  needs version %d of its game", title.RequiredTitleVersion))
	case title.RequiredTitleVersion > 0:
		lines = append(lines, "  needs firmware "+firmware(uint32(title.RequiredTitleVersion)))
	}
	for _, kind := range contentOrder {
		if content, ok := title.Contents[kind]; ok && content.ID != "" {
			lines = append(lines, fmt.Sprintf("  %s NCA %s", kind, content.ID))
		}
	}
	return lines
}

// firmware is a version of the system as the console shows it: 12.1.0 for 0x30100000.
func firmware(version uint32) string {
	return fmt.Sprintf("%d.%d.%d", version>>26, version>>20&0x3F, version>>16&0xF)
}

// describe tells how a file is stored, what it works with, and what trimming takes away.
func describe(path string) []string {
	facts, err := switchfs.DescribeFile(path)
	if err != nil {
		return nil
	}
	lines := []string{}
	switch {
	case facts.Solid > 0 && facts.Blocks > 0:
		lines = append(lines, fmt.Sprintf("%s, solid and in blocks: %s, %s uncompressed (%s smaller)", facts.Kind, size(facts.Size), size(facts.DecompressedSize), percent(facts.DecompressedSize-facts.Size, facts.DecompressedSize)))
	case facts.Solid > 0:
		lines = append(lines, fmt.Sprintf("%s, solid: %s, %s uncompressed (%s smaller)", facts.Kind, size(facts.Size), size(facts.DecompressedSize), percent(facts.DecompressedSize-facts.Size, facts.DecompressedSize)))
	case facts.Blocks > 0:
		lines = append(lines, fmt.Sprintf("%s, in blocks: %s, %s uncompressed (%s smaller)", facts.Kind, size(facts.Size), size(facts.DecompressedSize), percent(facts.DecompressedSize-facts.Size, facts.DecompressedSize)))
	default:
		lines = append(lines, fmt.Sprintf("%s, uncompressed: %s", facts.Kind, size(facts.Size)))
	}
	switch {
	case facts.Archive > 0:
		lines = append(lines, "works with: NXPress only, experimental archive format NCZSECT3: decompress it with NXPress to install it")
	case facts.Sparse > 0:
		lines = append(lines, "works with: NXPress only, experimental format NCZSECT2 for sparse storage: decompress it with NXPress to install it")
	case facts.Solid > 0:
		lines = append(lines, "works with: every installer (DBI, Tinfoil, Awoo, CyberFoil, Sphaira); emulators need it decompressed")
	case facts.Blocks > 0:
		lines = append(lines, "works with: every installer but Awoo; emulators that read NSZ run it as it is")
	default:
		lines = append(lines, "works with: every installer and emulator")
	}
	if facts.Padding > 0 {
		lines = append(lines, fmt.Sprintf("padding: %s that trimming takes away", size(facts.Padding)))
	}
	if facts.DeltaFragments > 0 {
		lines = append(lines, fmt.Sprintf("delta fragments: %s that trimming takes away", size(facts.DeltaFragments)))
	}
	return lines
}
