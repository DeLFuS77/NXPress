package main

import (
	"path/filepath"

	"github.com/DeLFuS77/NXPress/internal/space"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// SpaceCheck tells the drives where the new files of a request may not fit, before it starts.
func (a *App) SpaceCheck(request Request) []space.Short {
	return spaceShort(request, space.FreeSpace)
}

// spaceShort weighs each new file by the most it can take: as big as its original (all its
// parts, for a file in parts), and an NSZ or XCZ decompressed as big as its content.
func spaceShort(request Request, free func(string) (int64, error)) []space.Short {
	switch request.Mode {
	case "compress", "decompress", "recompress", "trim", "convert", "fat32", "split", "pack":
	default:
		return nil
	}
	replaces := request.RemoveSource || request.Output == "" && (request.Mode == "recompress" || request.Mode == "trim" || request.Mode == "fat32")
	files := []space.File{}
	for _, path := range request.Files {
		size, ok := space.OriginalSize(path)
		if !ok {
			continue
		}
		most := size
		if request.Mode == "decompress" {
			if facts, err := switchfs.DescribeFile(path); err == nil && facts.DecompressedSize > most {
				most = facts.DecompressedSize
			}
		}
		folder := filepath.Dir(path)
		switch {
		case request.Mode == "pack" && !request.PackGames && request.Pack != "":
			folder = filepath.Dir(request.Pack)
		case request.Output != "":
			folder = request.Output
		}
		drive := space.VolumeOf(folder)
		files = append(files, space.File{
			Folder: folder, Drive: drive, Size: size, Most: most,
			Replaces: replaces && request.Mode != "pack" && drive == space.VolumeOf(filepath.Dir(path)),
		})
	}
	return space.Needed(files, max(1, request.Multi), free)
}
