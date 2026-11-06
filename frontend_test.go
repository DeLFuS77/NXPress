package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// frontendChecks runs in node over the folder of the page: it loads the texts as the page does
// and prints one line for each problem it finds.
const frontendChecks = `
"use strict";
const fs = require("fs"), path = require("path"), vm = require("vm");
const dir = process.argv[1];
const problems = [];
const say = text => problems.push(text);

// the texts, loaded in the order of the page, with stand-ins for the sizes and times it formats
const order = ["base", "en", "es", "fr", "de", "it", "pt", "nl", "ru", "ja", "ko", "zh"];
const context = { size: bytes => bytes + " B", time: seconds => seconds + " s" };
vm.createContext(context);
let source = "";
for (const name of order) source += fs.readFileSync(path.join(dir, "i18n", name + ".js"), "utf8") + "\n;\n";
vm.runInContext(source + "this.texts = texts; this.noteRules = noteRules; this.languages = languages;", context);
const { texts, noteRules, languages } = context;
const en = texts.en;
const codes = order.slice(1);
const others = codes.filter(code => code !== "en");

// every language of the menu, with the same texts as English, of the same kind and arguments
if (languages.map(language => language[0]).join() !== codes.join()) say("the menu of languages is " + languages.map(language => language[0]).join());
const braces = text => (text.match(/\{[A-Za-z_]+\}/g) || []).sort().join(" ");
for (const code of codes) {
  const table = texts[code];
  if (!table) { say(code + ": no texts"); continue; }
  for (const key of Object.keys(en)) {
    if (!(key in table)) { say(code + ": no text " + key); continue; }
    if (typeof table[key] !== typeof en[key]) say(code + "." + key + " is a " + typeof table[key] + ", in English a " + typeof en[key]);
    else if (typeof en[key] === "function" && table[key].length !== en[key].length) say(code + "." + key + " takes " + table[key].length + " arguments, in English " + en[key].length);
    else if (typeof en[key] === "string" && braces(table[key]) !== braces(en[key])) say(code + "." + key + " has the fields " + braces(table[key]) + ", in English " + braces(en[key]));
  }
  for (const key of Object.keys(table)) if (!(key in en)) say(code + ": text " + key + " is not in English");
}

// every function answers with text for the arguments the page gives it
const calls = {
  keys: [[""], ["C:\\keys\\prod.keys"]],
  start: ["compress", "decompress", "recompress", "trim", "verify", "convert", "split", "pack", "info", "fat32", "rename"].map(mode => [mode]),
  gameKind: [["BASE"], ["UPD"], ["DLC"]],
  shutdownFailed: [[true], [false]],
};
for (const code of codes) {
  for (const [key, value] of Object.entries(texts[code])) {
    if (typeof value !== "function") continue;
    for (const args of calls[key] || [[1, 1, 1, 1], [2, 0, 1, 0], [5, 3, 2, 1], [21, 4, 21, 4]]) {
      let text;
      try { text = String(value(...args)); } catch (error) { say(code + "." + key + "(" + args.join(", ") + ") fails: " + error.message); continue; }
      if (/undefined|NaN/.test(text)) say(code + "." + key + "(" + args.join(", ") + ") gives " + text);
    }
  }
}

// plurals: [language, text, arguments, what it must say, what it must not]
const plurals = [
  ["en", "lifetime", ["S", 1], "1 file", "1 files"], ["es", "lifetime", ["S", 1], "1 archivo", "1 archivos"],
  ["fr", "lifetime", ["S", 1], "1 fichier", "1 fichiers"], ["de", "lifetime", ["S", 1], "1 Datei", "1 Dateien"],
  ["pt", "lifetime", ["S", 1], "1 arquivo", "1 arquivos"], ["nl", "lifetime", ["S", 1], "1 bestand", "1 bestanden"],
  ["ru", "lifetime", ["S", 1], "1 файле", ""], ["ru", "lifetime", ["S", 2], "2 файлах", ""], ["ru", "lifetime", ["S", 21], "21 файле", ""],
  ["ru", "popTitles", [21, 0, 0], "21 игра", ""], ["ru", "popTitles", [4, 0, 0], "4 игры", ""], ["ru", "popTitles", [11, 0, 0], "11 игр", ""],
  ["ru", "popTitles", [0, 22, 0], "22 обновления", ""], ["ru", "popTitles", [0, 25, 0], "25 обновлений", ""],
  ["ru", "measureIntro", [4, 18, false], "4 процессора", ""], ["ru", "measureIntro", [21, 18, false], "21 процессор,", ""], ["ru", "measureIntro", [8, 18, false], "8 процессоров", ""],
];
for (const [code, key, args, want, unwanted] of plurals) {
  const text = texts[code][key](...args);
  if (!text.includes(want) || unwanted && text.includes(unwanted)) say(code + "." + key + "(" + args.join(", ") + ") gives " + text + ", not " + want);
}

// the notes: each rule has a text in every language but English (which has one where its own
// differs), with the parts the rule finds and no more
const ruleKeys = noteRules.map(rule => rule[1]);
for (const key of ruleKeys.filter((key, i) => ruleKeys.indexOf(key) !== i)) say("the rule " + key + " is there twice");
for (const code of codes) {
  const notes = texts[code].notes || {};
  for (const key of Object.keys(notes)) if (!ruleKeys.includes(key)) say(code + ": note " + key + " has no rule");
  for (const [pattern, key] of noteRules) {
    if (!(key in notes)) { if (code !== "en") say(code + ": no note " + key); continue; }
    if (typeof notes[key] !== "string") continue;
    const parts = new RegExp(pattern.source + "|").exec("").length - 1;
    const used = (notes[key].match(/\$\d/g) || []).map(found => +found[1]);
    for (let part = 1; part <= parts; part++) if (!used.includes(part)) say(code + ".notes." + key + " leaves out $" + part);
    for (const part of used) if (part > parts) say(code + ".notes." + key + " uses $" + part + ", the rule finds " + parts);
  }
}
// the steps of the work have their names in every language that names them
const steps = new Set(others.flatMap(code => Object.keys(texts[code].steps || {})));
for (const code of others) for (const step of steps) if (!(texts[code].steps || {})[step]) say(code + ": no step " + step);

// notes of NXPress come out in the language, as the page shows them: [note, English left in it]
function noteText(note, code) {
  const own = texts[code].notes || {}, english = en.notes || {};
  for (const [pattern, key] of noteRules) {
    const replacement = own[key] !== undefined ? own[key] : english[key];
    if (replacement !== undefined) note = note.replace(pattern, replacement);
  }
  return note;
}
const samples = [
  ["left out of the games to combine", "left out"],
  ["already combined in Game [0100000000010000][v0] (1G+1U).nsp (-w writes it again)", "-w"],
  ["1.0 GB -> 500.0 MB, 50.0% smaller; the original is kept: the new file could only be checked in part", "the original is kept"],
  ["the priority could not be lowered: access denied", "could not be lowered"],
  ["no file to verify among the ones given", "among the ones given"],
  ["the file is empty", "the file is empty"],
  ["cannot be read: Invalid NSP file, reason - [failed to read PFS0 header]", "cannot be read"],
  ["it would get the same name as D:\b\Game.nsp, another file of this work", "another file of this work"],
  ["it would get the same name as the new file of D:\b\Game.nsp: Game.nsz", "the new file of"],
  ["the originals are kept: the pack lacks the ticket of 0100000000010800, the Program NCA of 0100000000010800 v65536", "the ticket of"],
  ["its part Game [0100000000010000][v0].nsp would take the place of the file itself: rename the file first, or split it into another folder", "take the place"],
  ["D:\Juegos\locked cannot be read: its files are left out", "left out"],
  ["nothing in it can be compressed, it cannot be measured with: it uses sparse storage, which only the experimental format NCZSECT2 compresses", "only the experimental format"],
  ["this game uses sparse storage: to compress it with the experimental format of NXPress (NCZSECT2), turn on Extra compression under Made for, or use --extra", "use --extra"],
  ["1.0 GB -> 900.0 MB, 10.0% smaller, 1 NCA in sparse storage in the experimental format NCZSECT2: decompress it with NXPress to install it", "decompress it with NXPress"],
  ["1.0 GB -> 900.0 MB, 10.0% smaller, 47606 LZ4 blocks of Nintendo kept decompressed in the experimental archive format NCZSECT3: decompress it with NXPress to install it", "kept decompressed"],
  ["1.0 GB -> 900.0 MB, 10.0% smaller, 1 NCA with their LZ4 blocks left as they are: their compressed storage could not be read (0123456789abcdef0123456789abcdef.nca: invalid bucket tree)", "left as they are"],
  ["works with: NXPress only, experimental format NCZSECT2 for sparse storage: decompress it with NXPress to install it", "works with"],
  ["works with: NXPress only, experimental archive format NCZSECT3: decompress it with NXPress to install it", "works with"],
  ["about 8.0 GB -> 5.1 GB, 36.2% smaller, 1 h 5 min at level 18", " at level "],
  ["about 8.0 GB -> 5.1 GB, 36.2% smaller, 40 s at level 3", " at level "],
  ["8.0 MB -> 42.3 KB, 99.5% smaller", " -> "],
  ["this work leaves NUP files as they are: Combine (--nup) makes them of dump folders, compressed with zstd, Decompress (-D) turns them back into folders, Details (-i) shows what they hold, Verify (-V) checks them and Rename (--rename) renames them", "--rename"],
  ["167315 files in 3 packages, 31.5 GB → 29.8 GB: 2104 with the data of another file, kept once, 9870 with zstd; dlc/040074a01bf13001/0/updates left out: NUP version 1 holds no updates of DLC", "with zstd"],
  ["2 files in 1 package, 4.0 KB → 1.0 KB: 2 with zstd; the original is kept: --rm-source does not delete dump folders or NUP files", "--rm-source"],
  ["2 files in 1 package, 4.0 KB → 1.0 KB: 2 with zstd; the dump folder is deleted: the NUP holds all its files, checked", "dump folder"],
  ["2 files in 1 package, 4.0 KB → 1.0 KB: 2 with zstd; 1 file stays in the dump folder, which the NUP does not hold as it is: the rest is deleted", "stays"],
  ["2 files in 1 package, 4.0 KB → 1.0 KB: 2 with zstd; 21 files stay in the dump folder, which the NUP does not hold as they are: the rest is deleted", "stay in"],
  ["2 files in 1 package, 4.0 KB → 1.0 KB: 2 with zstd; the dump folder is kept: the NUP cannot be read again: EOF", "read again"],
  ["2 files in 1 package, 4.0 KB → 1.0 KB: 2 with zstd; the dump folder is kept: the NUP does not hold the packages of the folder", "packages"],
  ["2 files in 1 package, 4.0 KB → 1.0 KB: 2 with zstd; the dump folder is kept: none of its files is in the NUP as it is now", "none of"],
  ["1232 files, 39.5 GB, checked by their SHA-256 as they were written", "checked by"],
  ["Kirby Air Riders [040077201BE50000] is already there: move or rename it to unpack the NUP again", "already there"],
  ["D:/Games/.Kirby.nxpress, left by a work that stopped, is in the way: remove it first", "in the way"],
  ["the new NUP does not read back whole: 3 files are not what was packed", "read back"],
  ["cannot be read: the folder holds no game, update or DLC to pack: no exefs or romfs with files in it", "no game"],
  ["cannot be read: it holds 2 programs (0, 1): NUP version 1 holds one", "programs"],
  ["NUP holds a file or folder whose name cannot be written here: romfs/../x", "cannot be written"],
  ["D:/Games/x.bin: its data gives 5 bytes instead of 9", "gives"],
  ["308 files checked by their SHA-256 (2 in no folder of any package); a NUP holds no signatures of Nintendo; not compared with the DAT, which lists no NUP files", "checked by"],
  ["checked on 2026-11-05, not changed since; 1 file checked by its SHA-256", "checked by"],
  ["308 files checked by their SHA-256; it meets what Eglantine and Lichii ask of a NUP", "Eglantine and Lichii ask"],
  ["3 files checked by their SHA-256; Eglantine and Lichii do not open it: package 2 is of kind 3, which the format keeps for later; package 0 has the flags 0x2, and only the first is known; the names or icons by language of package 1 run past their record", "do not open it"],
  ["Eglantine and Lichii do not open it: 1 file has codec flags, which version 1 does not have; 21 files have codec flags, which version 1 does not have", "codec flags"],
  ["Eglantine and Lichii do not open it: 1 file of 0 bytes with a place for data, which must be 0 and 0; 4 files of 0 bytes with a place for data, which must be 0 and 0", "a place for data"],
  ["Eglantine and Lichii do not open it: its folders mark their files with the high bit, as a build of the packer of the format did; the folder \"romfs\" is inside itself; its folders go more than 128 deep", "high bit"],
  ["3 files checked by their SHA-256; Eglantine and Lichii cannot start it: the ExeFS of the update v131072 lacks: main.npdm", "cannot start"],
  ["Eglantine and Lichii cannot start it: the ExeFS of the game v0 lacks: main, main.npdm", "lacks"],
  ["Eglantine and Lichii cannot start it: no game or update in it has an ExeFS", "no game or update"],
  ["Eglantine and Lichii cannot start it: it holds no game or update, and they do not load DLC from NUP files yet", "DLC from NUP"],
  ["cannot be read: NUP flags 0x1 are not supported; Eglantine and Lichii do not open it either", "do not open it either"],
  ["3 of 372587 files are damaged: game v65536: romfs/a.bin (its SHA-256 does not match); update v131072: romfs/b.bin (its zstd data cannot be decompressed (bad magic)); and 1 more", "damaged"],
  ["x.bin (in no folder) (its data decompresses to 5 bytes instead of 9); y.bin (its data cannot be read (I/O error)); z.bin (its data decompresses to more than its 9 bytes)", "its data"],
  ["Game.nup: damaged files:\n  game v0: romfs/a.bin: its SHA-256 does not match\n  and 3 more", "damaged files"],
  ["the NUP file is cut short: 1000 of 5000 bytes, a copy or a download that stopped before its end", "cut short"],
  ["cannot be read: NUP table of contents is damaged (its hash does not match)", "table of contents"],
  ["works with: programs that read NUP files; NXPress makes it of a dump folder (Combine, --nup) and turns it back into one (Decompress, -D)", "--nup"],
  ["Game.nup:\n  0400486024DEC000 BASE v65536 Game\n    version 1.0.2\n    publisher: Nobody\n    605 files: 19.6 GB, 19.5 GB as stored\n  NUP: 38.9 GB, 39.5 GB of files uncompressed, 38.9 GB as stored", "as stored"],
  ["Game.nup:\n  0400486024DEC000 BASE v65536\n    1 file: 4.0 KB, 1.0 KB as stored", "as stored"],
  ["Game.nup:\n  0400486024DEC000 BASE v65536\n    publisher: Nobody", "publisher"],
];
// in English too: what the window says in its own words
const ownEnglish = ["-w", " -> ", "-V", "--rm-source", "--nup"];
for (const [note, english] of samples) {
  for (const code of codes) {
    const text = noteText(note, code);
    if (/\$\d/.test(text)) say(code + ": " + note + " gives " + text);
    if ((code !== "en" || ownEnglish.includes(english)) && text.includes(english)) say(code + ": " + note + " keeps " + english + ": " + text);
  }
}
// the decimal mark of the language, as the page writes its own sizes
for (const code of ["es", "fr", "de", "it", "pt", "nl", "ru"]) {
  const text = noteText("8.0 MB -> 42.3 KB, 99.5% smaller", code);
  if (!text.includes("8,0 MB") || !text.includes("42,3 KB") || !text.includes("99,5%")) say(code + ": the figures of a note keep their point: " + text);
}

// the texts the page asks for by name are there
const page = fs.readFileSync(path.join(dir, "app.js"), "utf8") + fs.readFileSync(path.join(dir, "index.html"), "utf8");
for (const found of page.matchAll(/\bt\("([A-Za-z0-9]+)"\)|data-t(?:-title|-label|-placeholder)?="([A-Za-z0-9]+)"/g)) {
  const key = found[1] || found[2];
  if (!(key in en)) say("the page asks for " + key + ", which has no text");
}

console.log(problems.join("\n"));
`

// The page and its texts: every script of it reads, the 11 languages have the same texts with
// the same arguments and fields, the plurals agree, each note rule has its text with the parts it
// finds, the notes of NXPress come out translated, and the texts the page asks for are there.
func TestFrontendTexts(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	scripts, _ := filepath.Glob(filepath.Join("frontend", "*.js"))
	texts, _ := filepath.Glob(filepath.Join("frontend", "i18n", "*.js"))
	for _, script := range append(scripts, texts...) {
		if out, err := exec.Command(node, "--check", script).CombinedOutput(); err != nil {
			t.Errorf("%s does not read: %s", script, out)
		}
	}
	out, err := exec.Command(node, "-e", frontendChecks, "frontend").CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, problem := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if problem != "" {
			t.Error(problem)
		}
	}
}

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
