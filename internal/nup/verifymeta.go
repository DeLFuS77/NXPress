package nup

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"sort"
	"unicode/utf8"

	"github.com/klauspost/compress/zstd"
)

// the types of the metadata the verification looks at besides the ones read for the library
const (
	tlvNpdm       = 0x0001
	tlvBuildID    = 0x0007
	tlvSourceDump = 0x0008
)

// checkMeta looks at the metadata of each package, which no hash of the format covers. A record
// that cannot be what it says is damaged: a build ID that is not 32 bytes, text that is not
// UTF-8, records of languages that run past their block. A record that differs from the files
// of its package is told in a note instead, as the format lets them differ (the files win for
// the content, the record for what is shown): an NPDM other than its exefs/main.npdm, a build
// ID other than the one of its exefs/main, an icon that is neither JPEG nor PNG.
//
// The types of the records are only sure in a block with an NPDM: the packer of the format
// counted them from 0 for a while, which a block without one does not show (a DLC has none),
// so such a block only has its records of languages looked at, which are alike either way.
// damagedFiles are the files whose data is damaged, left out of the comparisons.
func checkMeta(file *os.File, container *Container, damagedFiles []bool) (damaged []Damaged, notes []string) {
	image := func(data []byte) bool {
		return bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}) || bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
	}
	for i, pkg := range container.Packages {
		label := Label(pkg)
		bad := func(record string, reason string) {
			damaged = append(damaged, Damaged{Package: label, Path: "metadata: " + record, Reason: reason})
		}
		localized := map[uint16]map[string][]byte{}
		for _, kind := range []uint16{tlvLocalizedTitles, tlvLocalizedIcons} {
			if block, ok := pkg.Meta[kind]; ok {
				records, err := parseLocalized(block)
				if err != nil {
					bad("records of languages", err.Error())
				}
				localized[kind] = records
			}
		}
		if !bytes.HasPrefix(pkg.Meta[tlvNpdm], []byte("META")) {
			continue
		}
		files := map[string]int{}
		_ = container.Walk(i, func(path string, index int) error {
			if path == "exefs/main" || path == "exefs/main.npdm" {
				files[path] = index
			}
			return nil
		})
		if index, found := files["exefs/main.npdm"]; found && !damagedFiles[index] && sha256.Sum256(pkg.Meta[tlvNpdm]) != container.Files[index].Hash {
			notes = append(notes, label+": the NPDM of its metadata differs from exefs/main.npdm")
		}
		if id, ok := pkg.Meta[tlvBuildID]; ok {
			index, found := files["exefs/main"]
			switch {
			case len(id) != 32:
				bad("build ID", fmt.Sprintf("it is %d bytes instead of 32", len(id)))
			case found && !damagedFiles[index]:
				if head, err := fileHead(file, container.Files[index], 0x60); err == nil && len(head) == 0x60 && !bytes.Equal(head[0x40:], id) {
					notes = append(notes, label+": the build ID of its metadata differs from the one of exefs/main")
				}
			}
		}
		for _, text := range []struct {
			kind   uint16
			record string
		}{{tlvDisplayVersion, "display version"}, {tlvTitleName, "name"}, {tlvPublisher, "publisher"}, {tlvSourceDump, "source dump"}} {
			if value, ok := pkg.Meta[text.kind]; ok && !utf8.Valid(value) {
				bad(text.record, "it is not UTF-8 text")
			}
		}
		for _, tag := range sortedTags(localized[tlvLocalizedTitles]) {
			if !utf8.Valid(localized[tlvLocalizedTitles][tag]) {
				bad("names by language", "the name in "+tag+" is not UTF-8 text")
			}
		}
		if icon, ok := pkg.Meta[tlvIcon]; ok && len(icon) > 0 && !image(icon) {
			notes = append(notes, label+": its icon is neither JPEG nor PNG")
		}
		for _, tag := range sortedTags(localized[tlvLocalizedIcons]) {
			if icon := localized[tlvLocalizedIcons][tag]; len(icon) > 0 && !image(icon) {
				notes = append(notes, label+": its icon in "+tag+" is neither JPEG nor PNG")
			}
		}
	}
	return damaged, notes
}

func sortedTags(localized map[string][]byte) []string {
	tags := make([]string, 0, len(localized))
	for tag := range localized {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// fileHead reads the first n bytes of a file of a NUP, decompressed when it is stored with zstd.
func fileHead(file *os.File, f File, n int) ([]byte, error) {
	data := io.Reader(io.NewSectionReader(file, int64(f.ExtentOffset), int64(f.ExtentSize)))
	if f.Codec == CodecZstd {
		decoder, err := zstd.NewReader(data, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxWindow(maxWindow), zstd.WithDecoderMaxMemory(maxWindow))
		if err != nil {
			return nil, err
		}
		defer decoder.Close()
		data = decoder
	}
	head := make([]byte, min(int64(n), int64(f.Size)))
	if _, err := io.ReadFull(data, head); err != nil {
		return nil, err
	}
	return head, nil
}
