package main

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
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

// loadDat reads a DAT file.
func loadDat(path string) (*dat, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result := &dat{roms: map[int64][]datRom{}}
	decoder := xml.NewDecoder(file)
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
