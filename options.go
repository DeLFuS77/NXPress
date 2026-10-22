package main

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

// options are what the command line asks for. The options of nsz work the same, so NXPress
// can take its place in scripts and in the tools that call it; a few of them have no use
// here and are accepted without effect.
type options struct {
	compress   bool
	decompress bool
	verify     bool
	info       bool
	convert    bool
	recompress bool
	trim       bool
	estimate   bool
	split      bool
	pack       string
	level      int
	block      bool
	threads    int
	multi      int
	output     string
	overwrite  bool
	rmSource   bool
	keys       string
	dat        string
	datFile    *dat
	minimal    bool
	machine    bool
	version    bool
	help       bool
	files      []string
	// the options of nsz accepted without effect, told once
	ignored []string
}

const (
	defaultLevel = 18
	maxLevel     = 19
)

// the options that take a value, by every name they have
var valued = map[string]string{
	"-l": "level", "--level": "level", "-t": "threads", "--threads": "threads", "-m": "multi", "--multi": "multi",
	"-o": "output", "--output": "output", "-s": "bs", "--bs": "bs", "--keys": "keys", "--pack": "pack", "--depth": "depth", "--dat": "dat",
}

// the options of nsz that NXPress accepts and has no use for
var noEffect = map[string]string{
	"-L": "--long", "--long": "--long", "-K": "--keep", "--keep": "--keep", "-F": "--fix-padding", "--fix-padding": "--fix-padding",
	"-P": "--alwaysParseCnmt", "--alwaysParseCnmt": "--alwaysParseCnmt", "-r": "--rm-old-version", "--rm-old-version": "--rm-old-version",
}

func parseOptions(args []string) (options, error) {
	o := options{level: defaultLevel, threads: 0, multi: 1}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			o.files = append(o.files, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			o.files = append(o.files, arg)
			continue
		}
		name, value, hasValue := strings.Cut(arg, "=")
		if what, ok := valued[name]; ok {
			if !hasValue {
				if i+1 >= len(args) {
					return o, fmt.Errorf("%s needs a value", name)
				}
				i++
				value = args[i]
			}
			if err := o.set(what, name, value); err != nil {
				return o, err
			}
			continue
		}
		if hasValue {
			return o, fmt.Errorf("%s takes no value", name)
		}
		switch name {
		case "-C":
			o.compress = true
		case "-D":
			o.decompress = true
		case "-V", "--verify", "-Q", "--quick-verify":
			o.verify = true
		case "-B", "--block":
			o.block = true
		case "-S", "--solid":
			o.block = false
		case "-w", "--overwrite":
			o.overwrite = true
		case "--rm-source":
			o.rmSource = true
		case "-i", "--info":
			o.info = true
		case "--convert":
			o.convert = true
		case "--recompress":
			o.recompress = true
		case "--trim":
			o.trim = true
		case "--estimate":
			o.estimate = true
		case "--split":
			o.split = true
		case "--minimal-output":
			o.minimal = true
		case "--machine-readable":
			o.machine = true
		case "--version":
			o.version = true
		case "-h", "--help":
			o.help = true
		default:
			if long, ok := noEffect[name]; ok {
				o.ignored = append(o.ignored, long)
				continue
			}
			return o, fmt.Errorf("unknown option %s (see nxpress --help)", name)
		}
	}
	return o, nil
}

func (o *options) set(what string, name string, value string) error {
	number := func(min int) (int, error) {
		n, err := strconv.Atoi(value)
		if err != nil || n < min {
			return 0, fmt.Errorf("%s needs a number, not %q", name, value)
		}
		return n, nil
	}
	var err error
	switch what {
	case "level":
		o.level, err = number(1)
	case "threads":
		// nsz takes a number below 1 as "choose for me"
		if n, convErr := strconv.Atoi(value); convErr == nil && n < 1 {
			o.threads = 0
			return nil
		}
		o.threads, err = number(1)
	case "multi":
		o.multi, err = number(1)
	case "output":
		o.output = value
	case "keys":
		o.keys = value
	case "dat":
		o.dat = value
	case "pack":
		o.pack = value
	case "bs", "depth":
		o.ignored = append(o.ignored, name)
	}
	return err
}

// check tells an impossible request, and settles the values left to choose.
func (o *options) check() error {
	actions := 0
	for _, on := range []bool{o.compress, o.decompress, o.convert, o.recompress, o.trim, o.estimate, o.split, o.pack != "", o.info} {
		if on {
			actions++
		}
	}
	if actions > 1 {
		return errors.New("choose one of -C, -D, -i, --convert, --recompress, --trim, --estimate, --split and --pack")
	}
	if o.dat != "" && actions > 0 {
		return errors.New("--dat goes with -V: it tells whether the files checked are dumps a DAT lists")
	}
	if o.dat != "" {
		o.verify = true
	}
	if actions == 0 && !o.verify {
		return errors.New("nothing to do: use -C to compress, -D to decompress or -V to check (see nxpress --help)")
	}
	if len(o.files) == 0 {
		return errors.New("no file or folder given")
	}
	if o.level > maxLevel {
		o.ignored = append(o.ignored, fmt.Sprintf("-l %d (levels above %d make a zstd window larger than the 8 MiB installers decompress with: %d is used)", o.level, maxLevel, maxLevel))
		o.level = maxLevel
	}
	if o.threads == 0 {
		o.threads = max(1, min(8, runtime.NumCPU()/2))
	}
	return nil
}

const usage = `NXPress %s, compresses and decompresses Nintendo Switch NSP and XCI files (NSZ and XCZ)

Usage: nxpress [options] file or folder...

  -C                  compress NSP and XCI files to NSZ and XCZ
  -D                  decompress NSZ and XCZ files to NSP and XCI
  -V, --verify        check files: with -C or -D it is always done, alone it checks the
                      files given (NSP, NSZ, XCI, XCZ)
  -Q, --quick-verify  the same as -V
  --dat FILE          with -V, tell whether each file is a dump the DAT lists (the XML of
                      Logiqx, as No-Intro publishes them), by its size and hashes
  -l, --level N       compression level, 1 to 19 (default 18)
  -B, --block         block compression (always for XCZ): emulators that read NSZ run it
                      as it is; every installer takes it but Awoo
  -S, --solid         solid compression (the default for NSZ): the smallest, and every
                      installer takes it
  -t, --threads N     threads per file (default: half the processors, at most 8)
  -m, --multi N       files at once (default 1)
  -o, --output DIR    folder of the new files (default: next to each file)
  -w, --overwrite     replace a file that is already there
  --rm-source         delete each original once its new file is checked
  -i, --info          show the titles a file holds
  --estimate          tell the size and the time a compression would give, with -l, -t
                      and -B, from samples of each NSP and XCI: nothing is written
  --keys PATH         prod.keys or keys.txt, or a folder holding one (default: the folder
                      of NXPress, then ~/.switch)
  --minimal-output    print only "<percentage>%% <step>"
  --machine-readable  print one JSON object per line
  --version           show the version

  --convert           turn XCI and XCZ files into NSP and NSZ
  --recompress        write NSZ and XCZ files again, solid (-S) or in blocks (-B), at the
                      level of -l: no NSP in between and no keys needed; a file is replaced
                      once its new copy is checked, unless -o gives another folder
  --trim              take away what installers do not use: the padding after the data
                      of XCI files, as game cards are dumped, and the delta fragments of
                      updates in NSP and NSZ files; in place, or into the folder of -o
  --split             split multi-title files (SuperNSP) into one file per title
  --pack FILE         merge the files given into one multi-title file (SuperNSP): an
                      NSP when they are all NSP, an NSZ when one of them is an NSZ; of a
                      title given twice, the newest version is kept (a SuperNSP and a
                      newer update of its game give the SuperNSP with the new update);
                      with an XCI or XCZ among them, its updates and DLC go into the game
                      card, and the pack is an XCI (or an XCZ)

Multi-title files (SuperNSP) compress like any other file. The options of nsz --long,
--keep, --fix-padding, --alwaysParseCnmt, --rm-old-version, --bs and --depth are accepted
and have no effect.
`
