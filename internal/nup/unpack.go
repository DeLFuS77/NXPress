package nup

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/klauspost/compress/zstd"
)

// The unpacking of a NUP into a folder, in the layout the packer of the format reads back: the
// game in base, each update in updates/<version>, each DLC in dlc/<title ID>/<index>. Every
// file is checked by its SHA-256 as it is written.

// PackageFolder is the folder of a package in an unpacked NUP, relative to it.
func (c *Container) PackageFolder(pkg int) string {
	p := c.Packages[pkg]
	switch p.Kind {
	case KindBase:
		return "base"
	case KindUpdate:
		return filepath.Join("updates", strconv.FormatUint(uint64(p.Version), 10))
	case KindAddon:
		index := 0
		for i := 0; i < pkg; i++ {
			if c.Packages[i].Kind == KindAddon && c.Packages[i].TitleID == p.TitleID {
				index++
			}
		}
		return filepath.Join("dlc", fmt.Sprintf("%016x", p.TitleID), strconv.Itoa(index))
	}
	return filepath.Join(fmt.Sprintf("kind%d", p.Kind), fmt.Sprintf("%016x", p.TitleID))
}

// UnpackResult is what an unpacked NUP holds.
type UnpackResult struct {
	Files int
	Size  int64
}

// Unpack writes the packages of a NUP into dest, a folder that must not be there yet; a file
// whose data is not what the table says fails the work, and the folder it made is removed.
// progress is told the bytes written of the ones to write.
func Unpack(ctx context.Context, path string, dest string, workers int, progress func(done int64, total int64)) (result UnpackResult, err error) {
	source, err := os.Open(path)
	if err != nil {
		return UnpackResult{}, err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return UnpackResult{}, err
	}
	container, err := Read(source, info.Size())
	if err != nil {
		return UnpackResult{}, err
	}
	if err := os.Mkdir(dest, 0o755); err != nil {
		return UnpackResult{}, err
	}
	// the folder made here, and only it, goes when the work fails
	defer func() {
		if err != nil {
			os.RemoveAll(dest)
		}
	}()
	type job struct {
		path string
		file int
	}
	jobs := []job{}
	folders := []string{}
	total := int64(0)
	for i, pkg := range container.Packages {
		if pkg.Kind > KindAddon {
			return UnpackResult{}, fmt.Errorf("NUP package %d has kind %d, which NUP version 1 keeps for later", i, pkg.Kind)
		}
		root := filepath.Join(dest, container.PackageFolder(i))
		folders = append(folders, root)
		err := container.walkFolders(i, func(folder string) error {
			if err := safePath(folder); err != nil {
				return err
			}
			folders = append(folders, filepath.Join(root, filepath.FromSlash(folder)))
			return nil
		})
		if err != nil {
			return UnpackResult{}, err
		}
		err = container.Walk(i, func(name string, index int) error {
			if err := safePath(name); err != nil {
				return err
			}
			jobs = append(jobs, job{filepath.Join(root, filepath.FromSlash(name)), index})
			total += int64(container.Files[index].Size)
			return nil
		})
		if err != nil {
			return UnpackResult{}, err
		}
	}
	for _, folder := range folders {
		if err := os.MkdirAll(folder, 0o755); err != nil {
			return UnpackResult{}, err
		}
	}
	// the files in the order their data is stored, so the NUP is read from start to end
	sort.SliceStable(jobs, func(a, b int) bool {
		return container.Files[jobs[a].file].ExtentOffset < container.Files[jobs[b].file].ExtentOffset
	})

	var (
		done    atomic.Int64
		lock    sync.Mutex
		failure error
		wait    sync.WaitGroup
	)
	tell := func(n int64) {
		value := done.Add(n)
		if progress != nil && lock.TryLock() {
			progress(value, total)
			lock.Unlock()
		}
	}
	if progress != nil {
		progress(0, total)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	work := make(chan job)
	var failed sync.Once
	for range max(1, min(workers, len(jobs))) {
		wait.Add(1)
		go func() {
			defer wait.Done()
			var decoder *zstd.Decoder
			defer func() {
				if decoder != nil {
					decoder.Close()
				}
			}()
			buffer := make([]byte, 1<<20)
			for j := range work {
				if ctx.Err() != nil {
					continue
				}
				file := container.Files[j.file]
				if file.Codec == CodecZstd && decoder == nil {
					made, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxWindow(maxWindow),
						zstd.WithDecoderMaxMemory(maxWindow))
					if err != nil {
						failed.Do(func() { failure = err })
						cancel()
						continue
					}
					decoder = made
				}
				if err := writeFile(ctx, source, file, j.path, decoder, buffer, tell); err != nil {
					failed.Do(func() { failure = err })
					cancel()
				}
			}
		}()
	}
	for _, j := range jobs {
		if ctx.Err() != nil {
			break
		}
		work <- j
	}
	close(work)
	wait.Wait()
	if failure != nil {
		return UnpackResult{}, failure
	}
	if err := ctx.Err(); err != nil {
		return UnpackResult{}, err
	}
	if progress != nil {
		progress(total, total)
	}
	return UnpackResult{Files: len(jobs), Size: total}, nil
}

// writeFile writes one file of a NUP, decompressed, and checks it by its SHA-256.
func writeFile(ctx context.Context, source *os.File, file File, path string, decoder *zstd.Decoder, buffer []byte,
	tell func(int64)) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	hash := sha256.New()
	var data io.Reader = &ctxReader{ctx: ctx, r: io.NewSectionReader(source, int64(file.ExtentOffset), int64(file.ExtentSize))}
	if file.Size == 0 {
		data = &io.LimitedReader{}
	} else if file.Codec == CodecZstd {
		if err := decoder.Reset(data); err != nil {
			out.Close()
			return fmt.Errorf("%s: its zstd data cannot be decompressed (%v)", path, err)
		}
		data = io.LimitReader(decoder, int64(file.Size)+1)
	}
	written := int64(0)
	for {
		n, err := data.Read(buffer)
		if n > 0 {
			if _, werr := out.Write(buffer[:n]); werr != nil {
				out.Close()
				return werr
			}
			hash.Write(buffer[:n])
			written += int64(n)
			tell(int64(n))
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			out.Close()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("%s: its data cannot be read (%v)", path, err)
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	switch {
	case written != int64(file.Size):
		return fmt.Errorf("%s: its data gives %d bytes instead of %d", path, written, file.Size)
	case sum != file.Hash:
		return fmt.Errorf("%s: its SHA-256 does not match", path)
	}
	return nil
}

// walkFolders calls fn for every folder of a package, by its path inside it, empty ones too.
func (c *Container) walkFolders(pkg int, fn func(path string) error) error {
	type frame struct {
		dir    uint32
		prefix string
		depth  int
	}
	seen := make([]bool, len(c.Dirs))
	stack := []frame{{dir: c.Packages[pkg].RootDir}}
	seen[c.Packages[pkg].RootDir] = true
	dirCount := uint32(len(c.Dirs))
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		dir := c.Dirs[top.dir]
		for slot := dir.FirstChild; slot < dir.FirstChild+dir.ChildCount; slot++ {
			node := c.Children[slot]
			if node >= dirCount || seen[node] {
				continue
			}
			if top.depth+1 >= maxDepth {
				return errors.New("NUP folders go too deep")
			}
			seen[node] = true
			path := top.prefix + c.Dirs[node].Name
			if err := fn(path); err != nil {
				return err
			}
			stack = append(stack, frame{dir: node, prefix: path + "/", depth: top.depth + 1})
		}
	}
	return nil
}

// windowsNames are the names Windows keeps for its devices, which no file can take.
var windowsNames = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true}

// safePath tells whether a path of a NUP can be written as it is under the folder of a package:
// names are labels in the format, never paths, so one that climbs out of its folder, or that the
// system cannot take, fails the work instead of going anywhere else.
func safePath(path string) error {
	for _, name := range strings.Split(path, "/") {
		bad := name == "" || name == "." || name == ".." || strings.ContainsAny(name, "\\\x00")
		for _, r := range name {
			if r < 0x20 {
				bad = true
			}
		}
		if runtime.GOOS == "windows" {
			base, _, _ := strings.Cut(strings.ToUpper(name), ".")
			bad = bad || strings.ContainsAny(name, `<>:"|?*`) || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") ||
				windowsNames[strings.TrimRight(base, " ")]
		}
		if bad {
			return fmt.Errorf("NUP holds a file or folder whose name cannot be written here: %q", path)
		}
	}
	return nil
}
