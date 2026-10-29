package main

import (
	"reflect"
	"strings"
	"testing"
)

// -o with no value, or with an option after it, puts the new files next to their originals as
// in nsz; the other options do not take the option after them as their value.
func TestOptionValueIsNotTheNextOption(t *testing.T) {
	o, err := parseOptions([]string{"-C", "-o", "--keys", "K", "a.nsp"})
	if err != nil {
		t.Fatal(err)
	}
	if o.output != "" || o.keys != "K" || !reflect.DeepEqual(o.files, []string{"a.nsp"}) {
		t.Fatalf("-o before an option: output %q, keys %q, files %v", o.output, o.keys, o.files)
	}
	for _, args := range [][]string{{"-C", "a.nsp", "-o"}, {"-C", "--output", "-w", "a.nsp"}, {"-C", "-o", "out", "-o", "--", "a.nsp"}} {
		if o, err := parseOptions(args); err != nil || o.output != "" || !reflect.DeepEqual(o.files, []string{"a.nsp"}) {
			t.Errorf("%v: output %q, files %v, %v", args, o.output, o.files, err)
		}
	}
	// a negative number is a value, and "-" alone a file
	if o, err := parseOptions([]string{"-o", "-", "-t", "-1", "a.nsp"}); err != nil || o.output != "-" || o.threads != 0 {
		t.Fatalf("values that start with -: %+v %v", o, err)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"-C", "--keys", "-o", "out"}, "--keys needs a value"},
		{[]string{"-C", "-l", "-w", "a.nsp"}, "-l needs a value"},
		{[]string{"--dat", "--", "a.nsp"}, "--dat needs a value"},
		{[]string{"--rename", "-V"}, "--rename needs a value"},
	} {
		if _, err := parseOptions(c.args); err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.args, err, c.want)
		}
	}
}
