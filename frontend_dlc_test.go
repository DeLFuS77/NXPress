package main

import (
	"os/exec"
	"strings"
	"testing"
)

// dlcChecks runs the warnings of the DLC missing from the list (#160) in node, with the English
// texts of the page, and prints one line for each problem it finds.
const dlcChecks = `
"use strict";
const fs = require("fs"), path = require("path"), vm = require("vm");
const dir = process.argv[1];
const read = name => fs.readFileSync(path.join(dir, name), "utf8").replace(/\r\n/g, "\n");
const source = read("app.js");
const start = source.indexOf("function dlcWarnings(");
const end = source.indexOf("\n// the versions published of each game, when the setting allows it", start);
const context = { asked: [], checked: true };
vm.createContext(context);
vm.runInContext(read("i18n/base.js") + "\n;\n" + read("i18n/en.js") + "\n;\n" +
  "const state = { rows: new Map(), dlc: new Map(), dlcReady: true, language: 'en' };\n" +
  "const $ = () => ({ get checked() { return checked; } });\n" +
  "const t = key => texts.en[key];\n" +
  "const app = () => ({ PublishedDLC: ids => { asked.push(ids); return new Promise(() => {}); } });\n" +
  "const warnSoon = () => {};\n" +
  source.slice(start, end) + "\nthis.state = state; this.dlcWarnings = dlcWarnings; this.dlcName = dlcName; this.texts = texts;", context);
const problems = [];
const want = (what, got, expected) => { if (JSON.stringify(got) !== JSON.stringify(expected)) problems.push(what + ": " + JSON.stringify(got) + ", not " + JSON.stringify(expected)); };

const A = "0100A1B002C4E000", B = "01007C9011E52000", C = "0100C3D00A7F6000", D = "0400B8E0019D4000", E = "0100F2A00D31A000";
const rows = [
  { game: { type: "BASE", titleId: A, name: "Star Courier" } },
  { game: { type: "UPD", titleId: "0100A1B002C4E800", gameId: A } },
  { game: { type: "DLC", titleId: "0100A1B002C4F001", gameId: A } },
  // the game twice: the warning goes on the first
  { game: { type: "BASE", titleId: A, name: "Star Courier" } },
  // a SuperNSP with the game and its DLC
  { game: { type: "BASE", titleId: B, dlcIds: ["01007C9011E53001", "01007c9011e53002"] } },
  { game: { type: "BASE", titleId: C, demo: true } },
  { game: { type: "BASE", titleId: D } },
  { game: { type: "BASE", titleId: E } },
  // a DLC without its game in the list
  { game: { type: "DLC", titleId: "0100F9990000B001", gameId: "0100F9990000A000" } },
];
rows.forEach((row, i) => context.state.rows.set(i, row));
const twice = "this title is in the list twice";
const run = () => { const warnings = new Map([[rows[0], twice]]); context.dlcWarnings(rows, warnings); return warnings; };

// first the DLC of the games are asked, once: not of a demo, nor of a DLC without its game
let warnings = run();
want("asked", context.asked, [[A, B, D, E]]);
want("before the answer", warnings.size, 1);
run();
want("asked again", context.asked.length, 1);

// the answer: A has three DLC out and one coming, the list holds one; B has both in its SuperNSP;
// the list of DLC does not know D; E has only one coming
context.state.dlc.set(A, { out: [{ id: "0100A1B002C4F001", name: "Star Courier – Comet Pack", date: "2025-02-04" }, { id: "0100A1B002C4F002", name: "Star Courier: Nebula Outfits" }, { id: "0100A1B002C4F003" }],
  soon: [{ id: "0100A1B002C4F004", name: "Moon Base", date: "2026-12-03" }] });
context.state.dlc.set(B, { out: [{ id: "01007C9011E53001" }, { id: "01007C9011E53002" }] });
context.state.dlc.set(D, null);
context.state.dlc.set(E, { out: [], soon: [{ id: "0100F2A00D31B001" }] });
warnings = run();
want("the warning of A", warnings.get(rows[0]), twice + " · 2 of its 3 DLC are not in the list");
want("the DLC of A", rows[0].dlc && [rows[0].dlc.total, rows[0].dlc.have, rows[0].dlc.missing.map(dlc => dlc.id), rows[0].dlc.soon.length], [3, 1, ["0100A1B002C4F002", "0100A1B002C4F003"], 1]);
want("A twice", [warnings.has(rows[3]), rows[3].dlc], [false, null]);
want("B, all in its SuperNSP", [warnings.has(rows[4]), rows[4].dlc && rows[4].dlc.have, rows[4].dlc && rows[4].dlc.total], [false, 2, 2]);
want("D, unknown", [warnings.has(rows[6]), rows[6].dlc], [false, null]);
want("E, none out yet", [warnings.has(rows[7]), rows[7].dlc && rows[7].dlc.total], [false, 0]);
want("warnings", warnings.size, 1);

// turned off, or before the lists are read, nothing
context.checked = false;
warnings = run();
want("turned off", [warnings.get(rows[0]), rows[0].dlc], [twice, null]);
context.checked = true;
context.state.dlcReady = false;
warnings = run();
want("not read yet", [warnings.get(rows[0]), rows[0].dlc], [twice, null]);

// the texts
const warn = context.texts.en.warnDlc;
want("one of one", warn(1, 1), "its DLC is not in the list");
want("all", warn(12, 12), "its 12 DLC are not in the list");
want("one of three", warn(1, 3), "1 of its 3 DLC is not in the list");
const count = context.texts.en.popDlcCount;
want("count", [count(1, 3), count(2, 2), count(1, 1)], ["1 of 3 · 2 not in the list", "2 of 2 · you have them all", "1 of 1 · you have it"]);

// the names, without the name of their game before them
const game = { name: "Star Courier" };
want("with a dash", context.dlcName({ id: "X", name: "Star Courier – Comet Pack" }, game), "Comet Pack");
want("with a colon", context.dlcName({ id: "X", name: "star courier: Nebula Outfits" }, game), "Nebula Outfits");
want("the game alone", context.dlcName({ id: "X", name: "Star Courier" }, game), "Star Courier");
want("another word", context.dlcName({ id: "X", name: "Star Couriers Deluxe" }, game), "Star Couriers Deluxe");
want("no game name", context.dlcName({ id: "X", name: "Comet Pack" }, {}), "Comet Pack");
want("no name", context.dlcName({ id: "0100A1B002C4F003" }, game), "DLC without a name · 0100A1B002C4F003");

console.log(problems.join("\n"));
`

// The DLC of a game not in the list are told on its file, once the lists are read and the setting
// allows it; the ones the list holds count in a SuperNSP too, a game the list of DLC does not know
// tells nothing, and the names of the DLC lose the name of their game (#160).
func TestDLCWarnings(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	out, err := exec.Command(node, "-e", dlcChecks, "frontend").CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, problem := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if problem != "" {
			t.Error(problem)
		}
	}
}
