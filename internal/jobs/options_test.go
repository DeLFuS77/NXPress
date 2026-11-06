package jobs

import (
	"reflect"
	"strings"
	"testing"
)

// The options of nsz work as there: with their value after them or after "=", short and long,
// and the ones without use here accepted and told.
func TestParseOptions(t *testing.T) {
	o, err := ParseOptions([]string{"-C", "-l", "12", "--threads=0", "-m", "2", "-B", "-o", "out", "--keep", "--bs=22", "a.nsp", "--", "-b.nsp"})
	if err != nil {
		t.Fatal(err)
	}
	if !o.Compress || o.Level != 12 || o.Threads != 0 || o.Multi != 2 || !o.Block || o.Output != "out" || o.BlockExponent != 22 {
		t.Fatalf("options: %+v", o)
	}
	// blocks of 2^4 bytes, as an old line for nsz could ask, are not ones readers take
	for _, bad := range [][]string{{"--bs=4"}, {"--bs=25"}, {"-s", "13"}} {
		if _, err := ParseOptions(append(append([]string{"-C", "-B"}, bad...), "a.nsp")); err == nil {
			t.Errorf("%v is taken", bad)
		}
	}
	if !reflect.DeepEqual(o.Ignored, []string{"--keep"}) || !reflect.DeepEqual(o.Files, []string{"a.nsp", "-b.nsp"}) {
		t.Fatalf("ignored %v, files %v", o.Ignored, o.Files)
	}
	if o, _ := ParseOptions([]string{"-B", "-S", "-"}); o.Block || !reflect.DeepEqual(o.Files, []string{"-"}) {
		t.Fatalf("-S after -B: %+v", o)
	}
	if o, _ := ParseOptions([]string{"--rename", "{name} [{id}]", "--pack-name={name}", "-t", "-1", "x"}); o.Template != "{name} [{id}]" || o.PackName != "{name}" || o.Threads != 0 {
		t.Fatalf("values: %+v", o)
	}
	if o, _ := ParseOptions(nil); o.Level != DefaultLevel || o.Multi != 1 {
		t.Fatalf("defaults: %+v", o)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"-C", "-l"}, "-l needs a value"},
		{[]string{"-l", "fast"}, `-l needs a number, not "fast"`},
		{[]string{"-m", "0"}, `-m needs a number, not "0"`},
		{[]string{"-C=1"}, "-C takes no value"},
		{[]string{"--nope"}, "unknown option --nope"},
	} {
		if _, err := ParseOptions(c.args); err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.args, err, c.want)
		}
	}
}

// check refuses what cannot be done together, and settles the level and the threads.
func TestCheckOptions(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"-C", "-D", "x"}, "choose one of"},
		{[]string{"--dat", "d.dat", "-C", "x"}, "--dat goes with -V"},
		{[]string{"x"}, "nothing to do"},
		{[]string{"-C"}, "no file or folder given"},
	} {
		o, err := ParseOptions(c.args)
		if err == nil {
			err = o.Check()
		}
		if err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.args, err, c.want)
		}
	}
	o, _ := ParseOptions([]string{"--dat", "d.dat", "x"})
	if err := o.Check(); err != nil || !o.Verify {
		t.Fatalf("--dat checks: %v %+v", err, o)
	}
	o, _ = ParseOptions([]string{"-C", "-l", "22", "-m", "2", "x"})
	if err := o.Check(); err != nil || o.Level != MaxLevel || len(o.Notes) != 1 || o.Threads != DefaultThreads(2) {
		t.Fatalf("level and threads: %v %+v", err, o)
	}
	for args, want := range map[string]string{"-C": "compress", "-D": "decompress", "--fat32": "fat32", "--pack-games": "pack", "-V": "verify", "-i": "info"} {
		if o, _ := ParseOptions([]string{args, "x"}); o.Action() != want {
			t.Errorf("%s: %s", args, o.Action())
		}
	}
	if o, _ := ParseOptions([]string{"--rename", "{name}", "x"}); o.Action() != "rename" {
		t.Errorf("--rename: %s", o.Action())
	}
}
