package main

import (
	"os/exec"
	"strings"
	"testing"
)

// dropChecks runs watchDrops of the page in node, on a window and a runtime of its own, and
// prints one line for each problem it finds.
const dropChecks = `
"use strict";
const fs = require("fs"), vm = require("vm");
const source = fs.readFileSync(process.argv[1], "utf8").replace(/\r\n/g, "\n");
const start = source.indexOf("function watchDrops(");
const code = source.slice(start, source.indexOf("\n}\n", start) + 3);
const context = {};
vm.createContext(context);
vm.runInContext(code + "this.watchDrops = watchDrops;", context);

const problems = [];
const listeners = () => {
  const on = {};
  return { addEventListener: (name, fn) => (on[name] = on[name] || []).push(fn), fire: (name, event) => (on[name] || []).forEach(fn => fn(event || {})) };
};
const files = { dataTransfer: { types: ["Files"], items: [1, 2] } };
function setup(busy) {
  const target = listeners(), layer = listeners(), events = {};
  const runtime = { EventsOn: (name, fn) => { events[name] = fn; } };
  const seen = { shown: false, count: 0 };
  context.watchDrops(target, runtime, layer, () => busy, (show, count) => { seen.shown = show; if (show) seen.count = count; });
  return { target, layer, seen, drop: () => events["wails:file-drop"](10, 10, ["a.nsp"]) };
}
const want = (what, got, expected) => { if (got !== expected) problems.push(what + ": " + got + ", not " + expected); };

// over the window and into an element of it, then out of both: the layer goes only at the end
let w = setup(false);
w.target.fire("dragenter", files);
w.target.fire("dragenter", files);
want("dragged over", w.seen.shown, true);
want("how many files", w.seen.count, 2);
w.target.fire("dragleave");
want("out of the element only", w.seen.shown, true);
w.target.fire("dragleave");
want("out of the window", w.seen.shown, false);

// dropped on the page (Windows)
w.target.fire("dragenter", files);
w.target.fire("drop");
want("dropped on the page", w.seen.shown, false);

// dropped where the system takes it (macOS): no drop and no leave for the page, the files only
w = setup(false);
w.target.fire("dragenter", files);
w.target.fire("dragenter", files);
w.drop();
want("files handed over by the system", w.seen.shown, false);
// the drags after it start from nothing: one leave ends the layer again
w.target.fire("dragenter", files);
w.target.fire("dragleave");
want("the next drag, out of the window", w.seen.shown, false);

// the mouse moving on the layer: no drag is going on
w.target.fire("dragenter", files);
w.layer.fire("mousemove");
want("the mouse moving on the layer", w.seen.shown, false);

// no layer while NXPress works, nor for what is not files
w = setup(true);
w.target.fire("dragenter", files);
want("while working", w.seen.shown, false);
w = setup(false);
w.target.fire("dragenter", { dataTransfer: { types: ["text/plain"] } });
want("text dragged", w.seen.shown, false);

console.log(problems.join("\n"));
`

// The layer of a drop goes once the files are dropped, also when the system takes the drop and
// the page gets neither a drop nor a leave, as on macOS (#46): it covered the window, and nothing
// could be clicked.
func TestDropLayerEnds(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	out, err := exec.Command(node, "-e", dropChecks, "frontend/app.js").CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, problem := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if problem != "" {
			t.Error(problem)
		}
	}
}
