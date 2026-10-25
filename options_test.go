package main

import (
	"reflect"
	"strings"
	"testing"
)

// The options of nsz work as there: with their value after them or after "=", short and long,
// and the ones without use here accepted and told.
func TestParseOptions(t *testing.T) {
	o, err := parseOptions([]string{"-C", "-l", "12", "--threads=0", "-m", "2", "-B", "-o", "out", "--keep", "--bs=4", "a.nsp", "--", "-b.nsp"})
	if err != nil {
		t.Fatal(err)
	}
	if !o.compress || o.level != 12 || o.threads != 0 || o.multi != 2 || !o.block || o.output != "out" {
		t.Fatalf("options: %+v", o)
	}
	if !reflect.DeepEqual(o.ignored, []string{"--keep", "--bs"}) || !reflect.DeepEqual(o.files, []string{"a.nsp", "-b.nsp"}) {
		t.Fatalf("ignored %v, files %v", o.ignored, o.files)
	}
	if o, _ := parseOptions([]string{"-B", "-S", "-"}); o.block || !reflect.DeepEqual(o.files, []string{"-"}) {
		t.Fatalf("-S after -B: %+v", o)
	}
	if o, _ := parseOptions([]string{"--rename", "{name} [{id}]", "--pack-name={name}", "-t", "-1", "x"}); o.template != "{name} [{id}]" || o.packName != "{name}" || o.threads != 0 {
		t.Fatalf("values: %+v", o)
	}
	if o, _ := parseOptions(nil); o.level != defaultLevel || o.multi != 1 {
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
		if _, err := parseOptions(c.args); err == nil || !strings.HasPrefix(err.Error(), c.want) {
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
		o, err := parseOptions(c.args)
		if err == nil {
			err = o.check()
		}
		if err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.args, err, c.want)
		}
	}
	o, _ := parseOptions([]string{"--dat", "d.dat", "x"})
	if err := o.check(); err != nil || !o.verify {
		t.Fatalf("--dat checks: %v %+v", err, o)
	}
	o, _ = parseOptions([]string{"-C", "-l", "22", "-m", "2", "x"})
	if err := o.check(); err != nil || o.level != maxLevel || len(o.ignored) != 1 || o.threads != defaultThreads(2) {
		t.Fatalf("level and threads: %v %+v", err, o)
	}
	for args, want := range map[string]string{"-C": "compress", "-D": "decompress", "--fat32": "fat32", "--pack-games": "pack", "-V": "verify", "-i": "info"} {
		if o, _ := parseOptions([]string{args, "x"}); o.action() != want {
			t.Errorf("%s: %s", args, o.action())
		}
	}
	if o, _ := parseOptions([]string{"--rename", "{name}", "x"}); o.action() != "rename" {
		t.Errorf("--rename: %s", o.action())
	}
}
