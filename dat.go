package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf16"
)

// datRom is a file a DAT lists: the dump of a game, with its size and hashes.
type datRom struct {
	game   string
	name   string
	size   int64
	hashes map[string]string
}

// dat is a DAT file in the XML of Logiqx, as No-Intro and others publish them: its files
// by size.
type dat struct {
	name  string
	roms  map[int64][]datRom
	count int
}

// errClrmamepro is told for a DAT in the text format of clrmamepro, which is not XML.
var errClrmamepro = errors.New("the DAT is in the text format of clrmamepro: only DATs in the XML of Logiqx can be read, as No-Intro publishes them")

// loadDat reads a DAT file: in UTF-8, in UTF-16 (told by its first bytes) or in ISO-8859-1 and
// windows-1252, as its XML declaration says.
func loadDat(path string) (*dat, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	input := bufio.NewReader(file)
	var text io.Reader = input
	start, _ := input.Peek(512)
	start = bytes.Clone(start)
	switch {
	case bytes.HasPrefix(start, []byte{0xEF, 0xBB, 0xBF}):
		// the mark of UTF-8, left out to see what the DAT begins with
		input.Discard(3)
		start = start[3:]
	case bytes.HasPrefix(start, []byte{0xFF, 0xFE}), bytes.HasPrefix(start, []byte{0xFE, 0xFF}),
		bytes.HasPrefix(start, []byte{'<', 0}), bytes.HasPrefix(start, []byte{0, '<'}):
		all, err := io.ReadAll(input)
		if err != nil {
			return nil, err
		}
		decoded := []byte(fromUTF16(all))
		text = bytes.NewReader(decoded)
		start = decoded[:min(len(decoded), 512)]
	}
	if begin := bytes.ToLower(bytes.TrimSpace(start)); bytes.HasPrefix(begin, []byte("clrmamepro")) || bytes.HasPrefix(begin, []byte("game (")) {
		return nil, errClrmamepro
	}
	result := &dat{roms: map[int64][]datRom{}}
	decoder := xml.NewDecoder(text)
	decoder.CharsetReader = datCharset
	game := ""
	inHeader := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("the DAT cannot be read: %w", err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "header":
				inHeader = true
			case "name":
				if inHeader && result.name == "" {
					var name string
					if decoder.DecodeElement(&name, &element) == nil {
						result.name = strings.TrimSpace(name)
					}
				}
			case "game", "machine":
				game = attribute(element, "name")
			case "rom":
				size, err := strconv.ParseInt(attribute(element, "size"), 10, 64)
				if err != nil || size <= 0 {
					continue
				}
				rom := datRom{game: game, name: attribute(element, "name"), size: size, hashes: map[string]string{}}
				for _, kind := range []string{"crc", "md5", "sha1", "sha256"} {
					if value := strings.ToLower(attribute(element, kind)); value != "" {
						rom.hashes[kind] = value
					}
				}
				if len(rom.hashes) > 0 {
					result.roms[size] = append(result.roms[size], rom)
					result.count++
				}
			}
		case xml.EndElement:
			if element.Name.Local == "header" {
				inHeader = false
			}
		}
	}
	if result.count == 0 {
		return nil, errors.New("the DAT lists no file with its hashes")
	}
	return result, nil
}

// fromUTF16 turns text in UTF-16 into a string: little endian, unless its mark or its first
// letter tells big endian. The mark is left out.
func fromUTF16(data []byte) string {
	order := binary.ByteOrder(binary.LittleEndian)
	if bytes.HasPrefix(data, []byte{0xFE, 0xFF}) || bytes.HasPrefix(data, []byte{0, '<'}) {
		order = binary.BigEndian
	}
	units := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		units = append(units, order.Uint16(data[i:]))
	}
	if len(units) > 0 && units[0] == 0xFEFF {
		units = units[1:]
	}
	return string(utf16.Decode(units))
}

// windows1252 are the letters windows-1252 has from 0x80 to 0x9F, where ISO-8859-1 has control
// characters that DATs do not use: a DAT said to be in ISO-8859-1 is read as windows-1252, as
// browsers read such pages.
var windows1252 = [32]rune{
	'€', 0x81, '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', 0x8D, 'Ž', 0x8F,
	0x90, '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', 0x9D, 'ž', 'Ÿ',
}

// datCharset reads the rest of a DAT in the encoding its XML declaration says.
func datCharset(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(charset) {
	case "iso-8859-1", "iso8859-1", "iso_8859-1", "latin1", "latin-1", "l1", "windows-1252", "cp1252", "us-ascii", "ascii":
		data, err := io.ReadAll(input)
		if err != nil {
			return nil, err
		}
		text := make([]rune, len(data))
		for i, b := range data {
			text[i] = rune(b)
			if b >= 0x80 && b < 0xA0 {
				text[i] = windows1252[b-0x80]
			}
		}
		return strings.NewReader(string(text)), nil
	case "utf-16", "utf-16le", "utf-16be", "unicode":
		// turned into UTF-8 already, told by its first bytes
		return input, nil
	}
	return nil, errors.New("DATs in UTF-8, UTF-16, ISO-8859-1 and windows-1252 can be read")
}

func attribute(element xml.StartElement, name string) string {
	for _, attr := range element.Attr {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

// match tells whether a file is a dump the DAT lists: by its size first, then by its hashes,
// taken in one reading of the file.
func (d *dat) match(ctx context.Context, path string, progress func(done int64, total int64)) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	candidates := d.roms[info.Size()]
	if len(candidates) == 0 {
		return "not in the DAT: no dump of its size", nil
	}
	needed := map[string]bool{}
	for _, rom := range candidates {
		for kind := range rom.hashes {
			needed[kind] = true
		}
	}
	hashes := map[string]hash.Hash{}
	writers := []io.Writer{}
	for kind, make := range map[string]func() hash.Hash{"crc": func() hash.Hash { return crc32.NewIEEE() }, "md5": md5.New, "sha1": sha1.New, "sha256": sha256.New} {
		if needed[kind] {
			hashes[kind] = make()
			writers = append(writers, hashes[kind])
		}
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if err := copyWithProgress(ctx, io.MultiWriter(writers...), file, info.Size(), progress); err != nil {
		return "", err
	}
	sums := map[string]string{}
	for kind, h := range hashes {
		sums[kind] = hex.EncodeToString(h.Sum(nil))
	}
	for _, rom := range candidates {
		agree, differ := 0, 0
		for kind, value := range rom.hashes {
			if sums[kind] == value {
				agree++
			} else {
				differ++
			}
		}
		if agree > 0 && differ == 0 {
			return "in the DAT: " + rom.game, nil
		}
	}
	return fmt.Sprintf("not in the DAT: %d dumps of its size, with other hashes", len(candidates)), nil
}
