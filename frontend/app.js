"use strict";

const modes = ["compress", "decompress", "recompress", "trim", "verify", "convert", "split", "pack", "info", "fat32", "rename"];
// the menu, by what the modes are for
const groups = [
  ["groupSpace", ["compress", "decompress", "recompress", "trim"]],
  ["groupOrganize", ["pack", "split", "fat32", "convert", "rename"]],
  ["groupCheck", ["verify", "info"]],
];
// the groups and the tools of the menu that are hidden (Settings, Sidebar): the names of the groups
// ("groupSpace") and of the tools ("compress", "worklog", "emulator") that are switched off
const hiddenSide = new Set(load("hiddenSide", [], Array.isArray).filter(name => typeof name === "string"));
// the groups of Settings, Sidebar that are open to show their tools; they start closed
const openSide = new Set(load("openSide", [], Array.isArray).filter(name => typeof name === "string"));
// the tools of a group, in the order of the menu: the log and the Emulator are pages, under Check
const sideTools = group => {
  const members = groups.find(entry => entry[0] === group)[1].slice();
  if (group === "groupCheck") members.push("worklog", ...(typeof emulatorButton === "function" ? ["emulator"] : []));
  return members;
};
const sideGroupOf = tool => groups.map(entry => entry[0]).find(group => sideTools(group).includes(tool));
const sideToolShown = tool => !hiddenSide.has(tool) && !hiddenSide.has(sideGroupOf(tool));
const sideGroupShown = group => !hiddenSide.has(group) && sideTools(group).some(sideToolShown);
// the first section of the menu that is shown, for when the one open is hidden
const firstSideMode = () => groups.flatMap(entry => entry[1]).find(sideToolShown) || "";
// what each mode takes, to show it; Compress with Optimize for install takes what --optimize takes
const takesOptimize = "NSP NSZ XCI XCZ";
const takesShown = {
  compress: "NSP XCI", decompress: "NSZ XCZ NUP", recompress: "NSZ XCZ", trim: "XCI NSP/NSZ:takesUpdates", verify: "NSP NSZ XCI XCZ NUP :takesCode", convert: "XCI XCZ",
  split: "NSP NSZ XCI XCZ NUP", pack: "NSP NSZ XCI XCZ :takesDumps", info: "NSP NSZ XCI XCZ NUP", fat32: "NSP NSZ XCI XCZ", rename: "NSP NSZ XCI XCZ NUP",
};

// a NUP holds its game decrypted: it is read and verified without keys
const isNup = path => /\.nup$/i.test(path);
// the cover of a dump folder, until its game is read: a folder
const folderIcon = `<svg class="cover-folder" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 7.5V18a1.5 1.5 0 0 0 1.5 1.5h15A1.5 1.5 0 0 0 21 18V9a1.5 1.5 0 0 0-1.5-1.5h-7.2L10.6 5.5H4.5A1.5 1.5 0 0 0 3 7.5z"/></svg>`;
// whether a work needs the keys: not to recompress or for FAT32, nor for a list of NUP files and
// dump folders only
// Recompress needs them only with the extra compression, which goes through an NSP (#92)
const needsKeys = mode => (mode === "recompress" ? $("extra").checked : mode !== "fat32") && !(state.items.length > 0 && state.items.every(item => isNup(item.path) || item.dump));

// the icon of each section: lines drawn on a grid of 24, in the color of the text
const icons = {
  compress: '<path d="M12 3v6m0 0-3-3m3 3 3-3M12 21v-6m0 0-3 3m3-3 3 3M5 12h14"/>',
  decompress: '<path d="M12 9V3m0 0-3 3m3-3 3 3M12 15v6m0 0-3-3m3 3 3-3M5 12h14"/>',
  recompress: '<path d="M20 12a8 8 0 1 1-2.3-5.7M20 4v4h-4"/>',
  trim: '<circle cx="6" cy="7" r="2.5"/><circle cx="6" cy="17" r="2.5"/><path d="M8.2 8.4 20 18M8.2 15.6 20 6"/>',
  verify: '<path d="M12 3 4.5 6v6c0 4.3 3.2 7.7 7.5 9 4.3-1.3 7.5-4.7 7.5-9V6z"/><path d="m8.5 12 2.5 2.5 4.5-5"/>',
  convert: '<path d="M4 8h13m0 0-3-3m3 3-3 3M20 16H7m0 0 3-3m-3 3 3 3"/>',
  split: '<path d="M12 3v7m0 0-6 6v5m6-11 6 6v5"/>',
  pack: '<path d="m3 7 9-4 9 4-9 4z"/><path d="M3 7v10l9 4 9-4V7M12 11v10"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7.5v.5"/>',
  fat32: '<path d="M7 3h7l4 4v14H7z"/><path d="M10 6v3m3-3v3"/>',
  rename: '<path d="M4 20h4L19 9l-4-4L4 16z"/><path d="m14 6 4 4"/>',
  // the log of the works: a clock going back
  worklog: '<path d="M3.8 12a8.2 8.2 0 1 0 2.4-5.8"/><path d="M3.5 3.8v3.9h3.9"/><path d="M12 7.5V12l3 2"/>',
};
const icon = name => `<svg class="icon" viewBox="0 0 24 24" aria-hidden="true">${icons[name] || ""}</svg>`;
// the color of the tile of an icon, by the group of its section
const tones = { groupSpace: "space", groupOrganize: "organize", groupCheck: "check" };
const tile = (name, group) => `<span class="tile tone-${tones[group] || "neutral"}">${icon(name)}</span>`;

const $ = id => document.getElementById(id);
// motion is left out when Windows asks for less of it
const calm = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;
// enter plays the animation of a class again on the elements given
function replay(className, ...elements) {
  for (const element of elements) {
    if (!element) continue;
    element.classList.remove(className);
    void element.offsetWidth;
    element.classList.add(className);
  }
}
// countUp shows a size growing to its value, in the text format gives
function countUp(element, bytes, format, html) {
  const put = text => { if (html) element.innerHTML = text; else element.textContent = text; };
  if (calm()) { put(format(bytes)); return; }
  const start = performance.now(), length = 900;
  const step = now => {
    const share = Math.min(1, (now - start) / length);
    put(format(bytes * (1 - Math.pow(1 - share, 3))));
    if (share < 1 && element.isConnected) requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
}
const app = () => window.go.main.App;
const state = {
  theme: "auto", mode: "compress", language: "en", items: [], rows: new Map(), busy: false, keys: "", output: "", dat: "",
  setup: null, sources: [], removed: new Set(), paused: false, starting: false, lastRun: null, shutdownAt: 0, template: "", sort: { key: "", dir: 1 }, skipGames: new Set(), off: {}, verifyLevel: "full",
  // the newest version published of each title ID asked (null when the list does not know it),
  // and the day of the list of versions: null until it is read, "" when there is none
  published: new Map(), versionsDate: null,
  // the log of the works, the oldest first, its text file, the works open in it, and whether it
  // shows only the ones with problems (#69)
  worklog: [], worklogFile: "", worklogOpen: new Set(), worklogProblems: false,
  // the works a search opens by itself stay closed after Collapse all, until the search changes
  worklogFindClosed: false,
  // the places of NXPress in the Explorer (#87, #88), and the files it gave while the window was
  // open: the ones waiting for the work to end, when the last came, how many make the list, and
  // the list they took the place of, for Undo
  explorer: { menu: "", files: "", others: [] }, openedWaiting: [], openedAt: 0, openedCount: 0, openedBefore: null,
  // a new version installed from NXPress (#91): its step and bytes while it goes, what kept it
  // from installing it, the version installed, and whether the restart waits for the work
  installing: null, installError: "", installed: "", restartAfterWork: false,
};

// load gives a value kept by the page, or fallback when there is none or it is not what valid
// expects (by default, a value of the kind of fallback): a value spoilt by hand or by another
// version must not stop the window
function load(name, fallback, valid = sameKind(fallback)) {
  try {
    const value = localStorage.getItem("nxpress." + name);
    if (value === null) return fallback;
    const parsed = JSON.parse(value);
    return valid(parsed) ? parsed : fallback;
  } catch { return fallback; }
}
function sameKind(fallback) {
  if (fallback === null) return () => true;
  if (Array.isArray(fallback)) return Array.isArray;
  if (typeof fallback === "object") return value => !!value && typeof value === "object" && !Array.isArray(value);
  return value => typeof value === typeof fallback;
}
// the checks of the values kept that hold more than one value
const isObject = value => !!value && typeof value === "object" && !Array.isArray(value);
const isLifetime = value => isObject(value) && Number.isFinite(value.bytes) && Number.isFinite(value.files);
const isReport = value => isObject(value) && Array.isArray(value.rows) && value.rows.every(row => isObject(row) && ["ok", "failed", "modified", "skipped", "cancelled"].includes(row.status));
const isPending = value => isObject(value) && modes.includes(value.mode) && isObject(value.request) && Array.isArray(value.request.files);
// the settings saved with a name, the ones that can be read
const savedPresets = () => load("presets", []).filter(preset => isObject(preset) && typeof preset.name === "string" && Number.isFinite(preset.level));
// the list as rows or as a grid of covers: each section keeps its own (#84); one not chosen yet
// shows the one chosen before for all of them
function viewOf(mode) {
  const view = load("views", {})[mode] || load("view", "list");
  return view === "grid" ? "grid" : "list";
}
function save(name, value) {
  try { localStorage.setItem("nxpress." + name, JSON.stringify(value)); } catch { }
}
// a text of the language chosen, else the English one
const t = key => (texts[state.language] && texts[state.language][key] !== undefined ? texts[state.language] : texts.en)[key];

function time(seconds) {
  if (seconds < 60) return seconds + " s";
  const m = Math.floor(seconds / 60), s = seconds % 60;
  return m < 60 ? `${m} min ${s} s` : `${Math.floor(m / 60)} h ${m % 60} min`;
}
// a size with the decimal mark of the language chosen (8,0 MB in Spanish)
const sizeFormats = {};
function size(bytes) {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes, unit = 0;
  while (value >= 1024 && unit < units.length - 1) { value /= 1024; unit++; }
  const digits = unit ? 1 : 0, key = state.language + digits;
  sizeFormats[key] = sizeFormats[key] || new Intl.NumberFormat(state.language, { minimumFractionDigits: digits, maximumFractionDigits: digits, useGrouping: false });
  return sizeFormats[key].format(value) + " " + units[unit];
}
// a day as the language writes it (21 oct 2026 in Spanish for 2026-10-21), "" for none
function day(date) {
  try { return date ? new Intl.DateTimeFormat(state.language, { dateStyle: "medium" }).format(new Date(date + "T00:00:00")) : ""; } catch { return date; }
}
// a path in a field cut at its start (direction: rtl) keeps its order with a mark of left to
// right at both ends: else "E:\Games" shows as "\Games:E" and brackets turn round
const ltr = path => "\u200E" + path + "\u200E";

// what to do about an error, by what it says
function adviceFor(error) {
  const text = error.toLowerCase();
  if (text.includes("not signed")) return "adviceUnsigned";
  if (/content id|hash|damaged|does not match|corrupt/.test(text)) return "adviceDamaged";
  if (/key|titlekek|title key|prod\.keys|rights id/.test(text)) return "adviceKeys";
  if (/no space|not enough space|disk full/.test(text)) return "adviceSpace";
  if (/access is denied|permission|in use|being used/.test(text)) return "adviceAccess";
  return "adviceGeneric";
}

// the word of a state: a check says Verified, and Corrupted when its hashes find the file damaged,
// as other tools say it (#55); a file that could not be checked (a key missing, unreadable) is not
// corrupted, and keeps Failed with its reason
const damaged = (status, error) => status === "failed" && !!error && error !== "cancelled" && adviceFor(error) === "adviceDamaged";
function stateWord(status, mode, isDamaged) {
  if (mode === "verify" && status === "ok") return t("verified");
  if (mode === "verify" && isDamaged) return t("corrupted");
  return t(status);
}

// the space saved since always, kept in the window: what every work that wrote files saved
// in the units the window shows sizes in (1 GB = 1024 MB), so they read round
const milestones = [100, 250, 500, 1024, 2048, 5120, 10240].map(gb => gb * 2 ** 30);
// a work of this window saved space: only then the empty list tells what NXPress has saved since
// always, not each time the window opens (#48)
let savedHere = false;
function keepSaved(bytes, files) {
  if (bytes <= 0) return;
  savedHere = true;
  const before = load("lifetime", { bytes: 0, files: 0 }, isLifetime);
  const after = { bytes: before.bytes + bytes, files: before.files + files };
  save("lifetime", after);
  // crossing a milestone is told once, with the bar of actions glowing
  const crossed = milestones.filter(mark => before.bytes < mark && after.bytes >= mark).pop();
  if (crossed) setTimeout(() => { showToast(t("milestone")(size(crossed))); replay("glow", document.querySelector(".actions")); }, 1600);
}
// the space saved since always, on the empty list once a work of this window saved some; it counts
// up the first time it shows
let lifetimeShown = false, lifetimeAt = 0;
function renderLifetime() {
  const kept = load("lifetime", { bytes: 0, files: 0 }, isLifetime);
  const element = $("lifetime");
  element.hidden = !(savedHere && kept.bytes > 0);
  if (element.hidden) return;
  if (!lifetimeShown && !$("empty").hidden) {
    lifetimeShown = true;
    lifetimeAt = performance.now();
    // the text is made of figures of this window only: it is drawn as HTML for its bold figure
    countUp(element, kept.bytes, bytes => t("lifetime")(size(bytes), kept.files), true);
  } else if (lifetimeShown && performance.now() - lifetimeAt > 1000) {
    element.innerHTML = t("lifetime")(size(kept.bytes), kept.files);
  }
}

// the speed of the work: the bytes done each second, for a line of the last minute and the time
// left; and the progress on the button of the taskbar
const speed = { samples: [], last: 0, at: 0, taskbarAt: 0 };
// workRows are the rows of the work running, by the start they wear: the files it was given
// (renamed ones too) and the ones a watch or a measure took; of a pack of one file only its new
// file, which tells the progress of all
function workRows() {
  const run = state.lastRun;
  if (!run) return [];
  const rows = [...state.rows.values()].filter(row => row.run === run.started);
  return onePack(run) ? rows.filter(row => row.item.result) : rows;
}
// rowShare is how far a file of the work is, from 0 to 1: a file in steps (the work, then the
// check of what it wrote) counts each step as its part, so it does not fall back at the check
function rowShare(row) {
  if (["ok", "failed", "modified", "skipped"].includes(row.status)) return 1;
  if (row.status !== "work" || !row.total) return 0;
  const share = Math.min(1, row.done / row.total);
  const steps = stepsOf(row);
  const index = steps.indexOf((row.step || "").split(" ")[0]);
  return steps.length > 1 && index >= 0 ? (index + share) / steps.length : share;
}
function workDone() {
  let done = 0, total = 0;
  for (const row of workRows()) {
    // a file weighs its size; the new file of a pack, the size of the files it is made of
    const bytes = (row.item.result ? state.lastRun.bytes : row.item.size) || row.total || 0;
    total += bytes;
    done += rowShare(row) * bytes;
  }
  return { done, total };
}
function trackSpeed() {
  const now = performance.now();
  const { done, total } = workDone();
  if (!speed.at) { speed.at = now; speed.last = done; }
  if (now - speed.at >= 1000) {
    const rate = Math.max(0, (done - speed.last) / ((now - speed.at) / 1000));
    speed.samples = [...speed.samples, rate].slice(-60);
    speed.at = now;
    speed.last = done;
  }
  if (now - speed.taskbarAt >= 700 || done >= total) {
    speed.taskbarAt = now;
    app().SetTaskbar(state.paused ? "paused" : "normal", done, total);
  }
  return { done, total };
}
function resetSpeed() {
  Object.assign(speed, { samples: [], last: 0, at: 0, taskbarAt: 0 });
}
function renderSpeed() {
  $("speed").hidden = !state.busy || speed.samples.length < 2;
  if ($("speed").hidden) return;
  const { done, total } = workDone();
  const samples = speed.samples;
  const top = Math.max(...samples, 1);
  const points = samples.map((value, i) => `${(i / (samples.length - 1)) * 120},${24 - (value / top) * 22 - 1}`);
  $("speed-line").setAttribute("d", "M" + points.join("L"));
  $("speed-area").setAttribute("d", "M0,24L" + points.join("L") + "L120,24Z");
  const recent = samples.slice(-5);
  const rate = recent.reduce((sum, value) => sum + value, 0) / recent.length;
  const left = rate > 0 ? (total - done) / rate : 0;
  $("speed-text").textContent = `${size(rate)}/s` + (left > 0 && !state.paused ? " · " + t("timeLeft")(time(Math.round(left))) : "");
}

// copyText puts text on the clipboard and says so; the message is the text itself when none is given
function copyText(text, message) {
  window.runtime.ClipboardSetText(text);
  showToast(message || t("copied")(text));
}

// reportOf is the full text of Details for a file (what the export to .txt writes for it), asked
// of NXPress once and kept with the row; "" when the file cannot be read, and then it says why
async function reportOf(row) {
  if (row.reportText) return row.reportText;
  try {
    row.reportText = await app().Report(row.item.path);
  } catch (error) {
    showToast(String(error));
    return "";
  }
  return row.reportText;
}

// "Full details" under a file in Details: everything NXPress can tell of it as text that can be
// selected, copied with the button or with a right click, and is the same the .txt export writes
function addReport(element, row) {
  row.reportText = "";
  const box = document.createElement("details");
  box.className = "card-report";
  box.innerHTML = `<summary></summary><div class="card-report-bar"><button class="quiet"></button></div><pre tabindex="0"></pre>`;
  box.querySelector("summary").textContent = t("cardReport");
  const pre = box.querySelector("pre"), button = box.querySelector("button");
  button.textContent = t("menuCopyReport");
  const copyReport = async () => { const text = await reportOf(row); if (text) copyText(text, t("copiedDetails")); };
  button.onclick = copyReport;
  box.addEventListener("toggle", async () => {
    if (box.open && !pre.textContent) pre.textContent = (await reportOf(row)) || "";
  });
  pre.addEventListener("contextmenu", event => {
    event.preventDefault();
    const chosen = String(getSelection());
    openMenu([
      chosen && [t("menuCopySelection"), () => copyText(chosen, t("copiedDetails"))],
      [t("menuCopyReport"), copyReport],
    ].filter(Boolean), event.clientX, event.clientY);
  });
  element.append(box);
}

// a card for a file in Info: its size beside the one as NSP or XCI, and what it works with
function renderCard(row) {
  row.tr.querySelector(".file-card")?.remove();
  const card = row.card;
  if (!card || state.mode !== "info") return;
  const element = document.createElement("div");
  element.className = "file-card";
  if (card.packages) {
    renderNupCard(element, card, row);
    row.tr.querySelector(".note").after(element);
    return;
  }
  const whole = Math.max(card.decompressed || card.size, card.size);
  const share = whole ? card.size / whole : 1;
  const compressed = card.solid + card.blocks > 0;
  // NXDock installs every one, the extra compression of NXPress too (NCZSECT2 and NCZSECT3)
  const works = card.format ? { NXPress: 1, NXDock: 1, DBI: 0, Tinfoil: 0, Awoo: 0, CyberFoil: 0, Sphaira: 0, emulators: 0 }
    : card.solid > 0 ? { NXDock: 1, DBI: 1, Tinfoil: 1, Awoo: 1, CyberFoil: 1, Sphaira: 1, emulators: 0 }
    : card.blocks > 0 ? { NXDock: 1, DBI: 1, Tinfoil: 1, Awoo: 0, CyberFoil: 1, Sphaira: 1, emulators: 1 }
    : { NXDock: 1, DBI: 1, Tinfoil: 1, Awoo: 1, CyberFoil: 1, Sphaira: 1, emulators: 1 };
  element.innerHTML = `<div class="card-sizes"><span class="kind"></span><div class="card-bar"><div></div></div><small></small></div><div class="tags"></div>`;
  element.querySelector(".kind").textContent = card.kind;
  element.querySelector(".card-bar div").style.width = (share * 100).toFixed(1) + "%";
  element.querySelector("small").textContent = compressed ? t("cardAs")(size(card.size), size(whole), card.kind.startsWith("XC") ? "XCI" : "NSP") : size(card.size) + (card.trimmable > 0 ? " · " + t("cardTrim")(size(card.trimmable)) : "");
  for (const [name, yes] of Object.entries(works)) {
    const tag = document.createElement("i");
    tag.className = yes ? "yes" : "no";
    tag.textContent = name === "emulators" ? t("tagEmulators") : name;
    element.querySelector(".tags").append(tag);
  }
  // NCA rewritten by a tool: it needs sigpatches, and some installers may not take it
  if (card.unsignedNote) {
    const warning = document.createElement("p");
    warning.className = "warning";
    warning.textContent = noteText(card.unsignedNote).replace(/^./, (c) => c.toUpperCase()).replace("--rebuild", "‑‑rebuild");
    element.append(warning);
  }
  row.tr.querySelector(".note").after(element);
}

// the part of the note of a NUP that tells what keeps Eglantine and Lichii from opening it, from
// starting its game or from showing it right: always at its end, after "; " (the note before it,
// and that part, as NXPress gives them in English); "" when they read it well
function splitEmulators(note) {
  const found = /(^|; )Eglantine and Lichii (do not open it:|cannot start it:|show its )/.exec(note || "");
  if (!found) return [note || "", ""];
  return [note.slice(0, found.index), note.slice(found.index + found[1].length)];
}

// the card of a NUP: its size beside the one of its files uncompressed, then a line for each of
// its packages (the game, an update, a DLC) with who it is and its files; the size as stored is
// told only where it differs, for the files stored with zstd. Under them, whether Eglantine and
// Lichii read it well, as the cards of the other files tell what reads them, and in orange what
// keeps them from it, with a button to write its metadata again when it is counted from 0.
function renderNupCard(element, card, row) {
  element.innerHTML = `<div class="card-sizes"><span class="kind"></span><div class="card-bar"><div></div></div><small></small></div><ul class="nup-packages"></ul><div class="tags"></div><div class="card-only"></div>`;
  // the green Eglantine / Lichii tags are not shown any more; what keeps them from a NUP still is
  if (!card.emulatorsOk && card.emulators) {
    const fix = document.createElement("div");
    fix.className = "card-fix";
    fix.innerHTML = `<p class="warning"></p>`;
    fix.querySelector("p").textContent = noteText(card.emulators);
    if (card.zeroBased) {
      const button = document.createElement("button");
      button.className = "outline";
      button.textContent = t("fixForEmulators");
      button.disabled = state.busy;
      button.onclick = () => fixNupMeta(row, button);
      fix.append(button);
    }
    element.querySelector(".tags").after(fix);
  }
  element.querySelector(".kind").textContent = card.kind;
  const whole = Math.max(card.decompressed, card.size);
  element.querySelector(".card-bar div").style.width = (whole ? card.size / whole * 100 : 100).toFixed(1) + "%";
  element.querySelector("small").textContent = t("cardNup")(size(card.size), size(card.decompressed));
  for (const pkg of card.packages) {
    const line = document.createElement("li");
    line.innerHTML = `<span class="k"></span><span class="who"><b></b><span></span></span><span class="num n-files"></span><span class="num n-size"></span><span class="num n-stored"></span>`;
    line.querySelector(".k").textContent = t("gameKind")(pkg.type);
    line.querySelector("b").textContent = pkg.name || "";
    line.querySelector("b").hidden = !pkg.name;
    const facts = [pkg.publisher, pkg.titleId, `v${pkg.version}`, pkg.shown].filter(Boolean).join(" · ");
    line.querySelector(".who span").textContent = facts;
    const stored = size(pkg.stored) !== size(pkg.size) ? size(pkg.stored) : "";
    // the files, the size and the compressed size each in a column of their own, with a small word under each
    for (const [column, value, word] of [[".n-files", String(pkg.files), t("nupFilesWord")], [".n-size", size(pkg.size), t("nupUncompressed")], [".n-stored", stored, t("nupCompressed")]]) {
      const cell = line.querySelector(column);
      if (!value) continue;
      const big = document.createElement("span"), small = document.createElement("small");
      big.textContent = value;
      small.textContent = word;
      cell.append(big, small);
    }
    element.querySelector(".nup-packages").append(line);
  }
  renderNupIcon(element, card, row);
}

// the icon of a NUP in its card, at its left; a NUP without one (made of a dump folder without
// control data) tells it, with a button for the icon of its game in titledb, one to search an
// image in the eShop (#102) and one for an image of one's own (#97), or one pasted. One given an
// icon already can have it changed (#103), unless it carries the icons of the game itself
let nupIconHelps = 0;
function renderNupIcon(element, card, row) {
  if (card.icon) {
    const icon = document.createElement("img");
    icon.className = "nup-icon";
    icon.alt = "";
    icon.src = card.icon;
    element.classList.add("has-icon");
    element.prepend(icon);
    // what was just done to give it: where the icon comes from, and the size of an image chosen
    let last = element.querySelector(".tags");
    if (row.iconNote) {
      const said = document.createElement("p");
      said.className = "nup-icon-note";
      said.textContent = row.iconNote;
      last.after(said);
      last = said;
    }
    if (card.gameIcons) return;
    if (!row.iconChanging) {
      const change = document.createElement("button");
      change.className = "nup-icon-change";
      change.textContent = t("changeImage");
      change.disabled = state.busy;
      change.onclick = () => {
        row.iconChanging = true;
        renderCard(row);
        row.tr.querySelector(".nup-no-icon button")?.focus();
      };
      last.after(change);
      return;
    }
    last.after(iconLine(row, true));
    return;
  }
  if (!card.noIcon) return;
  element.querySelector(".tags").after(iconLine(row, false));
}

// the line of the ways a NUP gets an icon: titledb, a search in the eShop, an image of one's own,
// or one pasted; with replace, the icon it carries is changed, and the line can be closed
function iconLine(row, replace) {
  const line = document.createElement("div");
  line.className = "card-fix nup-no-icon";
  line.innerHTML = `<p class="say"></p><button class="outline"></button><span class="help-text"></span><button class="outline"></button><button></button><span class="paste-hint"></span>`;
  const [fromTitledb, search, choose] = line.querySelectorAll("button");
  const help = line.querySelector(".help-text");
  const say = line.querySelector(".say");
  // why the last try gave it none stays in the line, in orange, until another try
  say.textContent = row.iconFailed || t(replace ? "changeImageSay" : "nupNoIcon");
  say.classList.toggle("warning", !!row.iconFailed);
  fromTitledb.textContent = t("iconFromTitledb");
  help.id = "nup-icon-help-" + ++nupIconHelps;
  help.textContent = t("iconFromTitledbHelp");
  fromTitledb.classList.add("has-help");
  fromTitledb.dataset.help = help.id;
  fromTitledb.setAttribute("aria-describedby", help.id);
  search.textContent = t("searchImage");
  choose.textContent = t("chooseImage");
  line.querySelector(".paste-hint").textContent = t("pasteHint");
  const buttons = [fromTitledb, search, choose];
  if (replace) {
    const close = document.createElement("button");
    close.className = "quiet";
    close.textContent = t("cancel");
    close.onclick = () => {
      row.iconChanging = false;
      row.iconFailed = "";
      renderCard(row);
    };
    line.querySelector(".paste-hint").before(close);
    buttons.push(close);
  }
  for (const button of buttons) button.disabled = state.busy;
  fromTitledb.onclick = () => giveNupIcon(row, () => app().FixNupIcon(row.item.path, replace));
  search.onclick = () => openCoverSearch(row, replace, search);
  choose.onclick = () => giveNupIcon(row, () => app().ChooseNupIcon(row.item.path, t("chooseImageTitle"), replace));
  // an image pasted goes to the NUP whose line was used last
  const aim = () => { iconTarget = { row, replace }; };
  line.addEventListener("pointerdown", aim);
  line.addEventListener("focusin", aim);
  if (!iconTarget || iconTarget.row === row || !document.body.contains(iconTarget.row.tr)) aim();
  return line;
}

// the NUP an image pasted (Ctrl+V) goes to: the one whose line of the icon was used last, or
// the one the window to search is open for
let iconTarget = null;

// reads an image pasted, as an address of data; null when what was pasted is no image
function pastedImage(event) {
  const items = event.clipboardData ? [...event.clipboardData.items] : [];
  const file = items.find(item => item.kind === "file" && item.type.startsWith("image/"))?.getAsFile();
  if (!file) return null;
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

// an image pasted in the window goes to the NUP of the window to search, or of the line of the
// icon used last, in Details; text pasted in a field stays text
async function pasteIcon(event) {
  const dialog = $("cover-dialog");
  const target = dialog.open ? cover.target : iconTarget;
  if (!target || state.busy) return;
  if (!dialog.open && (state.mode !== "info" || !document.body.contains(target.row.tr) || !target.row.tr.querySelector(".nup-no-icon"))) return;
  const reading = pastedImage(event);
  if (!reading) return;
  event.preventDefault();
  let data;
  try {
    data = await reading;
  } catch {
    return;
  }
  if (dialog.open) dialog.close();
  giveNupIcon(target.row, () => app().PasteNupIcon(target.row.item.path, data, target.replace));
}

// the window to search an image in the eShop, for one NUP (#102): the name it searches is the one
// of the game, which can be changed, as file names and title IDs are often wrong
const cover = { target: null, chosen: null, round: 0 };
function openCoverSearch(row, replace, opener) {
  cover.target = { row, replace };
  cover.chosen = null;
  // the name without the marks of file names: [title ID], [v0], (EU) and the extension
  const shown = shownName(row);
  const name = shown.replace(/\.(nup|nsp|nsz|xci|xcz)$/i, "").replace(/(\s*[\[(][^\])]*[\])])+/g, " ").replace(/\s+/g, " ").trim() || shown;
  $("cover-title").textContent = t("coverTitle")(name);
  $("cover-query").value = name;
  $("cover-grid").replaceChildren();
  $("cover-status").textContent = "";
  $("cover-use").disabled = true;
  openDialog($("cover-dialog"), opener);
  searchCovers();
}

async function searchCovers() {
  const round = ++cover.round;
  const query = $("cover-query").value.trim();
  cover.chosen = null;
  $("cover-use").disabled = true;
  $("cover-grid").replaceChildren();
  $("cover-status").classList.remove("warning");
  if (!query) {
    $("cover-status").textContent = t("coverWriteName");
    return;
  }
  $("cover-status").textContent = t("coverSearching");
  $("cover-search").disabled = true;
  try {
    const covers = await app().SearchCovers(query, systemOf("cover"));
    if (round !== cover.round) return;
    $("cover-status").textContent = covers.length ? "" : t("coverNone");
    for (const found of covers) {
      const tile = document.createElement("button");
      tile.type = "button";
      tile.className = "cover-tile";
      tile.innerHTML = `<img alt=""><span class="cover-name"></span><span class="cover-system"></span>`;
      tile.querySelector("img").src = found.image;
      tile.querySelector(".cover-name").textContent = found.title;
      tile.title = found.title;
      tile.querySelector(".cover-system").textContent = (found.systems || []).join(" · ");
      tile.setAttribute("aria-pressed", "false");
      tile.onclick = () => {
        for (const other of $("cover-grid").children) other.setAttribute("aria-pressed", "false");
        tile.setAttribute("aria-pressed", "true");
        cover.chosen = found.address;
        $("cover-use").disabled = false;
      };
      tile.ondblclick = () => {
        cover.chosen = found.address;
        useCover();
      };
      $("cover-grid").append(tile);
    }
  } catch (error) {
    if (round !== cover.round) return;
    $("cover-status").textContent = capitalize(noteText(String(error && error.message || error).replace(/^Error: /, "")));
    $("cover-status").classList.add("warning");
  } finally {
    if (round === cover.round) $("cover-search").disabled = false;
  }
}

function useCover() {
  const target = cover.target;
  const address = cover.chosen;
  if (!target || !address) return;
  $("cover-dialog").close();
  giveNupIcon(target.row, () => app().UseCover(target.row.item.path, address, target.replace));
}

// a note of NXPress begun with a capital, as a line of its own
function capitalize(text) {
  return text ? text[0].toUpperCase() + text.slice(1) : text;
}

// gives a NUP its icon, from its card, then reads the NUP again and tells what was done under the
// icon; with no image chosen, nothing. Why it got none stays in the card, not in a notice that goes
async function giveNupIcon(row, give) {
  const buttons = row.tr.querySelectorAll(".nup-no-icon button");
  for (const button of buttons) button.disabled = true;
  try {
    const note = await give();
    if (!note) {
      for (const button of buttons) button.disabled = state.busy;
      return;
    }
    row.iconFailed = "";
    row.iconChanging = false;
    row.iconNote = capitalize(noteText(note));
    row.card = await app().Card(row.item.path);
    renderCard(row);
    // the cover of its row, in the list and in the grid, is the new icon at once (#107)
    row.game = await app().Describe(row.item.path);
    paint(row);
  } catch (error) {
    row.iconFailed = capitalize(noteText(String(error && error.message || error).replace(/^Error: /, "")));
    renderCard(row);
  }
}

async function fixNupMeta(row, button) {
  button.disabled = true;
  try {
    const note = await app().FixNupMeta(row.item.path);
    showToast(noteText(note));
    row.card = await app().Card(row.item.path);
    renderCard(row);
    row.game = await app().Describe(row.item.path);
    paint(row);
  } catch (error) {
    button.disabled = false;
    showToast(noteText(String(error)));
  }
}

// the colors NXPress can wear, chosen in Settings: its own and three more with a scene (RuneStone, Vampire and Huntress), each with its background
// and its main color in dark and in light, for its swatch (the colors themselves are in style.css)
const palettes = [
  ["nxpress", "", ["#101216", "#3db7ea"], ["#eef0f3", "#0a6fb0"]],
  ["relic", "paletteRelic", ["#0b1316", "#4fe3ff"], ["#ebe2cc", "#0c6f86"]],
  ["vampire", "paletteVampire", ["#070a12", "#cdd9ff"], ["#cfd3e2", "#26336e"]],
  ["visor", "paletteVisor", ["#05070c", "#9cff3a"], ["#c9cfcf", "#2f6408"]],
];
function applyPalette() {
  if (state.palette === "nxpress") document.documentElement.removeAttribute("data-palette");
  else document.documentElement.dataset.palette = state.palette;
}
// the swatches of the colors, in the theme shown, the one chosen marked
function paintPalettes() {
  const box = $("palettes");
  const dark = state.theme === "dark" || (state.theme === "auto" && window.matchMedia("(prefers-color-scheme: dark)").matches);
  if (!box.children.length) {
    for (const [key] of palettes) {
      const button = document.createElement("button");
      button.className = "palette";
      button.setAttribute("role", "radio");
      button.dataset.palette = key;
      button.innerHTML = "<i></i><span></span>";
      button.onclick = () => { state.palette = key; save("palette", key); applyPalette(); paintPalettes(); };
      box.append(button);
    }
  }
  for (const button of box.children) {
    const [key, name, darkColors, lightColors] = palettes.find(palette => palette[0] === button.dataset.palette);
    const [ground, accent] = dark ? darkColors : lightColors;
    button.style.setProperty("--sw-bg", ground);
    button.style.setProperty("--sw-accent", accent);
    button.querySelector("span").textContent = name ? t(name) : "NXPress";
    button.classList.toggle("on", key === state.palette);
    button.setAttribute("aria-checked", String(key === state.palette));
  }
}

// the color of a cover: the mean of its pixels, the bright and colorful ones weighing more, so a
// card of the game can wear it; kept with the game once found
function coverTone(game) {
  if (!game || !game.icon) return "";
  if (game.tone !== undefined) return game.tone;
  game.tone = "";
  const image = new Image();
  image.onload = () => {
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 16;
    const context = canvas.getContext("2d", { willReadFrequently: true });
    context.drawImage(image, 0, 0, 16, 16);
    const pixels = context.getImageData(0, 0, 16, 16).data;
    let r = 0, g = 0, b = 0, sum = 0;
    for (let i = 0; i < pixels.length; i += 4) {
      const max = Math.max(pixels[i], pixels[i + 1], pixels[i + 2]), min = Math.min(pixels[i], pixels[i + 1], pixels[i + 2]);
      const weight = 1 + (max - min) / 32 + max / 255;
      r += pixels[i] * weight; g += pixels[i + 1] * weight; b += pixels[i + 2] * weight; sum += weight;
    }
    // its hue, a little livelier and never too dark or too light, so it shows on both themes
    r /= sum * 255; g /= sum * 255; b /= sum * 255;
    const max = Math.max(r, g, b), min = Math.min(r, g, b), light = (max + min) / 2;
    let hue = 0, saturation = 0;
    if (max > min) {
      saturation = (max - min) / (1 - Math.abs(2 * light - 1));
      hue = max === r ? ((g - b) / (max - min) + 6) % 6 : max === g ? (b - r) / (max - min) + 2 : (r - g) / (max - min) + 4;
    }
    game.tone = saturation < 0.08 ? "" : `hsl(${Math.round(hue * 60)} ${Math.round(Math.max(saturation, 0.55) * 100)}% ${Math.round(Math.min(Math.max(light, 0.42), 0.58) * 100)}%)`;
    document.dispatchEvent(new CustomEvent("nx-tone", { detail: game }));
  };
  image.src = game.icon;
  return "";
}

// over the cover and the name of a game read, a card with what it is: its publisher, title ID,
// version, the titles of each kind, the game of an update or a DLC, the version of its game a
// DLC needs, the firmware it needs and its languages
let popTimer = 0;
let popLeave = 0, popRow = null;
function showPop(row, anchor) {
  const game = row.game;
  if (!game || !game.titleId || !anchor.isConnected) return;
  // the menu of the cover is open: the card stays away, it would cover the choices
  if (document.querySelector(".ncover-menu:not([hidden])")) return;
  const pop = $("game-pop");
  const image = pop.querySelector("img");
  image.hidden = !game.icon;
  if (game.icon) image.src = game.icon;
  pop.querySelector(".pop-name").textContent = shownName(row);
  pop.querySelector(".pop-publisher").textContent = game.publisher || "";
  // each title the file holds with its ID and version once they are known (partsOfRow), else the
  // one title the card told before
  const parts = partsOfRow(row);
  const facts = parts.length ? partFacts(parts) : [
    ["Title ID", game.titleId],
    [t("popKind"), t("gameKind")(game.type) + (game.demo ? " · " + t("popDemo") : "")],
    [t("popVersion"), (game.versionTxt ? game.versionTxt + " · " : "") + "v" + game.latest],
  ];
  if (game.titles > 1) facts.push([t("popHolds"), t("popTitles")(game.bases, game.updates, game.dlcs)]);
  if (game.gameId) facts.push([t("popGame"), game.gameId]);
  if (game.needsGame) facts.push([t("popNeedsGame"), "v" + game.needsGame]);
  if (game.firmware) facts.push([t("popFirmware"), game.firmware]);
  // the newest version published, in blue when the list does not have it
  const online = row.online;
  if (online) {
    const newer = online.version > online.reach;
    facts.push([t("popNewest"), `v${online.version}${newestOf(row, online)}` + (newer ? (online.date ? " · " + day(online.date) : "") : " · " + t("popNewestHave")), newer ? "pop-newer" : ""]);
  }
  const list = pop.querySelector("dl");
  list.innerHTML = "";
  // Holds goes on a list of its own, at the bottom of the card, under a line, and the newest version
  // on one just above that line (wireCard makes them)
  const holds = pop.querySelector("dl.pop-holds"), newest = pop.querySelector("dl.pop-newest");
  if (holds) holds.innerHTML = "";
  if (newest) newest.innerHTML = "";
  for (const [name, value, tone] of facts) {
    const term = document.createElement("dt");
    const detail = document.createElement("dd");
    term.textContent = name;
    if (value && typeof value === "object") {
      // pieces joined by " - ", each one the text it is or None where the file does not tell it
      value.segments.forEach((piece, index) => {
        if (index) detail.append(" - ");
        if (piece === null) {
          const none = document.createElement("span");
          none.className = "pop-none";
          none.textContent = t("npopNone");
          detail.append(none);
        } else detail.append(piece);
      });
    } else detail.textContent = value;
    if (tone) detail.className = tone;
    (holds && name === t("popHolds") ? holds : newest && name === t("popNewest") ? newest : list).append(term, detail);
  }
  // the languages by their first tag: EN for en-US and en-GB
  const languages = pop.querySelector(".pop-languages");
  languages.innerHTML = "";
  languages.hidden = !(game.languages || []).length;
  for (const code of new Set((game.languages || []).map(tag => tag.split("-")[0].toUpperCase()))) {
    const chip = document.createElement("i");
    chip.textContent = code;
    languages.append(chip);
  }
  // beside a cover of the grid, else under the name; above or to the left when there is no room
  pop.hidden = false;
  const box = anchor.getBoundingClientRect();
  const width = pop.offsetWidth, height = pop.offsetHeight, margin = 8;
  let left, top, above = false;
  if ($("files").classList.contains("grid")) {
    left = box.right + margin + width < innerWidth ? box.right + margin : box.left - margin - width;
    top = box.top;
  } else {
    left = box.left + 44;
    top = box.bottom + 4 + height < innerHeight - margin ? box.bottom + 4 : box.top - 4 - height;
    // the close button is at the corner nearest the row: the top one when the card is under it, the
    // bottom one when it is above
    above = top < box.top;
  }
  pop.classList.toggle("above", above);
  pop.style.left = Math.max(margin, Math.min(left, innerWidth - width - margin)) + "px";
  pop.style.top = Math.max(margin, Math.min(top, innerHeight - height - margin)) + "px";
  // the card wears the color of the cover, as soon as it is known
  const tone = coverTone(game);
  pop.style.setProperty("--pop-tone", tone || "var(--accent)");
  popGame = game;
  // the cover of the row grows into the one of the card
  const small = row.tr.querySelector("img.icon:not([hidden])");
  if (game.icon && small && !calm()) {
    const from = small.getBoundingClientRect(), to = image.getBoundingClientRect();
    pop.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 180, easing: "ease-out" });
    image.animate([
      { transform: `translate(${from.left - to.left}px, ${from.top - to.top}px) scale(${from.width / to.width})`, transformOrigin: "0 0", borderRadius: "16px" },
      { transform: "none", transformOrigin: "0 0" },
    ], { duration: 260, easing: "cubic-bezier(.2, .8, .2, 1)" });
  } else replay("enter", pop);
}
// a color found while the card is open
let popGame = null;
document.addEventListener("nx-tone", event => {
  if (event.detail === popGame && !$("game-pop").hidden) $("game-pop").style.setProperty("--pop-tone", event.detail.tone || "var(--accent)");
});
function hidePop() {
  clearTimeout(popTimer);
  $("game-pop").hidden = true;
}
// leaving the name of the game does not close its card at once: the mouse may be going into it, to
// select what it says or to right click and copy it
function hidePopSoon() {
  clearTimeout(popTimer);
  clearTimeout(popLeave);
  popLeave = setTimeout(hidePop, 220);
}
// what the card says, as lines of text
function popText() {
  const pop = $("game-pop");
  const lines = [pop.querySelector(".pop-name").textContent];
  const publisher = pop.querySelector(".pop-publisher").textContent;
  if (publisher) lines.push(publisher);
  lines.push("");
  const terms = pop.querySelectorAll("dt"), details = pop.querySelectorAll("dd");
  terms.forEach((term, i) => lines.push(`${term.textContent}: ${details[i].textContent}`));
  const languages = [...pop.querySelectorAll(".pop-languages i")].map(chip => chip.textContent);
  if (languages.length) lines.push("", languages.join(", "));
  return lines.join("\n");
}
// ---- the cover of the NUP of a dump folder (Combine) ----
// Each dump folder in the list can be given a cover: a picture, cut to a square and made 256 × 256
// pixels here, with whatever the window can show (JPG, PNG, BMP, WebP, AVIF and the rest). NXPress
// is given the JPEG at once and keeps it for the folder: the NUP made of it carries it as its icon,
// in the place of the one of its control data or of titledb. The files of the game do not change.
const COVER_SIZE = 256, COVER_MAX_PIXELS = 50_000_000;
// the formats the picture can be in, as the file window and the cut take them
const COVER_FORMATS = "JPG, PNG, BMP, WebP, AVIF, GIF, ICO";
// by the path of the dump folder: { data: the JPEG in base64, url: it to show, path: the picture,
// view: how it was cut, to edit it again }
const covers = new Map();
// the cover being cut: { row, path, image, width, height, zoom, cx, cy, fit, drag }
let coverEdit = null;

const coverSide = c => Math.min(c.width, c.height) / c.zoom;
// the square is at most the picture, and at most 8 times smaller; its centre stays inside it
function coverClamp(c) {
  c.zoom = Math.max(1, Math.min(8, Math.min(c.width, c.height), c.zoom));
  const half = coverSide(c) / 2;
  c.cx = Math.max(half, Math.min(c.width - half, c.cx));
  c.cy = Math.max(half, Math.min(c.height - half, c.cy));
}

// coverPaint draws the cover on white, as it will be: the square, or the whole picture when it fits
function coverPaint(ctx, c, size, careful) {
  ctx.fillStyle = "#fff";
  ctx.fillRect(0, 0, size, size);
  ctx.imageSmoothingEnabled = true;
  ctx.imageSmoothingQuality = "high";
  if (c.fit) {
    const scale = Math.min(1, size / c.width, size / c.height);
    const w = Math.max(1, Math.round(c.width * scale)), h = Math.max(1, Math.round(c.height * scale));
    ctx.drawImage(c.image, Math.floor((size - w) / 2), Math.floor((size - h) / 2), w, h);
    return;
  }
  const side = coverSide(c);
  let source = c.image, x = c.cx - side / 2, y = c.cy - side / 2, w = side;
  // a picture many times bigger is halved down in steps: one jump to 256 pixels leaves jagged edges
  while (careful && w > size * 2) {
    const next = Math.max(size, Math.floor(w / 2));
    const step = document.createElement("canvas");
    step.width = step.height = next;
    const stepContext = step.getContext("2d");
    stepContext.imageSmoothingQuality = "high";
    stepContext.drawImage(source, x, y, w, w, 0, 0, next, next);
    source = step; x = 0; y = 0; w = next;
  }
  ctx.drawImage(source, x, y, w, w, 0, 0, size, size);
}

function coverRedraw() {
  const c = coverEdit;
  if (!c) return;
  const canvas = $("ncover-canvas");
  coverPaint(canvas.getContext("2d"), c, COVER_SIZE, false);
  canvas.classList.toggle("fit", c.fit);
  canvas.setAttribute("aria-label", t("ncoverTitle")(shownName(c.row)));
  $("ncover-zoom").disabled = c.fit;
  $("ncover-zoom").value = c.zoom;
}

// coverEncode makes the JPEG of the cover, the one NXPress checks again before it packs it
function coverEncode(c) {
  const canvas = document.createElement("canvas");
  canvas.width = canvas.height = COVER_SIZE;
  coverPaint(canvas.getContext("2d"), c, COVER_SIZE, true);
  const url = canvas.toDataURL("image/jpeg", 0.92);
  if (!url.startsWith("data:image/jpeg;base64,")) throw new Error("this window does not make JPEG images");
  return { url, data: url.slice(url.indexOf(",") + 1) };
}

// coverLoad reads a picture of the computer into an image the window can cut
async function coverLoad(path) {
  const image = new Image();
  image.src = path.startsWith("data:") ? path : await app().CoverPicture(path);
  await image.decode();
  const width = image.naturalWidth, height = image.naturalHeight;
  if (!width || !height) throw t("ncoverUnreadable");
  if (width * height > COVER_MAX_PIXELS) throw `${width} × ${height}: at most 50 megapixels`;
  return { image, width, height };
}

// openCover opens the cut of a picture, for the cover of a dump folder; it gives "" or why it could
// not (the reasons of NXPress come as text, the ones of the window as errors)
async function openCover(row, path, view, apply) {
  let loaded;
  try { loaded = await coverLoad(path); } catch (error) { return typeof error === "string" ? error : t("ncoverUnreadable"); }
  const dialog = coverDialog();
  coverEdit = { row, path, ...loaded, zoom: 1, cx: loaded.width / 2, cy: loaded.height / 2, fit: false, drag: null, ...(view || {}), apply };
  coverClamp(coverEdit);
  $("ncover-title").textContent = t("ncoverTitle")(shownName(row));
  $("ncover-intro").textContent = t("ncoverIntro");
  $("ncover-zoom-label").textContent = t("ncoverZoom");
  $("ncover-fit-label").textContent = t("ncoverFit");
  $("ncover-other").textContent = t("ncoverChoose");
  $("ncover-cancel").textContent = t("cancel");
  $("ncover-apply").textContent = t("ncoverApply");
  $("ncover-fit").checked = coverEdit.fit;
  $("ncover-zoom").max = Math.max(1, Math.min(8, Math.min(loaded.width, loaded.height)));
  $("ncover-error").hidden = true;
  coverRedraw();
  openDialog(dialog);
  $("ncover-canvas").focus();
  return "";
}
async function chooseCover(row) {
  if (state.busy) return;
  const path = await app().ChooseCoverPicture();
  if (!path) return;
  const why = await openCover(row, path);
  if (why) showToast(why);
}
async function editCover(row) {
  const entry = covers.get(row.item.path);
  if (state.busy || !entry) return;
  const why = await openCover(row, entry.path, entry.view);
  if (why) showToast(why);
}

// the cover of a dump folder in its row: its thumbnail, and the button that adds or edits it
function coverControl(row) {
  const entry = covers.get(row.item.path);
  const box = document.createElement("div");
  box.className = "ncover-row";
  const open = () => (entry ? editCover(row) : chooseCover(row));
  // the icon the control data of the game already has, when no cover was chosen: shown in the place
  // of the cover, and the button offers to replace it
  const found = !entry && row.game && row.game.controlIcon;
  const thumb = document.createElement(entry || found ? "img" : "span");
  thumb.className = "ncover-thumb" + (found ? " found" : "");
  if (entry) { thumb.src = entry.url; thumb.alt = ""; thumb.onclick = open; }
  if (found) { thumb.src = row.game.controlIcon; thumb.alt = ""; thumb.title = t("ncoverFound"); }
  // an edit button when there is a cover, and one button that opens a menu to get one: from a file, from the
  // Nintendo eShop or from SteamGridDB; under it the formats it takes
  const side = document.createElement("div");
  side.className = "ncover-side";
  const line = document.createElement("div");
  line.className = "ncover-line";
  const accepts = document.createElement("small");
  accepts.className = "ncover-accepts";
  accepts.textContent = t("ncoverAccepts")(COVER_FORMATS);
  const menu = document.createElement("div");
  menu.className = "ncover-menu";
  menu.setAttribute("role", "menu");
  menu.hidden = true;
  const pick = document.createElement("button");
  pick.className = "ncover-set";
  pick.setAttribute("aria-haspopup", "menu");
  pick.setAttribute("aria-expanded", "false");
  pick.textContent = t(found ? "ncoverReplace" : "ncoverMenu") + " ▾";
  pick.disabled = state.busy;
  const close = () => {
    menu.hidden = true;
    pick.setAttribute("aria-expanded", "false");
    document.removeEventListener("click", outside, true);
    document.removeEventListener("keydown", escape, true);
  };
  const outside = event => { if (!menu.contains(event.target) && event.target !== pick) close(); };
  const escape = event => { if (event.key === "Escape") { event.stopPropagation(); close(); pick.focus(); } };
  pick.onclick = () => {
    if (!menu.hidden) { close(); return; }
    hidePop();
    menu.hidden = false;
    pick.setAttribute("aria-expanded", "true");
    document.addEventListener("click", outside, true);
    document.addEventListener("keydown", escape, true);
    menu.querySelector("button").focus();
  };
  for (const [key, action] of [["ncoverFile", () => chooseCover(row)], ["ncoverMenuShop", () => shopCover(row)], ["ncoverMenuGrid", () => sgdbCover(row)]]) {
    const item = document.createElement("button");
    item.type = "button";
    item.setAttribute("role", "menuitem");
    item.textContent = t(key);
    item.onclick = () => { close(); action(); };
    menu.append(item);
  }
  if (entry) {
    const set = document.createElement("button");
    set.className = "ncover-set";
    set.textContent = t("ncoverEdit");
    set.disabled = state.busy;
    set.onclick = open;
    line.append(set);
  }
  line.append(pick);
  side.append(line, accepts, menu);
  if (found) {
    const note = document.createElement("small");
    note.className = "ncover-found";
    note.textContent = "✓ " + t("ncoverFound");
    side.insertBefore(note, accepts);
  }
  box.append(thumb, side);
  if (entry) {
    const clear = document.createElement("button");
    clear.className = "quiet ncover-clear";
    clear.textContent = "✕";
    clear.title = t("ncoverClear");
    clear.setAttribute("aria-label", t("ncoverClear"));
    clear.disabled = state.busy;
    clear.onclick = async () => {
      try { await app().SetNupCover(row.item.path, ""); } catch (error) { showToast(String(error)); return; }
      covers.delete(row.item.path);
      paint(row);
    };
    line.append(clear);
  }
  return box;
}

// coverDialog is the window of the cut, made the first time it is needed
function coverDialog() {
  let dialog = $("ncover-dialog");
  if (dialog) return dialog;
  dialog = document.createElement("dialog");
  dialog.id = "ncover-dialog";
  dialog.className = "ncover-dialog";
  dialog.setAttribute("aria-labelledby", "ncover-title");
  dialog.innerHTML = `<h2 id="ncover-title"></h2><p class="muted" id="ncover-intro"></p>
    <div class="ncover-stage"><canvas id="ncover-canvas" width="256" height="256" tabindex="0" role="img"></canvas></div>
    <label class="ncover-zoom"><span id="ncover-zoom-label"></span><input id="ncover-zoom" type="range" min="1" max="8" step="0.01" value="1"></label>
    <label class="check"><input id="ncover-fit" type="checkbox"><span id="ncover-fit-label"></span></label>
    <p class="update-error" id="ncover-error" role="alert" hidden></p>
    <div class="dialog-actions"><button id="ncover-other" class="quiet"></button><button id="ncover-cancel"></button><button id="ncover-apply" class="primary"></button></div>`;
  document.body.append(dialog);
  const canvas = $("ncover-canvas");
  // the picture cut is let go of (it can be big) once the window closes
  dialog.addEventListener("close", () => { coverEdit = null; });
  $("ncover-cancel").onclick = () => dialog.close();
  $("ncover-other").onclick = async () => {
    const path = await app().ChooseCoverPicture();
    if (!path || !coverEdit) return;
    // another picture goes where the first one was going: the cover of the folder, or the icon of a NUP
    const why = await openCover(coverEdit.row, path, undefined, coverEdit.apply);
    if (why) { $("ncover-error").textContent = why; $("ncover-error").hidden = false; }
  };
  $("ncover-apply").onclick = async () => {
    const c = coverEdit;
    if (!c) return;
    try {
      const made = coverEncode(c);
      if (c.apply) {
        // the cut goes to whoever asked for it, as the JPEG and its URL
        c.apply(made);
      } else {
        await app().SetNupCover(c.row.item.path, made.data);
        covers.set(c.row.item.path, { ...made, path: c.path, view: { zoom: c.zoom, cx: c.cx, cy: c.cy, fit: c.fit } });
      }
    } catch (error) {
      $("ncover-error").textContent = (error && error.message) || String(error) || t("ncoverUnreadable");
      $("ncover-error").hidden = false;
      return;
    }
    dialog.close();
    if (!c.apply) paint(c.row);
  };
  $("ncover-zoom").oninput = () => { if (!coverEdit) return; coverEdit.zoom = +$("ncover-zoom").value; coverClamp(coverEdit); coverRedraw(); };
  $("ncover-fit").onchange = () => { if (!coverEdit) return; coverEdit.fit = $("ncover-fit").checked; coverRedraw(); };
  // dragging moves the picture under the square; the wheel and the keys zoom and move it too
  canvas.addEventListener("pointerdown", event => {
    if (!coverEdit || coverEdit.fit) return;
    coverEdit.drag = { x: event.clientX, y: event.clientY };
    canvas.setPointerCapture(event.pointerId);
  });
  canvas.addEventListener("pointermove", event => {
    const c = coverEdit;
    if (!c || !c.drag) return;
    const unit = coverSide(c) / canvas.clientWidth;
    c.cx -= (event.clientX - c.drag.x) * unit;
    c.cy -= (event.clientY - c.drag.y) * unit;
    c.drag = { x: event.clientX, y: event.clientY };
    coverClamp(c);
    coverRedraw();
  });
  const release = () => { if (coverEdit) coverEdit.drag = null; };
  canvas.addEventListener("pointerup", release);
  canvas.addEventListener("pointercancel", release);
  canvas.addEventListener("wheel", event => {
    if (!coverEdit || coverEdit.fit) return;
    event.preventDefault();
    coverEdit.zoom *= event.deltaY < 0 ? 1.1 : 1 / 1.1;
    coverClamp(coverEdit);
    coverRedraw();
  }, { passive: false });
  canvas.addEventListener("keydown", event => {
    const c = coverEdit;
    if (!c || c.fit) return;
    const step = coverSide(c) * 0.05;
    const moves = { ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, -step], ArrowDown: [0, step] };
    if (moves[event.key]) { c.cx += moves[event.key][0]; c.cy += moves[event.key][1]; }
    else if (event.key === "+" || event.key === "=") c.zoom *= 1.1;
    else if (event.key === "-") c.zoom /= 1.1;
    else return;
    event.preventDefault();
    coverClamp(c);
    coverRedraw();
  });
  return dialog;
}

// wireCover adds the cover to each dump folder of the list from outside, as its row is drawn, so
// that the code that draws the rows stays as it is
function wireCover() {
  new MutationObserver(records => {
    for (const record of records) {
      for (const node of record.addedNodes) {
        if (node.nodeType !== 1) continue;
        const plans = node.matches(".dump-plan") ? [node] : [...node.querySelectorAll(".dump-plan")];
        for (const plan of plans) {
          const row = rowOfLine(plan.closest("tr"));
          if (row && !plan.querySelector(".ncover-row")) plan.append(coverControl(row));
          if (row) showNupName(row);
        }
      }
    }
  }).observe($("files").tBodies[0], { childList: true, subtree: true });
  // the template of the name of each SuperNSP is also the one of the NUP: written, it changes the names
  let wait = 0;
  $("pack-template").addEventListener("input", () => { clearTimeout(wait); wait = setTimeout(refreshNupNames, 200); });
  $("pack-template-reset").addEventListener("click", () => setTimeout(refreshNupNames, 300));
}

// ---- the card of a game over its name, in Details: its titles, and its details ----
// the titles of a file as the card tells them: from its dump folder when it is one, else from the
// file itself, asked of NXPress when the mouse is on the name (CardParts)
function partsOfRow(row) {
  if (row.parts && row.parts.length) return row.parts;
  const pieces = row.game && row.game.pieces;
  return pieces && pieces.length ? pieces.map(piece => ({ kind: piece.type, titleId: piece.titleId, version: piece.version, unknown: true })) : [];
}
async function loadParts(row) {
  if (row.parts || row.partsLoading || partsOfRow(row).length) return;
  row.partsLoading = true;
  try { row.parts = await app().CardParts(row.item.path); } catch (error) { row.parts = []; }
  row.partsLoading = false;
  // the card is open for this row: it is drawn again with the titles
  const anchor = row.tr.querySelector(".game");
  if (popRow === row && !$("game-pop").hidden && anchor) showPop(row, anchor);
}
// the rows of the card: Base, Update and DLC, each with its title ID and version; of a game with
// many DLC the first three, and how many more
function partFacts(parts) {
  const label = { BASE: t("npopBase"), UPD: t("gameKind")("UPD"), DLC: t("gameKind")("DLC") };
  const order = { BASE: 0, UPD: 1, DLC: 2 };
  const sorted = [...parts].sort((a, b) => (order[a.kind] ?? 3) - (order[b.kind] ?? 3));
  const facts = [];
  let dlc = 0;
  for (const part of sorted) {
    if (part.kind === "DLC" && ++dlc > 3) continue;
    // ID - internal version - the version the game shows: 040077201BE50800 - v458752 - 1.3.3. What the file
    // does not tell is None (in red, with a border): the ID, the internal version, and for a game or an update
    // the version it shows; a DLC has none to show, and a dump folder does not tell it
    const has = value => value !== undefined && value !== null && String(value).trim() !== "";
    const segments = [
      has(part.titleId) ? String(part.titleId).toUpperCase() : null,
      has(part.version) ? "v" + part.version : null,
    ];
    if (part.kind !== "DLC" && !part.unknown) segments.push(has(part.shown) ? String(part.shown) : null);
    facts.push([label[part.kind] || part.kind, { segments }, "pop-part"]);
  }
  if (dlc > 3) facts.push([label.DLC, t("npopMore")(dlc - 3), "pop-part"]);
  return facts;
}

// newestOf tells what the newest version published belongs to, beside it: " (Base)", " (Update)" or
// " (DLC)", by the title of the file that has that version; a version nobody here has is an update (a
// game is at version 0 until it has one). Nothing until the titles of the file are known.
function newestOf(row, online) {
  const parts = partsOfRow(row);
  if (!parts.length) return "";
  const has = kind => parts.some(part => part.kind === kind && Number(part.version) === Number(online.version));
  const kind = ["UPD", "BASE", "DLC"].find(has) || (Number(online.version) > 0 ? "UPD" : "BASE");
  const label = { BASE: t("npopBase"), UPD: t("gameKind")("UPD"), DLC: t("gameKind")("DLC") }[kind];
  return ` (${label})`;
}

// copyReportOf copies the full details of a file, as Copy full details of its menu does
async function copyReportOf(row) {
  const text = await reportOf(row);
  if (text) copyText(text, t("copiedDetails"));
}

// the window of the details of a file: all of what Copy full details copies, in a window that moves
// by its bar and can be sized, with its text selectable and a button to copy it
async function openDetailsInside(row) {
  hidePop();
  document.getElementById("nvd-window")?.remove();
  const box = document.createElement("div");
  box.id = "nvd-window";
  box.className = "nvd-window";
  box.setAttribute("role", "dialog");
  box.innerHTML = `<div class="nvd-bar"><span class="nvd-title"></span><button type="button" class="quiet nvd-close"></button></div><pre class="nvd-text" tabindex="0"></pre><div class="nvd-foot"><button type="button" class="primary nvd-copy"></button></div>`;
  const title = shownName(row);
  box.setAttribute("aria-label", title);
  box.querySelector(".nvd-title").textContent = title;
  const close = box.querySelector(".nvd-close"), copy = box.querySelector(".nvd-copy"), text = box.querySelector(".nvd-text");
  close.textContent = "✕";
  close.title = t("nvdClose");
  close.setAttribute("aria-label", t("nvdClose"));
  copy.textContent = t("nvdCopy");
  text.textContent = "…";
  document.body.append(box);
  // in the middle of the window, then where the person moves it
  const width = box.offsetWidth, height = box.offsetHeight;
  box.style.left = Math.max(8, (innerWidth - width) / 2) + "px";
  box.style.top = Math.max(8, (innerHeight - height) / 2) + "px";
  const shut = () => { box.remove(); document.removeEventListener("keydown", onKey, true); };
  const onKey = event => { if (event.key === "Escape") { event.stopPropagation(); shut(); } };
  document.addEventListener("keydown", onKey, true);
  close.onclick = shut;
  const bar = box.querySelector(".nvd-bar");
  let grab = null;
  bar.addEventListener("pointerdown", event => {
    if (event.target.closest("button")) return;
    grab = { x: event.clientX - box.offsetLeft, y: event.clientY - box.offsetTop };
    bar.setPointerCapture(event.pointerId);
  });
  bar.addEventListener("pointermove", event => {
    if (!grab) return;
    box.style.left = Math.max(0, Math.min(innerWidth - 80, event.clientX - grab.x)) + "px";
    box.style.top = Math.max(0, Math.min(innerHeight - 40, event.clientY - grab.y)) + "px";
  });
  const drop = () => { grab = null; };
  bar.addEventListener("pointerup", drop);
  bar.addEventListener("pointercancel", drop);
  const report = await reportOf(row);
  text.textContent = report || "";
  copy.onclick = () => { if (report) copyText(report, t("copiedDetails")); };
  text.focus();
}

// openDetailsWindow shows the details of a file in a window of the system, out of this one (NXPress starts
// another that shows only them: OpenDetailsWindow); when that cannot be, in the window inside this one
async function openDetailsWindow(row) {
  hidePop();
  const text = await reportOf(row);
  if (!text) return;
  try {
    await app().OpenDetailsWindow(shownName(row) + " · NXPress", text, t("nvdCopy"), t("copiedDetails"), t("export"), t("exported")("{name}"), shownName(row));
  } catch (error) {
    openDetailsInside(row);
  }
}

// wireCard adds, under the cover of the card of a game, the buttons that copy its details and open
// them in a window of their own
function wireCard() {
  const pop = $("game-pop"), image = pop.querySelector("img");
  const side = document.createElement("div");
  side.className = "pop-side";
  const actions = document.createElement("div");
  actions.className = "pop-actions";
  for (const [name, run] of [["copy", row => copyReportOf(row)], ["view", row => openDetailsWindow(row)]]) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "pop-action pop-" + name;
    button.addEventListener("click", () => { if (popRow) run(popRow); });
    actions.append(button);
  }
  image.replaceWith(side);
  side.append(image, actions);
  // the lists of the newest version and of Holds, under the one of the titles: the newest just above
  // the line, Holds under it, at the bottom of the card
  const newest = document.createElement("dl");
  newest.className = "pop-newest";
  const holds = document.createElement("dl");
  holds.className = "pop-holds";
  pop.querySelector("dl").after(newest, holds);
}
// the words of the two buttons, in the language of the window
function labelCard() {
  const copy = document.querySelector("#game-pop .pop-copy"), view = document.querySelector("#game-pop .pop-view");
  if (copy) copy.textContent = t("npopCopy");
  if (view) view.textContent = t("npopView");
  // a dump folder has no file to tell the details of: the card of Combine has no buttons
  const actions = document.querySelector("#game-pop .pop-actions");
  if (actions) actions.hidden = state.mode === "pack" || !!(popRow && popRow.item && popRow.item.dump);
}

// ---- Split: what to take out of each NUP ----
// At the right of each NUP of the list, in Split, its own choices: Base, Update, DLC and Icon, one
// under the other. All of them are always there; the ones the NUP does not have are shown off. All
// the ones it has are chosen at first. NXPress reads and writes only what is chosen of each NUP, and
// is told as it changes. Once a work has begun on the NUP its line tells how it went instead.
const splitPicks = new Map();   // NUP -> { base, update, dlc, icon }
function splitPickOf(path) {
  if (!splitPicks.has(path)) splitPicks.set(path, { base: true, update: true, dlc: true, icon: true });
  return splitPicks.get(path);
}
function ensureSplitControl(tr) {
  const row = rowOfLine(tr);
  const wanted = !!row && state.mode === "split" && isNup(row.item.path) && !row.status;
  const have = tr.querySelector(".nsplit-row");
  if (!wanted) { if (have) have.remove(); return; }
  const cell = tr.querySelector("td.state");
  if (!cell) return;
  // what the NUP holds: its packages, asked once, and whether it has an icon; until they are known
  // each is taken as there
  if (!row.parts && !row.partsLoading) loadParts(row).then(() => ensureSplitControl(tr));
  const kinds = new Set((row.parts || []).map(part => part.kind));
  const known = !!(row.parts && row.parts.length);
  const has = { base: !known || kinds.has("BASE"), update: !known || kinds.has("UPD"), dlc: !known || kinds.has("DLC"), icon: row.game ? !!row.game.icon : true };
  const shape = JSON.stringify([has, t("nsplitBase")]);
  if (have && have.dataset.shape === shape) return;
  if (have) have.remove();
  const pick = splitPickOf(row.item.path);
  const box = document.createElement("div");
  box.className = "nsplit-row";
  box.dataset.shape = shape;
  // no caption: the words that explain the boxes are their tooltip
  box.title = t("nsplitTitle") + ". " + t("nsplitHint");
  for (const [name, label] of [["base", t("nsplitBase")], ["update", t("nsplitUpdate")], ["dlc", t("nsplitDlc")], ["icon", t("neditIcon")]]) {
    const item = document.createElement("label");
    item.className = "check nsplit-check" + (has[name] ? "" : " is-off");
    const input = document.createElement("input");
    input.type = "checkbox";
    input.disabled = !has[name];
    input.checked = has[name] && pick[name];
    input.addEventListener("change", () => {
      pick[name] = input.checked;
      app().SetSplitParts(row.item.path, pick.base, pick.update, pick.dlc, pick.icon);
    });
    const text = document.createElement("span");
    text.textContent = label;
    item.append(input, text);
    box.append(item);
  }
  cell.append(box);
}

// ---- Rename: the title name and the icon of a NUP, in its metadata ----
let nupInfo = null;   // the NUP being edited: { row, icon: the cut chosen, or null }

// ensureEditControl gives a NUP of the list, in Rename, the button that opens the editor of its title
// and icon, and takes it away from a line that is not one or when the section is another
function ensureEditControl(tr) {
  const row = rowOfLine(tr);
  const wanted = !!row && state.mode === "rename" && isNup(row.item.path);
  const have = tr.querySelector(".nedit-row");
  if (!wanted) { if (have) have.remove(); return; }
  const cell = tr.querySelector("td.name");
  if (!cell) return;
  if (have) {
    const button = have.querySelector("button"), label = t("neditButton");
    if (button.textContent !== label) button.textContent = label;
    return;
  }
  const box = document.createElement("div");
  box.className = "nedit-row";
  const button = document.createElement("button");
  button.type = "button";
  button.className = "ncover-set";
  button.textContent = t("neditButton");
  button.disabled = state.busy;
  button.onclick = () => editNupInfo(row);
  box.append(button);
  // under the title and the file name, in line with their text, not under the icon
  (cell.querySelector(".game > div:last-child") || cell).append(box);
}

// nupInfoDialog is the window of the editor, made the first time it is needed
function nupInfoDialog() {
  let dialog = $("nedit-dialog");
  if (dialog) return dialog;
  dialog = document.createElement("dialog");
  dialog.id = "nedit-dialog";
  dialog.className = "ncover-dialog nedit-dialog";
  dialog.setAttribute("aria-labelledby", "nedit-title");
  dialog.innerHTML = `<h2 id="nedit-title"></h2>
    <label class="nedit-field"><span id="nedit-name-label"></span><input id="nedit-name" type="text" autocomplete="off" spellcheck="false"></label>
    <div class="nedit-icon"><img id="nedit-icon" alt=""><div class="nedit-icon-side"><span id="nedit-icon-label"></span><button type="button" id="nedit-pick" class="ncover-set"></button><button type="button" id="nedit-shop" class="ncover-set"></button><button type="button" id="nedit-grid" class="ncover-set"></button><small id="nedit-accepts" class="ncover-accepts"></small></div></div>
    <p class="muted" id="nedit-note"></p>
    <p class="update-error" id="nedit-error" role="alert" hidden></p>
    <div class="dialog-actions"><button type="button" id="nedit-cancel"></button><button type="button" id="nedit-save" class="primary"></button></div>`;
  document.body.append(dialog);
  const refresh = () => {
    if (!nupInfo) return;
    const name = $("nedit-name").value.trim();
    $("nedit-save").disabled = !((name && name !== nupInfo.name) || nupInfo.icon);
  };
  $("nedit-name").addEventListener("input", refresh);
  dialog.addEventListener("close", () => { nupInfo = null; });
  $("nedit-cancel").onclick = () => dialog.close();
  // the icon cut, from a picture of the computer, the eShop or SteamGridDB, becomes the one to save
  const takeIcon = made => {
    if (!nupInfo) return;
    nupInfo.icon = made;
    $("nedit-icon").src = made.url;
    $("nedit-icon").hidden = false;
    refresh();
  };
  $("nedit-pick").onclick = async () => {
    const info = nupInfo;
    if (!info) return;
    const path = await app().ChooseCoverPicture();
    if (!path || !nupInfo) return;
    const why = await openCover(info.row, path, undefined, takeIcon);
    if (why) { $("nedit-error").textContent = why; $("nedit-error").hidden = false; }
  };
  $("nedit-shop").onclick = () => { if (nupInfo) shopCover(nupInfo.row, takeIcon); };
  $("nedit-grid").onclick = () => { if (nupInfo) sgdbCover(nupInfo.row, takeIcon); };
  $("nedit-save").onclick = async () => {
    const info = nupInfo;
    if (!info) return;
    const name = $("nedit-name").value.trim();
    const newName = name && name !== info.name ? name : "";
    $("nedit-error").hidden = true;
    $("nedit-save").disabled = true;
    try {
      await app().EditNup(info.row.item.path, newName, info.icon ? info.icon.data : "");
    } catch (error) {
      $("nedit-error").textContent = String((error && error.message) || error);
      $("nedit-error").hidden = false;
      $("nedit-save").disabled = false;
      return;
    }
    // the row tells what the NUP now says of itself
    const row = info.row;
    if (row.game) {
      if (newName) row.game.name = newName;
      if (info.icon) row.game.icon = info.icon.url;
    }
    row.reportText = "";
    dialog.close();
    showToast(t("neditSaved"));
    paint(row);
  };
  return dialog;
}

// editNupInfo opens the editor of the title name and the icon of a NUP of the list
function editNupInfo(row) {
  if (state.busy) return;
  const dialog = nupInfoDialog();
  const name = (row.game && row.game.name) || shownName(row);
  nupInfo = { row, icon: null, name };
  $("nedit-title").textContent = t("neditTitle");
  $("nedit-name-label").textContent = t("neditName");
  $("nedit-name").value = name;
  $("nedit-icon-label").textContent = t("neditIcon");
  $("nedit-pick").textContent = t("neditPick");
  $("nedit-shop").textContent = t("ncoverShop");
  $("nedit-grid").textContent = t("ncoverGrid");
  $("nedit-accepts").textContent = t("ncoverAccepts")(COVER_FORMATS);
  $("nedit-note").textContent = t("neditNote");
  $("nedit-cancel").textContent = t("cancel");
  $("nedit-save").textContent = t("neditSave");
  $("nedit-save").disabled = true;
  $("nedit-error").hidden = true;
  const image = $("nedit-icon");
  image.hidden = !(row.game && row.game.icon);
  if (row.game && row.game.icon) image.src = row.game.icon;
  openDialog(dialog);
  $("nedit-name").focus();
  $("nedit-name").select();
}

// the controls a line of the list gets in the section it is in: Split and Rename have their own
function ensureRowControls(tr) {
  ensureSplitControl(tr);
  ensureEditControl(tr);
}

function wireRename() {
  // a hint in the options of Rename
  const anchor = document.querySelector('section.block[data-for="rename"]');
  if (anchor) {
    const hint = document.createElement("section");
    hint.className = "block";
    hint.id = "nrename-block";
    hint.dataset.for = "rename";
    hint.hidden = state.mode !== "rename";
    hint.innerHTML = `<h2 data-t="neditTitle"></h2><p class="hint" data-t="nrenameHint"></p>`;
    hint.querySelectorAll("[data-t]").forEach(element => { element.textContent = t(element.dataset.t); });
    anchor.after(hint);
  }
  const body = $("files").tBodies[0];
  const lines = new Set();
  new MutationObserver(records => {
    lines.clear();
    for (const record of records) {
      const places = [record.target, ...record.addedNodes];
      for (const place of places) {
        const element = place.nodeType === 1 ? place : place.parentElement;
        const tr = element && (element.matches("tr") ? element : element.closest("tr"));
        if (tr && tr.parentElement === body) lines.add(tr);
      }
    }
    lines.forEach(ensureRowControls);
  }).observe(body, { childList: true, subtree: true });
  // changing section changes what a line shows: the lines are looked at again
  document.addEventListener("click", event => {
    if (event.target.closest && event.target.closest("nav")) setTimeout(() => [...body.rows].forEach(ensureRowControls), 80);
  }, true);
}

// The template NXPress began with ended with the contents, ({contents}); that is not the default any
// more. One saved that is still exactly that is taken as the default, which is the one now.
const OLD_PACK_TEMPLATE = "{name} [{id}][v{version}] ({contents})";
const migratedPackTemplate = text => (text === OLD_PACK_TEMPLATE ? "" : text);

// ---- Combine: the name of the NUP by the template, and the images of the Nintendo eShop ----
// The name of the NUP a dump folder will be is told under it. It is made by the template of the name
// of each SuperNSP, the one written in the options, and by the same one when the NUP is made.
const nupNames = new Map();   // folder, template -> the name
async function showNupName(row) {
  const template = (state.packTemplate || "").trim();
  const key = row.item.path + "\0" + template;
  let name = nupNames.get(key);
  if (name === undefined) {
    try { name = await app().NupNameFor(row.item.path, template); } catch (error) { name = ""; }
    nupNames.set(key, name);
  }
  if (!name) return;
  if (row.game) row.game.nupName = name;
  const written = row.tr.querySelector(".dump-plan b");
  if (written && written.textContent !== name) written.textContent = name;
}
function refreshNupNames() {
  for (const row of state.rows.values()) if (row.item.dump && row.tr.querySelector(".dump-plan")) showNupName(row);
}

// the search of the eShop, in a window like the one of the card of a NUP: the image chosen goes to the
// cut of the cover of the folder
// the "Only Switch 1" and "Only Switch 2" boxes of an eShop search: each on its own, both for the
// games of either, and none ticked searches every game
const systemOf = prefix => {
  const one = $(prefix + "-switch1").checked, two = $(prefix + "-switch2").checked;
  return one && two ? "both" : one ? "switch" : two ? "switch2" : "";
};
function pairSystems(prefix, search) {
  $(prefix + "-switch1").onchange = search;
  $(prefix + "-switch2").onchange = search;
}
let shopSearch = null;   // { row, round, chosen }
function shopDialog() {
  let dialog = $("nshop-dialog");
  if (dialog) return dialog;
  dialog = document.createElement("dialog");
  dialog.id = "nshop-dialog";
  dialog.className = "cover-dialog";
  dialog.setAttribute("aria-labelledby", "nshop-title");
  dialog.innerHTML = `<h2 id="nshop-title"></h2>
    <div class="cover-bar"><input id="nshop-query" type="search" maxlength="200" spellcheck="false"><button type="button" id="nshop-search" class="primary small"></button></div>
    <p class="cover-from"><span id="nshop-from"></span> · <label><input id="nshop-switch1" type="checkbox" checked> <span id="nshop-only1"></span></label> · <label><input id="nshop-switch2" type="checkbox" checked> <span id="nshop-only"></span></label></p>
    <p id="nshop-status" class="cover-status" role="status" aria-live="polite"></p>
    <div id="nshop-grid" class="cover-grid"></div>
    <div class="dialog-actions"><button type="button" id="nshop-cancel"></button><button type="button" id="nshop-use" class="primary" disabled></button></div>`;
  document.body.append(dialog);
  $("nshop-cancel").onclick = () => dialog.close();
  $("nshop-search").onclick = searchShop;
  pairSystems("nshop", searchShop);
  $("nshop-query").addEventListener("keydown", event => { if (event.key === "Enter") { event.preventDefault(); searchShop(); } });
  $("nshop-use").onclick = useShop;
  return dialog;
}
async function searchShop() {
  const search = shopSearch;
  if (!search) return;
  const round = ++search.round;
  const query = $("nshop-query").value.trim();
  search.chosen = null;
  $("nshop-use").disabled = true;
  $("nshop-grid").replaceChildren();
  $("nshop-status").classList.remove("warning");
  if (!query) { $("nshop-status").textContent = t("coverWriteName"); return; }
  $("nshop-status").textContent = t("coverSearching");
  $("nshop-search").disabled = true;
  try {
    const found = await app().SearchCovers(query, systemOf("nshop"));
    if (round !== search.round) return;
    $("nshop-status").textContent = found.length ? "" : t("coverNone");
    for (const game of found) {
      const tile = document.createElement("button");
      tile.type = "button";
      tile.className = "cover-tile";
      tile.innerHTML = `<img alt=""><span class="cover-name"></span><span class="cover-system"></span>`;
      tile.querySelector("img").src = game.image;
      tile.querySelector(".cover-name").textContent = game.title;
      tile.title = game.title;
      tile.querySelector(".cover-system").textContent = (game.systems || []).join(" · ");
      tile.setAttribute("aria-pressed", "false");
      tile.onclick = () => {
        for (const other of $("nshop-grid").children) other.setAttribute("aria-pressed", "false");
        tile.setAttribute("aria-pressed", "true");
        search.chosen = game;
        $("nshop-use").disabled = false;
      };
      tile.ondblclick = () => { search.chosen = game; useShop(); };
      $("nshop-grid").append(tile);
    }
  } catch (error) {
    if (round !== search.round) return;
    $("nshop-status").textContent = String((error && error.message) || error).replace(/^Error: /, "");
    $("nshop-status").classList.add("warning");
  } finally {
    if (round === search.round) $("nshop-search").disabled = false;
  }
}
async function useShop() {
  const search = shopSearch;
  if (!search || !search.chosen) return;
  const { row, chosen, apply } = search;
  $("nshop-dialog").close();
  // the image goes to the cut, as a picture chosen from the computer does, and from there to the
  // cover of the folder or to whoever asked for the search (the editor of a NUP in Rename)
  const why = await openCover(row, chosen.image, undefined, apply);
  if (why) showToast(why);
}
function shopCover(row, apply) {
  if (state.busy) return;
  const dialog = shopDialog();
  const name = (row.game && row.game.name) || shownName(row);
  shopSearch = { row, round: 0, chosen: null, apply };
  $("nshop-title").textContent = t("coverTitle")(name);
  $("nshop-query").value = name;
  $("nshop-query").setAttribute("aria-label", t("coverQuery"));
  $("nshop-search").textContent = t("coverSearch");
  $("nshop-from").textContent = t("coverFrom");
  $("nshop-only1").textContent = t("coverOnlySwitch1");
  $("nshop-only").textContent = t("coverOnlySwitch2");
  $("nshop-cancel").textContent = t("cancel");
  $("nshop-use").textContent = t("coverUse");
  $("nshop-use").disabled = true;
  $("nshop-grid").replaceChildren();
  $("nshop-status").textContent = "";
  openDialog(dialog);
  searchShop();
}

// ---- Combine: the images of SteamGridDB ----
// Search a game of steamgriddb.com by its name, as its site does, then take an icon, a grid, a hero or
// a logo of it, the icons first; the one chosen goes to the cut of the cover. SteamGridDB asks for a
// key (free, in the profile on its site) which this window keeps with its settings.
const SGDB_KINDS = [["icons", "Icons"], ["grids", "Grids"], ["heroes", "Heroes"], ["logos", "Logos"]];
let sgdb = null;   // { row, key, games, game, kind, page, round, chosen }

function sgdbDialog() {
  let dialog = $("nsgdb-dialog");
  if (dialog) return dialog;
  dialog = document.createElement("dialog");
  dialog.id = "nsgdb-dialog";
  dialog.className = "cover-dialog nsgdb-dialog";
  dialog.setAttribute("aria-labelledby", "nsgdb-title");
  dialog.innerHTML = `<h2 id="nsgdb-title"></h2>
    <div id="nsgdb-keybox" class="nsgdb-key" hidden>
      <p id="nsgdb-keytext" class="muted"></p>
      <div class="cover-bar"><input id="nsgdb-key" type="password" autocomplete="off" spellcheck="false"><button type="button" id="nsgdb-keysave" class="primary small"></button></div>
      <button type="button" id="nsgdb-keyget" class="ncover-set"></button>
      <p id="nsgdb-nokey" class="muted"></p>
    </div>
    <div id="nsgdb-main">
      <div class="cover-bar"><input id="nsgdb-query" type="search" maxlength="200" spellcheck="false"><button type="button" id="nsgdb-search" class="primary small"></button></div>
      <div class="nsgdb-game"><select id="nsgdb-game"></select><button type="button" id="nsgdb-keychange" class="quiet small"></button><button type="button" id="nsgdb-web2" class="ncover-set"></button></div>
      <div id="nsgdb-tabs" class="nsgdb-tabs" role="tablist"></div>
      <p id="nsgdb-status" class="cover-status" role="status" aria-live="polite"></p>
      <div id="nsgdb-grid" class="cover-grid"></div>
      <button type="button" id="nsgdb-more" class="quiet" hidden></button>
    </div>
    <div class="dialog-actions"><button type="button" id="nsgdb-cancel"></button><button type="button" id="nsgdb-use" class="primary" disabled></button></div>`;
  document.body.append(dialog);
  for (const [kind, label] of SGDB_KINDS) {
    const tab = document.createElement("button");
    tab.type = "button";
    tab.className = "nsgdb-tab";
    tab.dataset.kind = kind;
    tab.setAttribute("role", "tab");
    tab.textContent = label;
    tab.onclick = () => { if (sgdb && sgdb.kind !== kind) { sgdb.kind = kind; sgdbTabs(); sgdbImages(true); } };
    $("nsgdb-tabs").append(tab);
  }
  $("nsgdb-cancel").onclick = () => dialog.close();
  $("nsgdb-search").onclick = sgdbSearch;
  $("nsgdb-query").addEventListener("keydown", event => { if (event.key === "Enter") { event.preventDefault(); sgdbSearch(); } });
  $("nsgdb-game").onchange = () => { if (sgdb) { sgdb.game = +$("nsgdb-game").value; sgdbImages(true); } };
  $("nsgdb-more").onclick = () => sgdbImages(false);
  $("nsgdb-use").onclick = useSgdb;
  $("nsgdb-keyget").onclick = () => app().OpenSteamGridKeyPage();
  // with no key: the site is searched in the browser, and the image copied there is pasted here
  $("nsgdb-web2").onclick = () => app().OpenSteamGridSite($("nsgdb-query").value.trim());
  // on the page, while the window is open: a paste goes to what has the focus, and may be nothing
  document.addEventListener("paste", async event => {
    if (!dialog.open) return;
    const search = sgdb;
    const item = search && [...((event.clipboardData && event.clipboardData.items) || [])].find(entry => entry.type.startsWith("image/"));
    const file = item && item.getAsFile();
    if (!file) return;
    event.preventDefault();
    let picture;
    try {
      picture = await new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(reader.result);
        reader.onerror = () => reject(reader.error);
        reader.readAsDataURL(file);
      });
    } catch (error) {
      $("nsgdb-status").textContent = t("ncoverUnreadable");
      return;
    }
    dialog.close();
    // the image pasted goes to the cut, as any picture does, and from it to where the search was asked for
    const why = await openCover(search.row, picture, undefined, search.apply);
    if (why) showToast(why);
  });
  $("nsgdb-keychange").onclick = () => sgdbKeyBox("");
  $("nsgdb-keysave").onclick = saveSgdbKey;
  $("nsgdb-key").addEventListener("keydown", event => { if (event.key === "Enter") { event.preventDefault(); saveSgdbKey(); } });
  return dialog;
}
function sgdbTabs() {
  for (const tab of $("nsgdb-tabs").children) tab.setAttribute("aria-selected", String(tab.dataset.kind === (sgdb && sgdb.kind)));
}
// the window of the key, with what is wrong with the one there was, if anything
function sgdbKeyBox(problem) {
  $("nsgdb-keybox").hidden = false;
  $("nsgdb-main").hidden = true;
  $("nsgdb-keytext").textContent = (problem ? problem + " " : "") + t("nsgdbKeyText");
  $("nsgdb-key").value = sgdb ? sgdb.key : "";
  $("nsgdb-key").focus();
}
function saveSgdbKey() {
  const key = $("nsgdb-key").value.trim();
  if (!sgdb || !key) return;
  sgdb.key = key;
  save("steamGridKey", key);
  $("nsgdb-keybox").hidden = true;
  $("nsgdb-main").hidden = false;
  sgdbSearch();
}
// what went wrong: a key that was not taken opens the window of the key, the rest is told in the status
function sgdbFail(error) {
  const text = String((error && error.message) || error).replace(/^Error: /, "");
  if (/API key/.test(text)) { sgdbKeyBox(t("nsgdbKeyBad")); return; }
  $("nsgdb-status").textContent = text;
  $("nsgdb-status").classList.add("warning");
}
async function sgdbSearch() {
  const search = sgdb;
  if (!search) return;
  const query = $("nsgdb-query").value.trim();
  const round = ++search.round;
  search.chosen = null;
  $("nsgdb-use").disabled = true;
  $("nsgdb-grid").replaceChildren();
  $("nsgdb-more").hidden = true;
  $("nsgdb-status").classList.remove("warning");
  if (!search.key) { sgdbKeyBox(""); return; }
  if (!query) { $("nsgdb-status").textContent = t("coverWriteName"); return; }
  $("nsgdb-status").textContent = t("coverSearching");
  $("nsgdb-search").disabled = true;
  try {
    const games = await app().SearchSteamGrid(search.key, query);
    if (round !== search.round) return;
    search.games = games;
    $("nsgdb-game").replaceChildren(...games.map(game => {
      const option = document.createElement("option");
      option.value = String(game.id);
      option.textContent = game.name + (game.verified ? " ✓" : "");
      return option;
    }));
    if (!games.length) { $("nsgdb-status").textContent = t("coverNone"); return; }
    search.game = games[0].id;
    await sgdbImages(true);
  } catch (error) {
    if (round === search.round) sgdbFail(error);
  } finally {
    // the images asked for after the games count a round more: the button is given back all the same
    if (sgdb === search) $("nsgdb-search").disabled = false;
  }
}
// the images of the kind chosen, of the game chosen: the first page, or the next one added to it
async function sgdbImages(reset) {
  const search = sgdb;
  if (!search || !search.game) return;
  const round = ++search.round;
  if (reset) {
    search.page = 0;
    search.chosen = null;
    $("nsgdb-use").disabled = true;
    $("nsgdb-grid").replaceChildren();
  } else {
    search.page++;
  }
  $("nsgdb-more").hidden = true;
  $("nsgdb-status").classList.remove("warning");
  $("nsgdb-status").textContent = t("coverSearching");
  const grid = $("nsgdb-grid");
  grid.className = "cover-grid nsgdb-grid " + (search.kind === "heroes" ? "wide" : search.kind === "icons" ? "" : "fit");
  try {
    const images = await app().SteamGridImages(search.key, search.game, search.kind, search.page);
    if (round !== search.round) return;
    $("nsgdb-status").textContent = !images.length && reset ? t("coverNone") : "";
    for (const image of images) {
      const tile = document.createElement("button");
      tile.type = "button";
      tile.className = "cover-tile";
      tile.innerHTML = `<img alt=""><span class="cover-name"></span><span class="cover-system"></span>`;
      tile.querySelector("img").src = image.thumb;
      tile.querySelector(".cover-name").textContent = image.style || "";
      tile.querySelector(".cover-system").textContent = image.width && image.height ? `${image.width} × ${image.height}` : "";
      tile.setAttribute("aria-pressed", "false");
      tile.onclick = () => {
        for (const other of grid.children) other.setAttribute("aria-pressed", "false");
        tile.setAttribute("aria-pressed", "true");
        search.chosen = image;
        $("nsgdb-use").disabled = false;
      };
      tile.ondblclick = () => { search.chosen = image; useSgdb(); };
      grid.append(tile);
    }
    // a page holds up to 50: when it is full there may be more
    $("nsgdb-more").hidden = images.length < 40;
  } catch (error) {
    if (round === search.round) sgdbFail(error);
  }
}
async function useSgdb() {
  const search = sgdb;
  if (!search || !search.chosen) return;
  const { row, chosen, apply } = search;
  $("nsgdb-use").disabled = true;
  $("nsgdb-status").classList.remove("warning");
  $("nsgdb-status").textContent = t("coverSearching");
  let picture;
  try {
    picture = await app().SteamGridPicture(chosen.address);
  } catch (error) {
    $("nsgdb-use").disabled = false;
    sgdbFail(error);
    return;
  }
  $("nsgdb-dialog").close();
  // the image goes to the cut, as a picture chosen from the computer does
  const why = await openCover(row, picture, undefined, apply);
  if (why) showToast(why);
}
// the instructions for no key, with the words that open the site as a button in the sentence: the text
// of each language marks them with [[ and ]]
function sgdbNoKeyText(box) {
  const text = t("nsgdbNoKey");
  const open = text.indexOf("[["), close = text.indexOf("]]");
  box.replaceChildren();
  if (open < 0 || close < open) { box.textContent = text.replace(/\[\[|\]\]/g, ""); return; }
  const button = document.createElement("button");
  button.type = "button";
  button.className = "ncover-set ncover-inline";
  button.textContent = text.slice(open + 2, close);
  button.onclick = () => app().OpenSteamGridSite($("nsgdb-query").value.trim());
  box.append(text.slice(0, open), button, text.slice(close + 2));
}
function sgdbCover(row, apply) {
  if (state.busy) return;
  const dialog = sgdbDialog();
  const name = (row.game && row.game.name) || shownName(row);
  sgdb = { row, key: load("steamGridKey", ""), games: [], game: null, kind: "icons", page: 0, round: 0, chosen: null, apply };
  $("nsgdb-title").textContent = t("coverTitle")(name) + " · SteamGridDB";
  $("nsgdb-query").value = name;
  $("nsgdb-query").setAttribute("aria-label", t("coverQuery"));
  $("nsgdb-search").textContent = t("coverSearch");
  $("nsgdb-game").setAttribute("aria-label", t("nsgdbGame"));
  $("nsgdb-game").replaceChildren();
  $("nsgdb-keychange").textContent = t("nsgdbKeyChange");
  sgdbNoKeyText($("nsgdb-nokey"));
  $("nsgdb-web2").textContent = t("nsgdbWeb");
  $("nsgdb-keysave").textContent = t("nsgdbKeySave");
  $("nsgdb-keyget").textContent = t("nsgdbKeyGet");
  $("nsgdb-more").textContent = t("nsgdbMore");
  $("nsgdb-cancel").textContent = t("cancel");
  $("nsgdb-use").textContent = t("coverUse");
  $("nsgdb-use").disabled = true;
  $("nsgdb-grid").replaceChildren();
  $("nsgdb-status").textContent = "";
  $("nsgdb-keybox").hidden = true;
  $("nsgdb-main").hidden = false;
  sgdbTabs();
  openDialog(dialog);
  if (!sgdb.key) sgdbKeyBox(""); else sgdbSearch();
}

// ---- the Emulator, under the Log in the menu ----
// A page of its own, as the Log is, in the color of the group of the Log (Check). Its icon is a Nintendo
// Switch: the console, its two Joy-Con apart from the screen by a line each, a thumbstick and a button.
icons.emulator = '<rect x="1.5" y="5.5" width="21" height="13" rx="3"/><path d="M6 5.5v13M18 5.5v13"/><path d="M3.75 10h.01M20.25 14.5h.01"/>';
// the two consoles, after their titles on the page, drawn from the pictures of them: the screen is white
// in both, with the number of the console in it. The first has its Joy-Con in neon blue on the left and
// neon red on the right (the values of the Joy-Con themselves), a dark body between them, a thumbstick
// on the left and a button on the right. The second has the larger screen and Joy-Con 2 that are dark,
// with a ring of blue around the thumbstick on the left and a ring of red around the one on the right,
// and a thin line of each along the side of the Joy-Con next to the screen.
const sw = x => +x.toFixed(2);   // the numbers of the paths, to two decimals
const switchInk = "#14171c", switchOutline = '<rect x="1" y="5" width="22" height="14" rx="3.2" fill="none" stroke="#8a9099" stroke-opacity=".45" stroke-width=".7"/>';
const switchNumber = d => `<path d="${d}" fill="none" stroke="${switchInk}" stroke-width="1.2"/>`;
const switch1Icon = (left, right, number) =>
  `<path d="M${left} 5H4.2A3.2 3.2 0 0 0 1 8.2v7.6A3.2 3.2 0 0 0 4.2 19h${sw(left - 4.2)}z" fill="#0ab9e6" stroke="none"/>` +
  `<path d="M${right} 5h${sw(19.8 - right)}A3.2 3.2 0 0 1 23 8.2v7.6A3.2 3.2 0 0 1 19.8 19h-${sw(19.8 - right)}z" fill="#ff3c28" stroke="none"/>` +
  `<rect x="${left}" y="5" width="${sw(right - left)}" height="14" fill="#2f3338" stroke="none"/>` +
  `<rect x="${sw(left + .55)}" y="6.4" width="${sw(right - left - 1.1)}" height="11.2" rx=".6" fill="#fff" stroke="none"/>` +
  `<circle cx="${sw((1 + left) / 2)}" cy="9.6" r="1.25" fill="${switchInk}" stroke="none"/>` +
  `<circle cx="${sw((23 + right) / 2)}" cy="14.6" r="1.25" fill="${switchInk}" stroke="none"/>` +
  switchOutline + switchNumber(number);
const switch2Icon = (left, right, number) => {
  const blue = "#4cc3f0", red = "#f2644f";
  const stick = (cx, cy, color) => `<circle cx="${cx}" cy="${cy}" r="1" fill="#14161a" stroke="none"/><circle cx="${cx}" cy="${cy}" r="1.55" fill="none" stroke="${color}" stroke-width=".75"/>`;
  return `<rect x="1" y="5" width="22" height="14" rx="3.2" fill="#373a3d" stroke="none"/>` +
    `<rect x="${left}" y="5" width="${sw(right - left)}" height="14" fill="#0e0f11" stroke="none"/>` +
    `<rect x="${sw(left + .5)}" y="6.3" width="${sw(right - left - 1)}" height="11.4" rx=".5" fill="#fff" stroke="none"/>` +
    `<path d="M${left} 6.2v11.6" fill="none" stroke="${blue}" stroke-width=".8"/><path d="M${right} 6.2v11.6" fill="none" stroke="${red}" stroke-width=".8"/>` +
    stick(sw((1 + left) / 2), 9.6, blue) + stick(sw((23 + right) / 2), 14.6, red) +
    switchOutline + switchNumber(number);
};
// Recompilations, in color like the two consoles: a monitor with a black screen of green code falling, as in the
// film The Matrix (columns of characters with a bright head and a trail that fades), in both themes
icons.recompile = '<g stroke="none"><path d="M9.4 16 8.8 20.2h6.4L14.6 16z" fill="#0d8fb3"/><rect x="6.6" y="19.8" width="10.8" height="1.9" rx=".9" fill="#0b87ab"/><rect x="1.8" y="2.8" width="20.4" height="14" rx="2.4" fill="#00a8d6"/><rect x="3.7" y="4.7" width="16.6" height="10.2" rx="1.2" fill="#050b07"/><rect x="4.85" y="12.80" width="0.7" height=".85" rx=".2" fill="#e0ffe0" opacity="1"/><rect x="4.75" y="11.75" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.8"/><rect x="4.75" y="10.70" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.55"/><rect x="4.75" y="8.60" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.28"/><rect x="4.75" y="7.55" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.2"/><rect x="6.65" y="9.65" width="0.9" height=".85" rx=".2" fill="#e0ffe0" opacity="1"/><rect x="6.75" y="8.60" width="0.7" height=".85" rx=".2" fill="#2dff5a" opacity="0.8"/><rect x="6.65" y="7.55" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.55"/><rect x="6.65" y="6.50" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.4"/><rect x="6.65" y="5.45" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.28"/><rect x="6.65" y="11.75" width=".9" height=".85" rx=".2" fill="#2dff5a" opacity="0.45"/><rect x="6.65" y="12.80" width=".9" height=".85" rx=".2" fill="#2dff5a" opacity="0.3"/><rect x="6.65" y="13.85" width=".9" height=".85" rx=".2" fill="#2dff5a" opacity="0.18"/><rect x="8.55" y="13.85" width="0.9" height=".85" rx=".2" fill="#e0ffe0" opacity="1"/><rect x="8.55" y="12.80" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.8"/><rect x="8.55" y="11.75" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.55"/><rect x="8.55" y="9.65" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.28"/><rect x="8.55" y="8.60" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.2"/><rect x="10.45" y="7.55" width="0.9" height=".85" rx=".2" fill="#e0ffe0" opacity="1"/><rect x="10.45" y="6.50" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.8"/><rect x="10.45" y="5.45" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.55"/><rect x="10.45" y="9.65" width=".9" height=".85" rx=".2" fill="#2dff5a" opacity="0.45"/><rect x="10.45" y="10.70" width=".9" height=".85" rx=".2" fill="#2dff5a" opacity="0.3"/><rect x="10.45" y="11.75" width=".9" height=".85" rx=".2" fill="#2dff5a" opacity="0.18"/><rect x="12.35" y="11.75" width="0.9" height=".85" rx=".2" fill="#e0ffe0" opacity="1"/><rect x="12.35" y="10.70" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.8"/><rect x="12.35" y="9.65" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.55"/><rect x="12.35" y="7.55" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.28"/><rect x="12.35" y="6.50" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.2"/><rect x="12.35" y="13.85" width=".9" height=".85" rx=".2" fill="#2dff5a" opacity="0.45"/><rect x="14.25" y="8.60" width="0.9" height=".85" rx=".2" fill="#e0ffe0" opacity="1"/><rect x="14.25" y="7.55" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.8"/><rect x="14.25" y="6.50" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.55"/><rect x="14.35" y="5.45" width="0.7" height=".85" rx=".2" fill="#2dff5a" opacity="0.4"/><rect x="14.25" y="10.70" width=".9" height=".85" rx=".2" fill="#2dff5a" opacity="0.45"/><rect x="14.25" y="11.75" width=".9" height=".85" rx=".2" fill="#2dff5a" opacity="0.3"/><rect x="14.25" y="12.80" width=".9" height=".85" rx=".2" fill="#2dff5a" opacity="0.18"/><rect x="16.15" y="13.85" width="0.9" height=".85" rx=".2" fill="#e0ffe0" opacity="1"/><rect x="16.15" y="12.80" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.8"/><rect x="16.15" y="11.75" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.55"/><rect x="16.25" y="9.65" width="0.7" height=".85" rx=".2" fill="#2dff5a" opacity="0.28"/><rect x="16.15" y="8.60" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.2"/><rect x="18.05" y="10.70" width="0.9" height=".85" rx=".2" fill="#e0ffe0" opacity="1"/><rect x="18.05" y="9.65" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.8"/><rect x="18.15" y="8.60" width="0.7" height=".85" rx=".2" fill="#2dff5a" opacity="0.55"/><rect x="18.05" y="7.55" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.4"/><rect x="18.05" y="6.50" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.28"/><rect x="18.05" y="5.45" width="0.9" height=".85" rx=".2" fill="#2dff5a" opacity="0.2"/><rect x="18.05" y="12.80" width=".9" height=".85" rx=".2" fill="#2dff5a" opacity="0.45"/><rect x="18.05" y="13.85" width=".9" height=".85" rx=".2" fill="#2dff5a" opacity="0.3"/></g>';
icons.switch1 = switch1Icon(7.2, 16.8, "M10.9 10.6l1.5-1.2v5.2");
icons.switch2 = switch2Icon(5.2, 18.8, "M10.2 10.5l.8-1 1.3-.2 1.1.6.2 1.1-1 1.2-2.4 2.4h3.7");

// the entry of the menu
function emulatorButton(group) {
  const button = document.createElement("button");
  button.innerHTML = tile("emulator", group) + "<span><b></b><small></small></span>";
  button.dataset.mode = "emulator";
  button.querySelector("b").textContent = t("nEmulator");
  button.title = t("nEmulator");
  button.querySelector("small").textContent = t("nHintEmulator");
  button.className = state.view === "emulator" ? "on" : "";
  button.onclick = openEmulator;
  return button;
}
// the page is opened as the log is: not while a work runs
function openEmulator() {
  if (state.busy || state.starting || state.view === "emulator") return;
  state.view = "emulator";
  render();
}
// paintEmulator makes the page the first time it is drawn, and shows it or hides it
function paintEmulator(on) {
  let page = $("emulator");
  if (!page) {
    page = document.createElement("section");
    page.id = "emulator";
    page.hidden = true;
    page.innerHTML = `<div class="page-head"><h1 class="title-row"><span class="tile tone-check" aria-hidden="true"></span><span id="emulator-title"></span></h1><p id="emulator-about" class="muted"></p></div><div id="emulator-tools" class="emu-tools"></div><div id="emulator-list" class="emu-list"></div>`;
    page.querySelector(".tile").innerHTML = icon("emulator");
    $("preferences").after(page);
  }
  page.hidden = !on;
  if (!on) { emuEditing = false; emuForm = null; return; }
  $("emulator-title").textContent = t("nEmulator");
  $("emulator-about").textContent = t("nAboutEmulator");
  if (emulatorList) paintEmulatorList(); else loadEmulators();
}

// the emulators, by section, as NXPress lists them (EmulatorList), asked for once
let emulatorList = null;
async function loadEmulators() {
  try { emulatorList = await app().EmulatorList(); } catch (error) { emulatorList = []; }
  if (state.view === "emulator") paintEmulatorList();
}

// The list the person sees is a layout: sections, each with categories (Yuzu-Based) and emulators, an
// emulator being a name, a page and a note. Until the person changes something it is the list NXPress
// comes with, drawn from EmulatorList each time; the first change keeps the whole layout ("emulatorLayout"),
// and Reset to default forgets it. The texts NXPress wrote (nameKey, noteKey) are translated, as they were,
// until the person writes their own.
let emuEditing = false;   // the buttons to change the list are shown
let emuForm = null;       // the form open: { kind: "section" | "category" | "emulator", id, parent }; id is the one it changes, parent the one it adds to
let emuSeq = 0;
const emuId = () => "n" + Date.now().toString(36) + (emuSeq++);
const isEmuLayout = value => Array.isArray(value) && value.every(section => isObject(section) && typeof section.id === "string" && Array.isArray(section.items)
  && section.items.every(item => isObject(item) && typeof item.id === "string" && (!item.items || (Array.isArray(item.items) && item.items.every(inner => isObject(inner) && typeof inner.id === "string")))));
// the list NXPress comes with, as a layout: the sections only, with nothing inside them; the person
// adds or imports their own emulators. The ids are the places, so the same list has the same ids
function emuSeed() {
  return (emulatorList || []).map((group, index) => ({ id: "s" + index, title: group.title, icon: group.icon || "", items: [] }));
}
const emuView = () => load("emulatorLayout", null, isEmuLayout) || emuSeed();
// emuChange changes the layout, keeps it and draws the page again
function emuChange(change) {
  const layout = emuView();
  change(layout);
  save("emulatorLayout", layout);
  emuForm = null;
  paintEmulatorList();
}
// where an item is: the list that holds it, and its place in it
function emuFind(layout, id) {
  const lists = [layout];
  for (const section of layout) { lists.push(section.items); for (const item of section.items) if (item.items) lists.push(item.items); }
  for (const list of lists) { const index = list.findIndex(item => item.id === id); if (index >= 0) return { list, index, item: list[index] }; }
  return null;
}
// the text of a field: the one the person wrote, else the one NXPress translates
const emuText = (item, field) => (item[field + "Key"] ? t(item[field + "Key"]) : item[field]) || "";
function emuSet(item, field, value) {
  if (value === emuText(item, field)) return;
  item[field] = value;
  delete item[field + "Key"];
}
// a page: with its scheme, http or https, and a host; "" when it is not one
function emuLink(text) {
  let link = text.trim();
  if (!link) return "";
  if (!/^[a-z][a-z0-9+.-]*:/i.test(link)) link = "https://" + link;
  try {
    const page = new URL(link);
    return (page.protocol === "https:" || page.protocol === "http:") && page.hostname && !/\s/.test(link) ? link : "";
  } catch { return ""; }
}

// the small buttons: 📝 changes, the green plus adds, ⛔ deletes
let emuSectionAt = "top";
let emuWatch = null;
const emuPlus = `<svg width="19" height="19" viewBox="0 0 22 22" aria-hidden="true"><circle cx="11" cy="11" r="10" fill="#34c759"/><path d="M11 6v10M6 11h10" stroke="#fff" stroke-width="2.4" stroke-linecap="round"/></svg>`;
function emuButton(kind, tip, action) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "emu-ib";
  if (kind === "add") button.innerHTML = emuPlus; else button.textContent = kind === "edit" ? "📝" : "⛔";
  button.title = tip;
  button.setAttribute("aria-label", tip);
  // inside the title of a section the click must not open or close it
  button.onclick = event => { event.preventDefault(); event.stopPropagation(); action(button); };
  return button;
}
// a delete that holds something asks twice: the button says "Sure?" for a moment
function emuDeleteButton(holds, remove) {
  return emuButton("delete", t("nEmuTipDelete"), button => {
    if (!holds || button.classList.contains("arm")) { remove(); return; }
    button.classList.add("arm");
    button.textContent = t("nEmuSure");
    setTimeout(() => { if (button.isConnected) { button.classList.remove("arm"); button.textContent = "⛔"; } }, 4000);
  });
}
function emuControls(...buttons) {
  const box = document.createElement("span");
  box.className = "emu-ctl";
  box.append(...buttons);
  return box;
}
// the form of a name, or of a name, a page and a note: Enter saves, Escape leaves it
function emuFormBox(fields, save) {
  const form = document.createElement("div");
  form.className = "emu-form";
  const inputs = fields.map(field => {
    const label = document.createElement("label");
    label.textContent = field.label;
    const input = document.createElement("input");
    input.type = "text";
    input.value = field.value;
    input.spellcheck = false;
    form.append(label, input);
    return input;
  });
  const done = () => save(inputs.map(input => input.value));
  const cancel = () => { emuForm = null; paintEmulatorList(); };
  form.addEventListener("keydown", event => {
    if (event.key === "Enter") { event.preventDefault(); done(); }
    if (event.key === "Escape") { event.preventDefault(); cancel(); }
  });
  const actions = document.createElement("div");
  actions.className = "emu-form-actions";
  const no = document.createElement("button");
  no.type = "button"; no.className = "ncover-set"; no.textContent = t("cancel"); no.onclick = cancel;
  const yes = document.createElement("button");
  yes.type = "button"; yes.className = "ncover-set emu-save"; yes.textContent = t("neditSave"); yes.onclick = done;
  actions.append(no, yes);
  form.append(actions);
  return form;
}
const emuNameOf = value => {
  const name = value.trim();
  if (!name) showToast(t("nEmuNeedName"));
  return name;
};
// the form of a section or of a category: its name
function emuNameForm(kind, item, parent) {
  return emuFormBox([{ label: t("nEmuName"), value: item ? emuText(item, kind === "section" ? "title" : "name") : "" }], ([value]) => {
    const name = emuNameOf(value);
    if (!name) return;
    emuChange(layout => {
      if (item) { emuSet(emuFind(layout, item.id).item, kind === "section" ? "title" : "name", name); return; }
      if (kind === "section") layout.push({ id: emuId(), title: name, icon: "", items: [] });
      else emuFind(layout, parent).item.items.push({ id: emuId(), name, items: [] });
    });
  });
}
// the form of an emulator: its name, its page and its note
function emuEmulatorForm(item, parent) {
  const fields = [
    { label: t("nEmuName"), value: item ? emuText(item, "name") : "" },
    { label: t("nEmuLink"), value: item ? item.address : "https://" },
    { label: t("nEmuNote"), value: item ? emuText(item, "note") : "" },
  ];
  return emuFormBox(fields, ([value, address, note]) => {
    const name = emuNameOf(value);
    if (!name) return;
    const link = emuLink(address);
    if (!link) { showToast(t("nEmuBadLink")); return; }
    emuChange(layout => {
      if (item) {
        const found = emuFind(layout, item.id).item;
        emuSet(found, "name", name);
        found.address = link;
        emuSet(found, "note", note.trim());
        return;
      }
      const added = { id: emuId(), name, address: link };
      if (note.trim()) added.note = note.trim();
      emuFind(layout, parent).item.items.push(added);
    });
  });
}
const emuFormFor = (kind, id, parent) => emuEditing && emuForm && emuForm.kind === kind && (id ? emuForm.id === id : emuForm.parent === parent && !emuForm.id);
const emuOpenForm = form => { emuForm = form; paintEmulatorList(); };

// an emulator: a card with its name, its page, its note and the button that opens the page; its buttons are
// outside the card, at its right
function emuRow(item) {
  const row = document.createElement("div");
  row.className = "emu-row";
  row.dataset.emu = item.id;
  const controls = emuEditing ? emuControls(
    emuButton("edit", t("nEmuTipEdit"), () => emuOpenForm({ kind: "emulator", id: item.id })),
    emuDeleteButton(false, () => emuChange(layout => { const at = emuFind(layout, item.id); at.list.splice(at.index, 1); })),
  ) : null;
  if (emuFormFor("emulator", item.id)) {
    row.append(emuEmulatorForm(item));
    if (controls) row.append(controls);
    return row;
  }
  const card = document.createElement("div");
  card.className = "emu-card";
  const text = document.createElement("div");
  text.className = "emu-text";
  const name = document.createElement("b");
  name.textContent = emuText(item, "name");
  const page = document.createElement("small");
  page.textContent = item.address.replace(/^https?:\/\//, "").replace(/\/$/, "");
  text.append(name, page);
  if (emuText(item, "note")) {
    const note = document.createElement("p");
    note.className = "emu-note";
    note.textContent = (item.plain ? "" : "⚠ ") + emuText(item, "note");
    text.append(note);
  }
  const open = document.createElement("button");
  open.type = "button";
  open.className = "ncover-set";
  open.textContent = t("nEmuOpen");
  open.onclick = () => app().OpenEmulatorPage(item.address).catch(error => showToast(String((error && error.message) || error)));
  card.append(text, open);
  if (emuEditing) row.append(emuGrip("emulator", item.id));
  row.append(card);
  if (controls) row.append(controls);
  return row;
}
// a category: its name, and under it its emulators, a little to the right; outside editing its name
// opens and closes it (an arrow after the name), and the ones closed are remembered
function emuCategory(category, list, sectionTitle) {
  if (!emuEditing && !category.items.length) return;
  const heading = document.createElement("h3");
  heading.className = "emu-family";
  heading.dataset.emu = category.id;
  const title = document.createElement("span");
  title.className = "emu-title";
  title.textContent = emuText(category, "name");
  if (emuEditing) heading.append(emuGrip("category", category.id));
  heading.append(title);
  const foldKey = sectionTitle + " / " + emuText(category, "name");
  const folded = !emuEditing && !!load("emulatorsCategoriesClosed", {})[foldKey];
  if (!emuEditing) {
    heading.classList.add("fold");
    const arrow = document.createElement("span");
    arrow.className = "emu-chev";
    heading.append(arrow);
    heading.tabIndex = 0;
    heading.setAttribute("role", "button");
  }
  if (emuEditing) heading.append(emuControls(
    emuButton("edit", t("nEmuTipEdit"), () => emuOpenForm({ kind: "category", id: category.id })),
    emuButton("add", t("nEmuTipAdd"), () => emuOpenForm({ kind: "emulator", parent: category.id })),
    emuDeleteButton(category.items.length > 0, () => emuChange(layout => { const at = emuFind(layout, category.id); at.list.splice(at.index, 1); })),
  ));
  list.append(heading);
  if (emuFormFor("category", category.id)) list.append(emuNameForm("category", category));
  const body = document.createElement("div");
  body.className = "emu-sub";
  body.dataset.body = category.id;
  for (const item of category.items) body.append(emuRow(item));
  if (emuFormFor("emulator", null, category.id)) body.append(emuEmulatorForm(null, category.id));
  list.append(body);
  if (!emuEditing) {
    const show = closed => {
      heading.classList.toggle("closed", closed);
      body.hidden = closed;
      heading.setAttribute("aria-expanded", String(!closed));
    };
    show(folded);
    const flip = () => {
      const now = load("emulatorsCategoriesClosed", {});
      const closed = !now[foldKey];
      if (closed) now[foldKey] = true; else delete now[foldKey];
      save("emulatorsCategoriesClosed", now);
      show(closed);
    };
    heading.onclick = flip;
    heading.onkeydown = event => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); flip(); } };
  }
}
// a section, which opens and closes by its title; all start closed, and the ones left open are remembered
function emuSection(section, opened) {
  const details = document.createElement("details");
  details.className = "emu-section";
  details.dataset.emu = section.id;
  const formInside = emuEditing && emuForm && (emuForm.parent === section.id || emuForm.id === section.id
    || section.items.some(item => emuForm.parent === item.id || emuForm.id === item.id || (item.items || []).some(inner => inner.id === emuForm.id)));
  const title = emuText(section, "title");
  details.open = !!opened[title] || !!formInside;
  details.addEventListener("toggle", () => {
    const now = load("emulatorsOpen", {});
    if (details.open) now[title] = true; else delete now[title];
    save("emulatorsOpen", now);
  });
  const heading = document.createElement("summary");
  heading.className = "emu-group";
  const label = document.createElement("span");
  label.className = "emu-title";
  label.textContent = title;
  if (emuEditing) heading.append(emuGrip("section", section.id));
  heading.append(label);
  if (icons[section.icon]) {
    const mark = document.createElement("span");
    mark.className = "emu-console";
    mark.innerHTML = icon(section.icon);
    // the monitor of Recompilations fits the line of its name; the consoles are a little taller
    if (section.icon === "recompile") mark.classList.add("fit");
    heading.append(mark);
  }
  if (emuEditing) heading.append(emuControls(
    emuButton("edit", t("nEmuTipEdit"), () => emuOpenForm({ kind: "section", id: section.id })),
    emuButton("add", t("nEmuTipAdd"), () => emuOpenForm({ kind: "category", parent: section.id })),
    emuDeleteButton(section.items.length > 0, () => emuChange(layout => { const at = emuFind(layout, section.id); at.list.splice(at.index, 1); })),
  ));
  const list = document.createElement("div");
  list.className = "emu-cards";
  if (emuFormFor("section", section.id)) list.append(emuNameForm("section", section));
  for (const item of section.items) {
    if (item.items) emuCategory(item, list, title); else list.append(emuRow(item));
  }
  if (emuFormFor("category", null, section.id)) list.append(emuNameForm("category", null, section.id));
  details.append(heading, list);
  return details;
}
// Edit at the top of the page; while editing, Done editing and Reset to default (asked twice)
function paintEmulatorTools() {
  const tools = $("emulator-tools");
  if (!tools) return;
  tools.replaceChildren();
  const make = (text, action, extra = "") => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "ncover-set " + extra;
    button.textContent = text;
    button.onclick = () => action(button);
    return button;
  };
  const toggle = () => { emuEditing = !emuEditing; emuForm = null; paintEmulatorList(); };
  if (!emuEditing) { tools.append(make("📝 " + t("nEmuEdit"), toggle)); return; }
  const reset = make(t("nEmuReset"), button => {
    if (!button.classList.contains("arm")) {
      button.classList.add("arm");
      button.textContent = t("nEmuSure");
      setTimeout(() => { if (button.isConnected) { button.classList.remove("arm"); button.textContent = t("nEmuReset"); } }, 4000);
      return;
    }
    try { localStorage.removeItem("nxpress.emulatorLayout"); } catch { }
    emuForm = null;
    paintEmulatorList();
  });
  tools.append(make(t("nEmuImport"), () => emuImportOpen()), emuShareMenu(make), reset, make(t("nEmuDone"), toggle, "emu-save"));
}
// the page: each section, with its categories and its emulators
function paintEmulatorList() {
  const box = $("emulator-list");
  if (!box) return;
  box.replaceChildren();
  box.classList.toggle("editing", emuEditing);
  paintEmulatorTools();
  const opened = load("emulatorsOpen", {});
  for (const section of emuView()) box.append(emuSection(section, opened));
  if (emuEditing) {
    // Add section above the first section; a second one after the last, shown only once the first is out of view
    const addButton = where => {
      const add = document.createElement("button");
      add.type = "button";
      add.className = "emu-addsection " + where;
      add.innerHTML = emuPlus + "<span></span>";
      add.lastChild.textContent = t("nEmuAddSection");
      add.onclick = () => { emuSectionAt = where; emuOpenForm({ kind: "section" }); };
      return add;
    };
    const formHere = where => emuFormFor("section", null, undefined) && emuSectionAt === where;
    const top = addButton("top"), bottom = addButton("bottom");
    box.prepend(top);
    if (formHere("top")) top.after(emuNameForm("section"));
    if (emuFormFor("section", null, undefined) && emuSectionAt !== "top") box.append(emuNameForm("section"));
    box.append(bottom);
    bottom.style.visibility = "hidden";
    if (emuWatch) emuWatch.disconnect();
    if (typeof IntersectionObserver === "function") {
      emuWatch = new IntersectionObserver(entries => {
        bottom.style.visibility = entries[entries.length - 1].isIntersecting ? "hidden" : "visible";
      });
      emuWatch.observe(top);
    }
  }
  const input = box.querySelector(".emu-form input");
  if (input) input.focus();
}

// ---- moving by dragging: the three dots at the left of a section, a category or an emulator pick it up.
// A section goes before or after another section; a category before or after another one of its own section
// (never into another section); an emulator into any category, or among the emulators of a section.
// It is done with the pointer (not the drag and drop of the page, which the window may take for files): the
// item stays faded in its place, a copy follows the pointer, and a blue line shows where it will land.
let emuDrag = null;
function emuGrip(kind, id) {
  const grip = document.createElement("span");
  grip.className = "emu-grip";
  grip.textContent = "⋮";
  grip.title = t("nEmuTipMove");
  grip.setAttribute("role", "button");
  grip.setAttribute("aria-label", t("nEmuTipMove"));
  grip.tabIndex = 0;
  grip.dataset.grip = id;
  // inside the title of a section the click must not open or close it
  grip.onclick = event => { event.preventDefault(); event.stopPropagation(); };
  grip.onpointerdown = event => emuDragStart(event, kind, id);
  // without a mouse: the arrows move it one place in its list
  grip.onkeydown = event => {
    if (event.key === " " || event.key === "Enter") { event.preventDefault(); event.stopPropagation(); return; }
    const step = event.key === "ArrowUp" ? -1 : event.key === "ArrowDown" ? 1 : 0;
    if (!step) return;
    event.preventDefault();
    event.stopPropagation();
    emuChange(layout => {
      const at = emuFind(layout, id);
      const to = at ? at.index + step : -1;
      if (!at || to < 0 || to >= at.list.length) return;
      at.list.splice(to, 0, at.list.splice(at.index, 1)[0]);
    });
    const again = document.querySelector(`#emulator-list [data-grip="${CSS.escape(id)}"]`);
    if (again) again.focus();
  };
  return grip;
}
function emuDragStart(event, kind, id) {
  if (event.button !== 0 || emuDrag) return;
  event.preventDefault();
  const drag = { kind, id, startX: event.clientX, startY: event.clientY, x: event.clientX, y: event.clientY, live: false, target: null, frame: 0 };
  emuDrag = drag;
  const end = commit => {
    window.removeEventListener("pointermove", move, true);
    window.removeEventListener("pointerup", up, true);
    window.removeEventListener("pointercancel", cancel, true);
    window.removeEventListener("keydown", key, true);
    cancelAnimationFrame(drag.frame);
    emuDragStop(drag);
    emuDrag = null;
    if (commit && drag.live && drag.target) emuDrop(drag);
  };
  const move = e => {
    drag.x = e.clientX;
    drag.y = e.clientY;
    if (!drag.live) {
      if (Math.hypot(drag.x - drag.startX, drag.y - drag.startY) < 4) return;
      if (!emuDragBegin(drag)) { end(false); return; }
    }
    emuDragUpdate(drag);
  };
  const up = () => end(true);
  const cancel = () => end(false);
  const key = e => { if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); end(false); } };
  window.addEventListener("pointermove", move, true);
  window.addEventListener("pointerup", up, true);
  window.addEventListener("pointercancel", cancel, true);
  window.addEventListener("keydown", key, true);
}
// the part of the page that scrolls the list: the nearest one that scrolls, else the page
function emuScroller(from) {
  for (let node = from.parentElement; node && node !== document.body; node = node.parentElement) {
    const overflow = getComputedStyle(node).overflowY;
    if ((overflow === "auto" || overflow === "scroll") && node.scrollHeight > node.clientHeight) return node;
  }
  return document.scrollingElement || document.documentElement;
}
function emuDragBegin(drag) {
  const box = $("emulator-list");
  const source = box && box.querySelector(`[data-emu="${CSS.escape(drag.id)}"]`);
  if (!source) return false;
  const layout = emuView();
  const at = emuFind(layout, drag.id);
  if (!at) return false;
  drag.source = source;
  drag.box = box;
  drag.section = drag.kind === "category" ? (layout.find(section => section.items.includes(at.item)) || {}).id : "";
  const shown = drag.kind === "section" ? source.querySelector("summary") : source;
  const rect = shown.getBoundingClientRect();
  drag.offX = drag.startX - rect.left;
  drag.offY = drag.startY - rect.top;
  const ghost = shown.cloneNode(true);
  ghost.removeAttribute("data-emu");
  ghost.classList.add("emu-ghost");
  ghost.style.width = rect.width + "px";
  document.body.append(ghost);
  drag.ghost = ghost;
  const line = document.createElement("div");
  line.className = "emu-dropline";
  line.hidden = true;
  line.append(document.createElement("span"));
  document.body.append(line);
  drag.line = line;
  box.classList.add("drag-" + drag.kind);
  source.classList.add("emu-dragging");
  document.body.classList.add("emu-moving");
  drag.scroller = emuScroller(box);
  drag.live = true;
  const tick = () => {
    if (emuDrag !== drag) return;
    const scroller = drag.scroller;
    const view = scroller === document.scrollingElement || scroller === document.documentElement ? { top: 0, bottom: window.innerHeight } : scroller.getBoundingClientRect();
    const zone = 70;
    let by = 0;
    if (drag.y < view.top + zone) by = -Math.ceil((view.top + zone - drag.y) / 5);
    else if (drag.y > view.bottom - zone) by = Math.ceil((drag.y - (view.bottom - zone)) / 5);
    if (by) { scroller.scrollTop += by; emuDragUpdate(drag); }
    drag.frame = requestAnimationFrame(tick);
  };
  drag.frame = requestAnimationFrame(tick);
  return true;
}
function emuDragStop(drag) {
  if (!drag.live) return;
  if (drag.ghost) drag.ghost.remove();
  if (drag.line) drag.line.remove();
  if (drag.box) drag.box.classList.remove("drag-" + drag.kind);
  if (drag.source) drag.source.classList.remove("emu-dragging");
  document.body.classList.remove("emu-moving");
}
function emuDragUpdate(drag) {
  drag.ghost.style.left = (drag.x - drag.offX) + "px";
  drag.ghost.style.top = (drag.y - drag.offY) + "px";
  drag.target = emuDropTarget(drag);
  const line = drag.line;
  if (!drag.target) { line.hidden = true; return; }
  const { rect, pos, label, indent } = drag.target;
  line.hidden = false;
  line.style.left = (rect.left + indent) + "px";
  line.style.width = Math.max(40, rect.width - indent) + "px";
  line.style.top = (pos === "before" ? rect.top - 4 : rect.bottom + 4) + "px";
  line.firstChild.textContent = label;
  line.firstChild.hidden = !label;
}
// the one of the candidates the pointer is on, else the nearest one, and whether it is in the upper or the lower half
function emuNearest(candidates, y) {
  let best = null;
  for (const element of candidates) {
    const rect = element.getBoundingClientRect();
    if (!rect.height) continue;
    const gap = y < rect.top ? rect.top - y : y > rect.bottom ? y - rect.bottom : 0;
    if (!best || gap < best.gap) best = { element, rect, gap };
  }
  return best;
}
function emuDropTarget(drag) {
  const box = drag.box;
  const half = best => (drag.y < (best.rect.top + best.rect.bottom) / 2 ? "before" : "after");
  if (drag.kind === "section") {
    const best = emuNearest([...box.children].filter(node => node.matches("details.emu-section") && node.dataset.emu !== drag.id), drag.y);
    return best && { ref: best.element.dataset.emu, pos: half(best), rect: best.rect, label: "", indent: 0 };
  }
  if (drag.kind === "category") {
    const cards = box.querySelector(`details[data-emu="${CSS.escape(drag.section || "")}"] > .emu-cards`);
    if (!cards) return null;
    const best = emuNearest([...cards.children].filter(node => (node.matches("h3.emu-family") || node.matches(".emu-row")) && node.dataset.emu !== drag.id), drag.y);
    return best && { ref: best.element.dataset.emu, pos: half(best), rect: best.rect, label: "", indent: 0 };
  }
  // an emulator: onto an emulator of any category or section that is open, or onto the name of a category
  const open = [...box.querySelectorAll("details.emu-section[open]")];
  const candidates = [];
  for (const section of open) {
    for (const node of section.querySelectorAll(".emu-row, h3.emu-family")) {
      if (node.dataset.emu === drag.id || node.querySelector(".emu-form")) continue;
      candidates.push(node);
    }
  }
  const best = emuNearest(candidates, drag.y);
  if (!best) return null;
  const node = best.element;
  const into = node.closest(".emu-sub");
  const nameOf = heading => (heading && heading.querySelector(".emu-title") ? heading.querySelector(".emu-title").textContent : "");
  const where = prefix => t("nEmuMoveHere") + (prefix ? ": " + prefix : "");
  if (node.matches("h3.emu-family")) {
    // the first place of the category, under its name
    return { ref: node.dataset.emu, pos: "start", rect: best.rect, label: where(nameOf(node)), indent: 16 };
  }
  const place = into ? nameOf(box.querySelector(`h3[data-emu="${CSS.escape(into.dataset.body)}"]`)) : (node.closest("details").querySelector("summary .emu-title") || {}).textContent;
  return { ref: node.dataset.emu, pos: half(best), rect: best.rect, label: where(place), indent: 0 };
}
// the drop: take it out of its place, put it in the new one
function emuDrop(drag) {
  const target = drag.target;
  if (!target || target.ref === drag.id) return;
  emuChange(layout => {
    const from = emuFind(layout, drag.id);
    if (!from) return;
    const ref = emuFind(layout, target.ref);
    if (!ref) return;
    // where it may go: a section among the sections; a category among the places of its own section; an emulator
    // in a category or among the emulators of a section
    if (drag.kind === "section" && ref.list !== layout) return;
    if (drag.kind === "category" && ref.list !== from.list) return;
    if (drag.kind === "emulator" && (ref.list === layout || (target.pos === "start") !== !!ref.item.items)) return;
    from.list.splice(from.index, 1);
    if (target.pos === "start") { ref.item.items.unshift(from.item); return; }
    const to = emuFind(layout, target.ref);
    to.list.splice(to.index + (target.pos === "after" ? 1 : 0), 0, from.item);
  });
}

// ---- Share and Import
// A list is shared as a file (JSON) or as a code: the same text, in base64, with NXPE1: before it. It carries
// the texts as they are shown, not the keys NXPress translates, so that it reads the same on any computer.
function emuExport() {
  const emulator = item => {
    const out = { name: emuText(item, "name"), address: item.address };
    if (emuText(item, "note")) out.note = emuText(item, "note");
    if (item.plain) out.plain = true;
    return out;
  };
  return {
    nxpressEmulators: 1,
    sections: emuView().map(section => ({
      title: emuText(section, "title"),
      icon: section.icon || "",
      items: section.items.map(item => (item.items ? { name: emuText(item, "name"), items: item.items.map(emulator) } : emulator(item))),
    })),
  };
}
function emuCodeOf(data) {
  const bytes = new TextEncoder().encode(JSON.stringify(data));
  let binary = "";
  for (let at = 0; at < bytes.length; at += 0x8000) binary += String.fromCharCode(...bytes.subarray(at, at + 0x8000));
  return "NXPE1:" + btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}
// what a file or a code holds, checked: { sections, counts: { s, c, e } }, or null when it is not a list of emulators.
// Only what is understood is kept: a name, a web page, a note; the text is cut to a sane length.
function emuRead(text) {
  text = String(text || "").trim();
  if (!text) return null;
  let json = text;
  if (text[0] !== "{") {
    const body = text.replace(/^NXPE1:/i, "").replace(/\s+/g, "");
    if (!/^[A-Za-z0-9_-]+={0,2}$/.test(body)) return null;
    try {
      const binary = atob(body.replace(/-/g, "+").replace(/_/g, "/"));
      json = new TextDecoder("utf-8", { fatal: true }).decode(Uint8Array.from(binary, character => character.charCodeAt(0)));
    } catch { return null; }
  }
  let data;
  try { data = JSON.parse(json); } catch { return null; }
  if (!isObject(data) || data.nxpressEmulators !== 1 || !Array.isArray(data.sections) || data.sections.length > 200) return null;
  const words = (value, max) => (typeof value === "string" ? value.trim().slice(0, max) : "");
  const emulator = raw => {
    if (!isObject(raw)) return null;
    const name = words(raw.name, 120), address = emuLink(words(raw.address, 500));
    return name && address ? { name, address, note: words(raw.note, 300), plain: raw.plain === true } : null;
  };
  const counts = { s: 0, c: 0, e: 0 };
  const sections = [];
  for (const rawSection of data.sections) {
    if (!isObject(rawSection) || !Array.isArray(rawSection.items) || rawSection.items.length > 500) continue;
    const title = words(rawSection.title, 80);
    if (!title) continue;
    const section = { title, icon: typeof rawSection.icon === "string" && icons[rawSection.icon] ? rawSection.icon : "", items: [] };
    for (const raw of rawSection.items) {
      if (isObject(raw) && Array.isArray(raw.items)) {
        const name = words(raw.name, 80);
        if (!name || raw.items.length > 500) continue;
        const category = { name, items: raw.items.map(emulator).filter(Boolean) };
        counts.c++;
        counts.e += category.items.length;
        section.items.push(category);
      } else {
        const one = emulator(raw);
        if (one) { counts.e++; section.items.push(one); }
      }
    }
    counts.s++;
    sections.push(section);
  }
  return sections.length && counts.e <= 5000 ? { sections, counts } : null;
}
// the list read, as items of a layout with new ids
function emuLayoutOf(sections) {
  const emulator = one => ({ id: emuId(), name: one.name, address: one.address, note: one.note, plain: one.plain });
  return sections.map(section => ({
    id: emuId(), title: section.title, icon: section.icon,
    items: section.items.map(item => (item.items ? { id: emuId(), name: item.name, items: item.items.map(emulator) } : emulator(item))),
  }));
}
// Add: what the list read has and the list of the person has not, put in the section and the category of the same name
function emuMerge(layout, fresh) {
  const same = (a, b) => a.trim().toLowerCase() === b.trim().toLowerCase();
  const page = address => address.toLowerCase().replace(/\/+$/, "");
  const have = new Set();
  for (const section of layout) for (const item of section.items) for (const one of item.items || [item]) have.add(page(one.address));
  const put = (list, one) => {
    if (have.has(page(one.address)) || list.some(other => !other.items && same(emuText(other, "name"), one.name))) return;
    have.add(page(one.address));
    list.push(one);
  };
  for (const section of fresh) {
    let into = layout.find(old => same(emuText(old, "title"), section.title));
    if (!into) { into = { id: section.id, title: section.title, icon: section.icon, items: [] }; layout.push(into); }
    for (const item of section.items) {
      if (!item.items) { put(into.items, item); continue; }
      let category = into.items.find(old => old.items && same(emuText(old, "name"), item.name));
      const made = !category;
      if (made) { category = { id: item.id, name: item.name, items: [] }; into.items.push(category); }
      for (const one of item.items) put(category.items, one);
      if (made && !category.items.length && item.items.length) into.items.pop();
    }
  }
}
let emuFound = null;
function emuImportCheck() {
  const text = $("emu-import-text").value;
  emuFound = text.trim() ? emuRead(text) : null;
  const status = $("emu-import-found");
  status.textContent = emuFound ? t("nEmuImportFound")(emuFound.counts.s, emuFound.counts.c, emuFound.counts.e) : (text.trim() ? t("nEmuImportBad") : "");
  status.classList.toggle("warning", !emuFound && !!text.trim());
  $("emu-import-add").disabled = $("emu-import-replace").disabled = !emuFound;
}
function emuImportApply(replace) {
  const found = emuFound;
  if (!found) return;
  const fresh = emuLayoutOf(found.sections);
  emuChange(layout => { if (replace) layout.splice(0, layout.length, ...fresh); else emuMerge(layout, fresh); });
  $("emu-import-dialog").close();
  showToast(t("nEmuImported"));
}
function emuImportOpen() {
  const dialog = $("emu-import-dialog");
  if (!dialog) return;
  if (!dialog.dataset.wired) {
    dialog.dataset.wired = "1";
    $("emu-import-text").addEventListener("input", emuImportCheck);
    $("emu-import-cancel").onclick = () => dialog.close();
    $("emu-import-add").onclick = () => emuImportApply(false);
    $("emu-import-replace").onclick = () => emuImportApply(true);
    $("emu-import-file").onclick = async () => {
      try {
        const text = await app().OpenEmulators();
        if (text) { $("emu-import-text").value = text; emuImportCheck(); }
      } catch (error) {
        showToast(/too big/i.test(String((error && error.message) || error)) ? t("nEmuImportTooBig") : t("nEmuImportBad"));
      }
    };
  }
  $("emu-import-text").value = "";
  emuImportCheck();
  openDialog(dialog);
}
async function emuShareFile() {
  try {
    const path = await app().SaveEmulators(JSON.stringify(emuExport(), null, 2));
    if (path) showToast(t("nEmuSavedFile"));
  } catch (error) { showToast(String((error && error.message) || error)); }
}
// the button Share, and under it its menu: Save as a file…, Copy as a code
function emuShareMenu(make) {
  const wrap = document.createElement("span");
  wrap.className = "emu-share";
  const menu = document.createElement("div");
  menu.className = "ncover-menu";
  menu.setAttribute("role", "menu");
  menu.hidden = true;
  const pick = make(t("nEmuShare") + " ▾", () => {
    if (!menu.hidden) { close(); return; }
    menu.hidden = false;
    pick.setAttribute("aria-expanded", "true");
    document.addEventListener("click", outside, true);
    document.addEventListener("keydown", escape, true);
    menu.querySelector("button").focus();
  });
  pick.setAttribute("aria-haspopup", "menu");
  pick.setAttribute("aria-expanded", "false");
  const close = () => {
    menu.hidden = true;
    pick.setAttribute("aria-expanded", "false");
    document.removeEventListener("click", outside, true);
    document.removeEventListener("keydown", escape, true);
  };
  const outside = event => { if (!menu.contains(event.target) && event.target !== pick) close(); };
  const escape = event => { if (event.key === "Escape") { event.stopPropagation(); close(); pick.focus(); } };
  for (const [key, action] of [["nEmuShareFile", emuShareFile], ["nEmuShareCode", () => copyText(emuCodeOf(emuExport()), t("nEmuCopiedCode"))]]) {
    const item = document.createElement("button");
    item.type = "button";
    item.setAttribute("role", "menuitem");
    item.textContent = t(key);
    item.onclick = () => { close(); action(); };
    menu.append(item);
  }
  wrap.append(pick, menu);
  return wrap;
}

// the row of a line of the list
const rowOfLine = tr => tr && [...state.rows.values()].find(row => row.tr === tr);

// wirePop adds to the card of a game and to the card of a file what Details needs, from outside:
// listeners that watch the page, so that the code that draws those cards stays as it is
function wirePop() {
  wireCover();
  wireCard();
  wireRename();
  const pop = $("game-pop");
  // the name of a game under the mouse: remember its row, and when the mouse leaves it let the card
  // stay a moment, in case the mouse goes into it (to select its text or right click it). The
  // capturing listener on the page runs before the one of the name, and keeps it from closing the card
  document.addEventListener("mouseenter", event => {
    if (event.target.classList && event.target.classList.contains("game")) {
      clearTimeout(popLeave);
      popRow = rowOfLine(event.target.closest("tr")) || popRow;
      labelCard();
      if (popRow) loadParts(popRow);
    }
  }, true);
  document.addEventListener("mouseleave", event => {
    if (event.target.classList && event.target.classList.contains("game")) {
      event.stopImmediatePropagation();
      hidePopSoon();
    }
  }, true);
  // a card of a file is drawn: its Full details are added under it
  new MutationObserver(records => {
    for (const record of records) {
      for (const node of record.addedNodes) {
        if (node.nodeType !== 1) continue;
        const cards = node.matches(".file-card") ? [node] : [...node.querySelectorAll(".file-card")];
        for (const card of cards) {
          const row = rowOfLine(card.closest("tr"));
          if (row && !card.querySelector(".card-report")) addReport(card, row);
        }
      }
    }
  }).observe($("files").tBodies[0], { childList: true, subtree: true });
  // a button in the corner of the card closes it
  const close = document.createElement("button");
  close.type = "button";
  close.className = "pop-close";
  close.textContent = "×";
  close.title = t("popClose");
  close.setAttribute("aria-label", t("popClose"));
  close.addEventListener("click", event => { event.stopPropagation(); hidePop(); });
  pop.append(close);
  pop.addEventListener("mouseenter", () => clearTimeout(popLeave));
  pop.addEventListener("mouseleave", hidePop);
  pop.addEventListener("contextmenu", event => {
    event.preventDefault();
    const row = popRow, game = row && row.game;
    if (!game) return;
    // taken now: the card closes when the mouse goes to the menu
    const text = popText(), chosen = String(getSelection());
    openMenu([
      chosen && [t("menuCopySelection"), () => copyText(chosen, t("copiedDetails"))],
      [t("menuCopyDetails"), () => copyText(text, t("copiedDetails"))],
      game.titleId && [t("menuCopyId"), () => copyText(game.titleId)],
      [t("menuCopyName"), () => copyText(shownName(row))],
      [t("menuCopyPath"), () => copyText(row.item.path)],
    ].filter(Boolean), event.clientX, event.clientY);
  });
}

// the help of the options (#67): the text that tells what an option does is out of the way, and
// shows in a card while the mouse is on the name of the option, or the option has the focus of
// the keyboard; a small ⓘ after the name tells there is one. The texts stay in the page, filled
// by render as before, and describe their option to screen readers.
const helpOwners = new WeakMap();
let helpTimer = 0, helpHideTimer = 0, helpOpen = null, helpControl = null, helpWarm = 0;
// the name a text of help goes with: the box or the button before it, else the title of its field
function helpName(hint) {
  if (hint.id === "about") return $("title");
  const item = hint.closest("label.choice, .pref-text");
  if (item) return item.querySelector("b");
  for (let element = hint.previousElementSibling; element; element = element.previousElementSibling) {
    if (element.matches("label.check, .level-head")) return element.querySelector("span");
    if (element.matches("button:not(.small-link), h2, .label")) return element;
  }
  return null;
}
function setupHelp() {
  const hints = document.querySelectorAll("#about, #settings .hint:not(.always), #settings label.choice small, #preferences .pref-text > small:not(.pref-keys):not(.pref-status):not(.credit-role)");
  hints.forEach((hint, i) => {
    const name = helpName(hint);
    if (!name) return;
    hint.id ||= "help-" + i;
    hint.classList.add("help-text");
    name.classList.add("has-help");
    name.dataset.help = hint.id;
    // the control the name goes with is the one described, and its focus shows the help; a name
    // with no control (a title) takes the focus itself
    const control = name.closest("label")?.control || (name.id && document.querySelector(`[aria-labelledby~="${name.id}"]`)) || name.closest(".pref-item")?.querySelector(":scope > :is(input, select, button)") || name;
    const described = (control.getAttribute("aria-describedby") || "").split(" ").filter(Boolean);
    if (!described.includes(hint.id)) control.setAttribute("aria-describedby", [...described, hint.id].join(" "));
    helpOwners.set(control, name);
    if (control === name && !name.matches("button")) name.tabIndex = 0;
  });
  document.addEventListener("pointerover", event => {
    if ($("help-pop").contains(event.target)) { clearTimeout(helpHideTimer); return; }
    const name = event.target.closest(".has-help");
    if (!name || event.buttons) return;
    clearTimeout(helpHideTimer);
    if (name === helpOpen) return;
    clearTimeout(helpTimer);
    // a moment on the name first, so the cards do not flash while the mouse goes by; one after
    // another comes at once
    helpTimer = setTimeout(() => showHelp(name, null), helpOpen || Date.now() < helpWarm ? 40 : 300);
  });
  document.addEventListener("pointerout", event => {
    const name = event.target.closest(".has-help");
    if (!name && !$("help-pop").contains(event.target)) return;
    const to = event.relatedTarget;
    if (to && ((name && name.contains(to)) || $("help-pop").contains(to) || (helpOpen && helpOpen.contains(to)))) return;
    // brought by the keyboard, it stays while its field has the focus
    if (helpControl && document.activeElement === helpControl) return;
    clearTimeout(helpTimer);
    helpHideTimer = setTimeout(hideHelp, 180);
  });
  document.addEventListener("focusin", event => {
    const name = helpOwners.get(event.target);
    if (name && event.target.matches(":focus-visible")) showHelp(name, event.target);
  });
  document.addEventListener("focusout", event => { if (helpOpen && event.target === helpControl) hideHelp(); });
  document.addEventListener("keydown", event => { if (event.key === "Escape") hideHelp(); });
  // a click elsewhere closes it; one in the field being filled in leaves it open
  document.addEventListener("pointerdown", event => { if (!$("help-pop").contains(event.target) && !(helpControl && helpControl.contains(event.target))) hideHelp(); });
  document.addEventListener("scroll", hideHelp, true);
  window.addEventListener("blur", hideHelp);
}
function showHelp(name, control) {
  const hint = $(name.dataset.help);
  const text = hint ? hint.textContent.trim() : "";
  if (!text || !name.getClientRects().length) return;
  clearTimeout(helpTimer);
  clearTimeout(helpHideTimer);
  if (helpOpen && helpOpen !== name) helpOpen.classList.remove("help-open");
  helpOpen = name;
  helpControl = control;
  name.classList.add("help-open");
  const pop = $("help-pop");
  pop.classList.remove("in", "above");
  pop.classList.toggle("wide", hint.id === "about" || text.length > 240);
  // in the color of the group of the section; in Settings, in the blue of NXPress
  const tone = name.closest("#preferences") ? "" : getComputedStyle($("title-tile")).getPropertyValue("--tone").trim();
  pop.style.setProperty("--help-tone", tone || "var(--accent)");
  pop.querySelector(".help-title span").textContent = name.textContent.trim();
  pop.querySelector(".help-body").textContent = text;
  pop.hidden = false;
  // under the ⓘ, which follows the last word of the name, or above it when there is no room
  // under; never out of the window; when the keyboard brought it, it leaves the field being
  // filled in sight
  const range = document.createRange();
  range.selectNodeContents(name);
  const words = range.getClientRects();
  const box = name.getBoundingClientRect();
  const last = words.length ? words[words.length - 1] : box;
  const x = Math.min(last.right + (name.id === "title" ? 17 : 13), box.right);
  let top = box.top, bottom = box.bottom;
  if (control && control !== name) {
    const field = control.getBoundingClientRect();
    top = Math.min(top, field.top);
    bottom = Math.max(bottom, field.bottom);
  }
  const width = pop.offsetWidth, height = pop.offsetHeight, gap = 10;
  const above = bottom + gap + height > innerHeight - 8 && top - gap - height > 8;
  const left = Math.max(8, Math.min(x - width / 2, innerWidth - width - 8));
  pop.style.left = left + "px";
  pop.style.top = Math.max(8, above ? top - gap - height : bottom + gap) + "px";
  pop.classList.toggle("above", above);
  pop.querySelector(".help-arrow").style.left = Math.max(10, Math.min(x - left - 6, width - 22)) + "px";
  requestAnimationFrame(() => pop.classList.add("in"));
}
function hideHelp() {
  clearTimeout(helpTimer);
  if (!helpOpen) return;
  helpOpen.classList.remove("help-open");
  helpOpen = null;
  helpControl = null;
  helpWarm = Date.now() + 600;
  $("help-pop").classList.remove("in");
  setTimeout(() => { if (!helpOpen) $("help-pop").hidden = true; }, 160);
}
// the window drawn again: the card follows the text of its option (the level, the check chosen),
// and goes with an option no longer shown
function refreshHelp() {
  if (!helpOpen) return;
  const hint = $(helpOpen.dataset.help);
  if (!helpOpen.getClientRects().length || !hint || !hint.textContent.trim()) hideHelp();
  else $("help-pop").querySelector(".help-body").textContent = hint.textContent.trim();
}

// the menu of a file, with a right click: show it in its folder, copy its title ID, its name or
// its path, check only it, take it out of the list
// a file of the work at hand can be skipped while the rest goes on, at work or still waiting:
// not in Combine, Measure or Optimize for install, which take the files together
function skippable(row) {
  const run = state.lastRun;
  if (!state.busy || !run || row.run !== run.started || !["work", "waiting"].includes(row.status)) return false;
  return !["pack", "measure"].includes(run.mode) && !(run.mode === "compress" && run.request.optimize);
}
function showRowMenu(row, remove, x, y) {
  hidePop();
  const menu = $("row-menu");
  const copy = text => { window.runtime.ClipboardSetText(text); showToast(t("copied")(text)); };
  const items = [
    ["menuShow", () => app().ShowFile(row.output || row.item.path)],
    state.mode === "info" && ["menuCopyReport", async () => { const text = await reportOf(row); if (text) copyText(text, t("copiedDetails")); }],
    row.game && row.game.titleId && ["menuCopyId", () => copy(row.game.titleId)],
    ["menuCopyName", () => copy(shownName(row))],
    ["menuCopyPath", () => copy(row.item.path)],
    skippable(row) && ["menuSkip", () => { app().Skip(row.item.path); showToast(t("skipAsked")(shownName(row))); }],
    /\.(nsp|nsz|xci|xcz)$/i.test(row.item.path) && ["menuVerify", () => start("verify", { files: [row.item.path], removeSource: false, output: "", watch: false }), true],
    ["menuRemove", remove, true],
  ].filter(Boolean);
  openMenu(items.map(([key, action, needsIdle]) => [t(key), action, !!needsIdle && state.busy]), x, y);
}
// openMenu shows a menu of [text, action, off] at x, y, inside the window
function openMenu(entries, x, y) {
  const menu = $("row-menu");
  menu.innerHTML = "";
  for (const [text, action, off] of entries) {
    const button = document.createElement("button");
    button.setAttribute("role", "menuitem");
    button.textContent = text;
    button.disabled = !!off;
    button.onclick = () => { hideRowMenu(); action(); };
    menu.append(button);
  }
  menu.hidden = false;
  menu.style.left = Math.max(8, Math.min(x, innerWidth - menu.offsetWidth - 8)) + "px";
  menu.style.top = Math.max(8, Math.min(y, innerHeight - menu.offsetHeight - 8)) + "px";
  replay("enter", menu);
  // what had the focus gets it back when the menu closes
  if (!menu.contains(document.activeElement)) menuOpener = document.activeElement;
  menu.querySelector("button:not(:disabled)")?.focus();
}
let menuOpener = null;

// the export of Details and of the log to a spreadsheet (#89): a menu of the two formats under
// the button, then where to save it
function exportMenu(button, run, withText) {
  const box = button.getBoundingClientRect();
  const formats = [[t("exportXlsx"), () => run("xlsx")], [t("exportCsv"), () => run("csv")]];
  // Details also writes the full text of each file, one after the other
  if (withText) formats.push([t("exportTxt"), () => run("txt")]);
  openMenu(formats, box.left, box.bottom + 4);
}
const today = () => { const now = new Date(); return [now.getFullYear(), now.getMonth() + 1, now.getDate()].map(n => String(n).padStart(2, "0")).join("-"); };
async function exported(call) {
  try {
    const path = await call();
    if (path) showToast(t("exported")(path.split(/[\\/]/).pop()));
  } catch (error) { setStatus(String(error)); }
}
// Details: a row for each title of each file, in the order of the list
function exportDetails(format) {
  const byRow = new Map([...state.rows].map(([path, row]) => [row.tr, path]));
  const paths = [...$("files").tBodies[0].rows].map(tr => byRow.get(tr)).filter(Boolean);
  const header = [t("file"), t("colFormat"), t("colSize"), t("colTitleId"), t("colType"), t("colVersion"), t("colShown"), t("colName"), t("colPublisher"), t("colNewest"), t("colFolder"), t("colProblem")];
  const kinds = Object.fromEntries(["BASE", "UPD", "DLC"].map(kind => [kind, t("gameKind")(kind)]));
  exported(() => app().ExportDetails(paths, format, { sheet: t("info"), file: `NXPress ${t("info")} ${today()}`, header, kinds }));
}
// the log: a row for each file of the works shown, as they are shown
function exportLog(format) {
  const { workShown, rowShown } = worklogFilter();
  const stamp = when => { const date = new Date(when); return [date.getFullYear(), date.getMonth() + 1, date.getDate()].map(n => String(n).padStart(2, "0")).join("-") + " " + date.toLocaleTimeString(state.language, { hour: "2-digit", minute: "2-digit" }); };
  const rows = state.worklog.slice().reverse().filter(workShown).flatMap(work => work.rows.filter(rowShown).map(row => [stamp(work.when), workName(work.mode), row.name, stateWord(row.status, work.mode, row.damaged), row.note || ""]));
  const header = [t("colDate"), t("colSection"), t("file"), t("colResult"), t("colNote")];
  exported(() => app().ExportLog(format, { sheet: t("worklog"), file: `NXPress ${t("worklog")} ${today()}`, header, kinds: {} }, rows));
}

function hideRowMenu() {
  const menu = $("row-menu");
  if (menu.hidden) return;
  const inside = menu.contains(document.activeElement);
  menu.hidden = true;
  if (inside) refocus(menuOpener);
  menuOpener = null;
}
// refocus gives the focus back to an element that had it, when it can still take it
function refocus(element) {
  if (element && element !== document.body && element.isConnected && !element.disabled && !element.closest("[hidden]")) element.focus({ preventScroll: true });
}
// openDialog opens a dialog over the window and, once closed, gives the focus back to what
// opened it (once the work it started has drawn the window again)
function openDialog(dialog, opener = document.activeElement) {
  if (dialog.open) return;
  dialog.addEventListener("close", () => setTimeout(() => refocus(opener)), { once: true });
  dialog.showModal();
}

// the name of a file to show: the one of its game when read, else its name without [ID][version]
function shownName(row) {
  if (row.game && row.game.name) return row.game.name;
  return row.item.name.replace(/(\s*\[[^\]]*\])+\s*$/, "").trim() || row.item.name;
}

// a notice at the bottom of the window, with Undo when there is something to undo
let toastTimer = 0;
let undoLast = null;
function showToast(text, undo) {
  // a notice waits for the dialog open (the card at the end of a work) to close, so it is not
  // shown under it, as the library app does
  const dialog = document.querySelector("dialog[open]");
  if (dialog) { dialog.addEventListener("close", () => showToast(text, undo), { once: true }); return; }
  clearTimeout(toastTimer);
  $("toast-text").textContent = text;
  $("toast-undo").hidden = !undo;
  undoLast = undo || null;
  $("toast").hidden = false;
  replay("enter", $("toast"));
  toastTimer = setTimeout(() => { $("toast").hidden = true; undoLast = null; }, undo ? 6000 : 2500);
}
function undo() {
  if (!undoLast || state.busy) return;
  const action = undoLast;
  undoLast = null;
  $("toast").hidden = true;
  action();
}
// files taken out of the list come back with Undo, where they were, and with where they came from
// (restore puts it back)
function takenOut(items, name, restore) {
  const kept = items.slice();
  showToast(kept.length === 1 ? t("toastRemoved")(name || kept[0].name) : t("toastCleared")(kept.length), () => {
    if (restore) restore();
    for (const item of kept) add(item);
    previewPacks();
    render();
  });
}

// the card at the end of a work that wrote files: what it saved, how long it took, and the
// files that saved the most
function showDone(event, status) {
  const writes = ["compress", "decompress", "recompress", "trim", "convert", "split", "pack", "fat32"];
  if (event.cancelled || !state.lastRun || !writes.includes(state.lastRun.mode)) return;
  const rows = [...state.rows.values()].filter(row => row.run === state.lastRun.started);
  if (!rows.length) return;
  const savings = rows.filter(row => row.status === "ok" && row.after > 0 && row.item.size > row.after)
    .map(row => ({ row, saved: row.item.size - row.after })).sort((a, b) => b.saved - a.saved);
  const saved = savings.reduce((sum, entry) => sum + entry.saved, 0);
  $("done-saved").hidden = saved === 0;
  if (saved > 0) countUp($("done-saved"), saved, bytes => t("saved")(bytes));
  $("done-stats").textContent = status;
  $("done-top-block").hidden = savings.length < 2;
  $("done-top").innerHTML = "";
  for (const { row, saved: amount } of savings.slice(0, 3)) {
    const item = document.createElement("li");
    item.innerHTML = `<span></span><b></b>`;
    item.querySelector("span").textContent = shownName(row);
    item.querySelector("b").textContent = size(amount);
    $("done-top").append(item);
  }
  const output = rows.find(row => row.output);
  $("done-folder").hidden = !output;
  $("done-folder").onclick = () => { if (output) app().ShowFile(output.output); };
  // the card wears the color of the game that saved the most; with nothing saved it stays
  // neutral, and red only when every file failed
  const best = savings.length ? savings[0].row : null;
  const tone = best && best.game && coverTone(best.game);
  const allFailed = rows.every(row => row.status === "failed");
  $("done-dialog").style.setProperty("--done-tone", saved > 0 ? tone || "var(--ok)" : allFailed ? "var(--bad)" : "var(--tone-neutral)");
  // after the third work that saved space, once and never again: a star on GitHub
  const works = load("worksSaved", 0) + (saved > 0 ? 1 : 0);
  save("worksSaved", works);
  $("done-star").hidden = works < 3 || load("starAsked", false);
  openDialog($("done-dialog"));
}

// the notes of the results, which come in English, in the language chosen
let nupHintAt = 0;
// one note of NXPress as the language writes it
function noteKey(key) {
  const own = texts[state.language] && texts[state.language].notes || {};
  const english = texts.en.notes || {};
  return own[key] !== undefined ? own[key] : english[key];
}
function noteText(note) {
  const own = texts[state.language] && texts[state.language].notes || {};
  const english = texts.en.notes || {};
  for (const [pattern, key] of noteRules) {
    const replacement = own[key] !== undefined ? own[key] : english[key];
    if (replacement !== undefined) note = note.replace(pattern, replacement);
  }
  return note;
}

// the line of state under the list, kept as what it says, a text of the window with its figures
// ({key, args, after}) or a note of NXPress, so it is said again in another language
let statusParts = [];
function setStatus(...parts) {
  statusParts = parts;
  paintStatus();
}
function addStatus(part) {
  statusParts.push(part);
  paintStatus();
}
function paintStatus() {
  $("status").textContent = statusParts.map(part => {
    if (typeof part === "string") return noteText(part);
    const text = t(part.key);
    return (typeof text === "function" ? text(...(part.args || [])) : text) + (part.after || "");
  }).filter(Boolean).join(" · ");
}
// onePack tells a work that writes one file with all the files given into a file chosen by
// name, as the window did before 1.3.0 (a work of then, resumed): its result is that file. One
// file with everything named by its template tells its files as a SuperNSP per game does
const onePack = run => !!run && run.mode === "pack" && !run.request.packGames && !!run.request.pack;

// stepsOf are the steps of a file in the work running, in their order: the work, then the check
// of what it wrote; a trimmed NSP is checked while it is written, so it has one step
function stepsOf(row) {
  const mode = (state.lastRun && state.lastRun.mode) || state.mode;
  // splitting and joining for FAT32 share their mode: the work is the one this file was doing
  if (row.step && !row.step.startsWith("Verifying")) row.work = row.step.split(" ")[0];
  const work = { compress: "Compressing", recompress: "Recompressing", fat32: row.work || "Splitting" }[mode];
  if (work) return [work, "Verifying"];
  // a dump folder is packed into a NUP, then the NUP is read back whole
  if (mode === "pack" && row.item.dump) return ["Packaging", "Verifying"];
  if (mode === "trim" && /\.xc[iz]$/i.test(row.item.path)) return ["Trimming", "Verifying"];
  return [];
}

// the colour of a step, as in the library app: the check green, the work orange
function stepTone(step) {
  return step === "Verifying" ? "check" : "work";
}

function stepText(step) {
  const table = texts[state.language] && texts[state.language].steps || {};
  const [first, ...rest] = step.split(" ");
  return [table[first] || first, ...rest].join(" ");
}

// warnings of the list, from the games read: an update with a newer one beside it, a title given
// twice; in Pack also an update or a DLC without its game, and a DLC that needs a newer game
// than the list brings; and, with the versions published, a newer version out
function listWarnings() {
  const read = [...state.rows.values()].filter(row => !row.item.result && row.game && row.game.titleId);
  const games = new Map();
  for (const row of read) {
    const id = row.game.type === "BASE" ? row.game.titleId : row.game.gameId;
    if (!id) continue;
    if (!games.has(id)) games.set(id, { base: false, reach: -1 });
    const game = games.get(id);
    if (row.game.type === "BASE") game.base = true;
    if (row.game.type !== "DLC") game.reach = Math.max(game.reach, row.game.latest || 0);
  }
  const warnings = new Map();
  // in Rename a title given twice matters when both files would get the same name: the same
  // version, with the same extension and in the same folder (an NSP and its NSZ do not clash)
  const extension = row => (row.item.path.match(/\.\w+$/) || [""])[0].toLowerCase();
  const folder = row => row.item.path.replace(/[\\/][^\\/]*$/, "").toLowerCase();
  const twice = (row, other) => other.game.version === row.game.version && (state.mode !== "rename" || extension(other) === extension(row) && folder(other) === folder(row));
  // in Rename, with any template: a file whose new name another file of its folder would end
  // with too (renamed or not); NXPress then leaves it as it is
  if (state.mode === "rename") {
    const rows = [...state.rows.values()].filter(row => !row.item.result);
    const final = row => folder(row) + "/" + (row.preview || row.item.name).toLowerCase();
    const count = new Map();
    for (const row of rows) count.set(final(row), (count.get(final(row)) || 0) + 1);
    for (const row of rows) if (row.preview && row.preview !== row.item.name && count.get(final(row)) > 1) warnings.set(row, t("warnSameName"));
  }
  for (const row of read) {
    if (warnings.has(row)) continue;
    const same = read.filter(other => other !== row && other.game.titleId === row.game.titleId && other.game.type === row.game.type);
    const newer = row.game.type === "UPD" ? same.filter(other => other.game.version > row.game.version).sort((a, b) => b.game.version - a.game.version)[0] : null;
    const game = games.get(row.game.type === "BASE" ? row.game.titleId : row.game.gameId);
    if (newer) warnings.set(row, t("warnNewer")(newer.game.versionTxt || "v" + newer.game.version));
    else if (same.some(other => twice(row, other))) warnings.set(row, t("warnTwice"));
    else if (state.mode === "pack" && row.game.type !== "BASE" && row.game.titles === 1 && game && !game.base) warnings.set(row, t("warnNoGame"));
    else if (state.mode === "pack" && row.game.type === "DLC" && row.game.needsGame > 0 && game && game.base && game.reach < row.game.needsGame) warnings.set(row, t("warnNeedsGame")(row.game.needsGame, game.reach));
  }
  publishedWarnings(read, warnings);
  return warnings;
}
// the title ID the versions of a file are published under: the one of its game for an update,
// its own for a game or a DLC
const publishedId = game => game.type === "UPD" ? game.gameId : game.titleId;
// a newer version published than the list brings: told on the newest file of the game (or of the
// DLC), as "you have" its version (v0 for a game without its update), and not on the older ones,
// which have the warning of the newer one in the list already. Each row keeps what is known, for
// its card.
function publishedWarnings(read, warnings) {
  for (const row of state.rows.values()) row.online = null;
  if (!$("published").checked || !state.versionsDate) return;
  const unknown = [...new Set(read.map(row => publishedId(row.game)).filter(id => id && !state.published.has(id)))];
  if (unknown.length) askPublished(unknown);
  const reach = new Map();
  for (const row of read) {
    const id = publishedId(row.game);
    if (id) reach.set(id, Math.max(reach.has(id) ? reach.get(id) : -1, row.game.latest || 0));
  }
  for (const row of read) {
    const id = publishedId(row.game), newest = id && state.published.get(id);
    if (!newest) continue;
    const have = reach.get(id);
    row.online = { version: newest.version, date: newest.date, reach: have };
    if (!warnings.has(row) && newest.version > have && (row.game.latest || 0) === have) warnings.set(row, t("warnPublished")(newest.version, day(newest.date), have));
  }
}
// asks once for the newest versions published of title IDs, and tells the warnings again once known
function askPublished(ids) {
  for (const id of ids) state.published.set(id, undefined);
  app().Published(ids).then(found => {
    for (const id of ids) state.published.set(id, found[id] || null);
    warnSoon();
  }).catch(() => { for (const id of ids) state.published.delete(id); });
}
// the versions published of each game, when the setting allows it: downloaded once a day at most,
// else the copy kept, with nothing said when there is no connection (see published.go)
async function loadVersions() {
  if (!$("published").checked) return;
  const date = await app().LoadVersions().catch(() => "");
  if (!$("published").checked) return;
  state.versionsDate = date;
  state.published.clear();
  paintPublished();
  warnSoon();
}
// the day of the list of versions under its setting, or that there is none yet
function paintPublished() {
  const date = $("published").checked ? state.versionsDate : null;
  $("published-status").textContent = date === null ? "" : date ? t("publishedOf")(day(date)) : t("publishedNone");
}
// the most text the log of a work keeps, its last part (#135)
const logKept = 4 << 20;
// what the work told and the log has not shown yet, added ten times a second at most (#135)
let said = [], sayFrame = 0;
// clearLog empties the log, with what was still to be shown in it
function clearLog() {
  said = [];
  if (sayFrame) clearTimeout(sayFrame);
  sayFrame = 0;
  $("log").textContent = "";
  $("log").hidden = true;
}
let warnTimer = 0;
function warnSoon() {
  clearTimeout(warnTimer);
  warnTimer = setTimeout(() => {
    const warnings = listWarnings();
    for (const row of state.rows.values()) {
      const warn = warnings.get(row) || "";
      if (warn !== (row.warn || "")) { row.warn = warn; paint(row); }
    }
  }, 100);
}

// the places of NXPress in the Explorer (#87, #88): each button adds what is not there, or there
// for another copy of NXPress, and takes away what is there for this one; green when it is
// there, orange when it needs a look
function paintExplorer() {
  const places = state.explorer;
  for (const [button, status, place] of [["explorer-menu", "menu-status", places.menu], ["explorer-files", "files-status", places.files]]) {
    $(button).textContent = place === "here" ? t("sendToOff") : place === "moved" ? t("explorerAgain") : t("sendToOn");
    $(status).textContent = place === "here" ? t("explorerAdded") : place === "moved" ? t("explorerMoved") : "";
    $(status).classList.toggle("ok", place === "here");
    $(status).classList.toggle("warn", place === "moved");
  }
  // the types another program still opens, which Windows lets only the user give to NXPress
  $("files-others").textContent = places.files === "here" && places.others.length ? t("explorerOthers")(places.others.join(", ")) : "";
  $("open-start-row").hidden = places.files !== "here";
  $("open-start").checked = load("openStart", false);
}
// the entries of the menu of the Explorer, by the names of the sections
const menuTexts = () => ({ verify: t("verify"), info: t("info"), compress: t("compress"), decompress: t("decompress"), pack: t("pack") });
async function setExplorer(call, after) {
  try {
    state.explorer = await call();
    if (after) after();
  } catch (error) {
    setStatus(String(error));
    state.explorer = await app().ExplorerPlaces().catch(() => state.explorer);
  }
  render();
}

// files from the Explorer while the window is open: dropped on NXPress or sent to it, opened with
// a double click (#87) or from its menu (#88). They make a new list in their section, which Undo
// takes back; the ones that come within a few seconds join it, as the Explorer starts an NXPress
// for each file chosen. While a work goes on they wait for its end.
async function takeOpened() {
  let opened = [];
  try { opened = await app().TakeOpened(); } catch { return; }
  for (const files of opened) await receiveOpened(files);
}
async function receiveOpened(opened) {
  if (!opened.files.length) return;
  if (state.busy || state.starting) {
    state.openedWaiting.push(opened);
    showToast(t("openedLater")(state.openedWaiting.reduce((sum, waiting) => sum + waiting.files.length, 0)));
    return;
  }
  const section = modes.includes(opened.section) ? opened.section : state.mode;
  const now = Date.now();
  const joins = now - state.openedAt < 3000 && state.mode === section;
  state.openedAt = now;
  state.view = "";
  if (joins) {
    await refill(opened.files, false);
    state.openedCount += opened.files.length;
  } else {
    const before = { mode: state.mode, items: state.items.slice(), sources: state.sources.slice(), removed: [...state.removed] };
    state.mode = section;
    save("mode", section);
    await refill(opened.files, true);
    state.openedCount = opened.files.length;
    state.openedBefore = before.items.length ? before : null;
  }
  if (state.openedBefore) showToast(t("openedNew")(state.openedCount), () => {
    const before = state.openedBefore;
    state.openedBefore = null;
    clearList();
    state.mode = before.mode;
    save("mode", before.mode);
    addOrigins(before.sources);
    for (const path of before.removed) state.removed.add(path);
    for (const item of before.items) add(item);
    if (state.mode === "rename") previewNames();
    previewPacks();
    render();
  });
  render();
  if (opened.open && load("openStart", false)) startSoon();
}
// a double click starts checking by itself when asked (#87), once the files stop coming
let startTimer = 0;
function startSoon() {
  clearTimeout(startTimer);
  startTimer = setTimeout(() => {
    startTimer = 0;
    if (!state.busy && !state.starting && state.mode === "verify" && state.items.length) start();
    // a restart waited for this check: with none to do, it restarts now
    else if (state.restartAfterWork) restartUpdated();
  }, 1500);
}

function render() {
  paintOfficial();
  warnSoon();
  document.documentElement.lang = state.language;
  // the question asked as the window closes with files at work, in this language too
  if (state.closeLanguage !== state.language) {
    state.closeLanguage = state.language;
    app().CloseTexts(t("closeAsk"), t("closeYes"), t("closeNo")).catch(() => { });
  }
  // the menu of the Explorer speaks the language of the page (#88)
  if (state.explorer.menu === "here" && load("menuLanguage", "") !== state.language) {
    save("menuLanguage", state.language);
    app().RelabelExplorerMenu(menuTexts()).catch(() => { });
  }
  paintStatus();
  document.querySelectorAll("[data-t]").forEach(element => { element.textContent = t(element.dataset.t); });
  document.querySelectorAll("[data-t-title]").forEach(element => { element.title = t(element.dataset.tTitle); });
  // the name of a button that shows an icon, or no text on a narrow window
  document.querySelectorAll("[data-t-label]").forEach(element => { element.setAttribute("aria-label", t(element.dataset.tLabel)); });
  document.querySelectorAll("[data-t-placeholder]").forEach(element => { element.placeholder = t(element.dataset.tPlaceholder); });
  // the section chosen with the keyboard keeps the focus while the menu is drawn again
  const focused = $("modes").contains(document.activeElement) ? document.activeElement.dataset.mode : "";
  $("modes").innerHTML = "";
  for (const [group, members] of groups) {
    if (!sideGroupShown(group)) continue;
    const title = document.createElement("div");
    title.className = "group";
    title.textContent = t(group);
    $("modes").append(title);
    for (const mode of members) {
      if (!sideToolShown(mode)) continue;
      const button = document.createElement("button");
      button.innerHTML = tile(mode, group) + "<span><b></b><small></small></span>";
      button.dataset.mode = mode;
      button.querySelector("b").textContent = t(mode);
      // the menu keeps its tiles only on a narrow window: the name shows on hover
      button.title = t(mode);
      button.querySelector("small").textContent = t("hint" + mode[0].toUpperCase() + mode.slice(1));
      button.className = mode === state.mode && !state.view ? "on" : "";
      // the tile chosen pops; the menu is drawn again while it does, and the pop goes on where it was
      const since = performance.now() - (state.chosenAt || 0);
      if (mode === state.mode && !state.view && since < 300) { button.classList.add("chosen"); button.style.setProperty("--since", -since + "ms"); }
      button.onclick = () => {
        // while a work runs a section changes nothing, but it leaves the settings for the work
        if (state.busy) { if (state.view) { state.view = ""; render(); } return; }
        const changed = state.view || mode !== state.mode;
        // a section leaves the page of the settings
        if (state.view) { state.view = ""; render(); }
        if (changed) state.chosenAt = performance.now();
        setMode(mode);
        if (changed) { render(); replay("enter", document.querySelector("#work .page-head"), $("drop"), $("settings")); }
      };
      $("modes").append(button);
    }
    // the log of the works, with what checks them (#69): a page, not a section of files
    if (group === "groupCheck") {
      if (sideToolShown("worklog")) {
      const button = document.createElement("button");
      button.innerHTML = tile("worklog", group) + "<span><b></b><small></small></span>";
      button.dataset.mode = "worklog";
      button.querySelector("b").textContent = t("worklog");
      button.title = t("worklog");
      button.querySelector("small").textContent = t("hintWorklog");
      button.className = state.view === "worklog" ? "on" : "";
      button.onclick = openWorklog;
      $("modes").append(button);
      }
      // the Emulator, under it, in the same color
      if (sideToolShown("emulator")) $("modes").append(emulatorButton(group));
    }
  }
  if (focused) $("modes").querySelector(`[data-mode="${focused}"]`)?.focus({ preventScroll: true });
  if (state.sideLanguage !== state.language) { state.sideLanguage = state.language; paintSidebarPrefs(); syncSidebarPrefs(); }
  // the log takes the place of the list of the section, and its settings the one of the options
  const worklog = state.view === "worklog";
  document.querySelectorAll("[data-for]").forEach(element => { element.hidden = worklog ? element.dataset.for !== "worklog" : !element.dataset.for.split(" ").includes(state.mode); });
  $("work").classList.toggle("worklog-view", worklog);
  if (worklog) paintWorklog();
  // the page of the settings takes the place of the section and its options
  const preferences = state.view === "preferences";
  // the page of the Emulator takes the place of the section and its options too
  const emulator = state.view === "emulator";
  $("work").hidden = preferences || emulator;
  // a section without options (Details) gives the room of their column to the list
  $("settings").hidden = preferences || emulator || ![...$("settings").children].some(block => !block.hidden);
  $("preferences").hidden = !preferences;
  paintEmulator(emulator);
  $("open-preferences").classList.toggle("on", preferences);
  // with no keys, Settings says so in red; the side menu keeps its own line
  $("keys-path").textContent = state.keys || t("prefKeysNone");
  $("keys-path").classList.toggle("keys-none", !state.keys);
  paintPublished();
  const page = worklog ? "worklog" : state.mode;
  $("title").textContent = t(page);
  // the section in the color of its group, its tile beside its title
  const group = worklog ? "groupCheck" : (groups.find(([, members]) => members.includes(state.mode)) || [""])[0];
  $("title-tile").className = `tile tone-${tones[group] || "neutral"}`;
  $("title-tile").innerHTML = icon(page);
  $("about").textContent = t("about" + page[0].toUpperCase() + page.slice(1));
  $("takes").innerHTML = "";
  for (const entry of (optimizing() ? takesOptimize : toCard() ? "NSP NSZ" : takesShown[state.mode]).split(" ")) {
    // "NSP/NSZ:takesUpdates": the kinds of file, and which of them it works on; ":takesDumps"
    // alone: what it takes besides files
    const [type, which] = entry.split(":");
    const tag = document.createElement("i");
    tag.textContent = which ? (type ? `${type} (${t(which)})` : t(which)) : type;
    $("takes").append(tag);
  }
  $("language-select").value = state.language;
  if (state.pendingCount) $("resume-text").textContent = t("resumeFound")(state.pendingCount);
  paintUpdate();
  for (const button of document.querySelectorAll("button[data-theme]")) button.classList.toggle("on", button.dataset.theme === state.theme);
  for (const button of document.querySelectorAll("button[data-pick-style]")) button.classList.toggle("on", button.dataset.pickStyle === state.pickStyle);
  for (const button of document.querySelectorAll("button[data-pop-close]")) button.classList.toggle("on", button.dataset.popClose === state.popClose);
  paintPalettes();
  // the list as rows, or as a grid of covers, as the section shown was left (#84)
  const view = viewOf(state.mode);
  $("files").classList.toggle("grid", view === "grid");
  paintPicks();
  for (const button of document.querySelectorAll("[data-view]")) { button.classList.toggle("on", button.dataset.view === view); button.setAttribute("aria-checked", button.dataset.view === view); }
  // what Verify checks: the button chosen, and a sentence for it
  for (const button of document.querySelectorAll("[data-verify-level]")) {
    const on = button.dataset.verifyLevel === state.verifyLevel;
    button.classList.toggle("on", on);
    button.setAttribute("aria-checked", on);
  }
  $("verify-level-hint").textContent = t({ full: "verifyFullHint", signature: "verifySignatureHint", both: "verifyBothHint" }[state.verifyLevel]);
  $("keys-banner").hidden = !!state.keys || !needsKeys(state.mode);
  // in the sections that take NUP files, the banner says they need no keys
  $("keys-nup").hidden = !["verify", "info", "rename"].includes(state.mode);
  $("toolbar").hidden = state.items.length === 0;
  applyListFind();
  // the list stays as it is while its files are at work, also from the keyboard
  for (const id of ["clear", "add-files", "add-folder", "empty-files", "empty-folder"]) $(id).disabled = state.busy;
  $("drop").classList.toggle("filled", state.items.length > 0);
  // the level falls in light (1 to 6), medium (7 to 14) or maximum (15 to 19)
  const level = +$("level").value;
  const band = level <= 6 ? "3" : level <= 14 ? "12" : "19";
  for (const button of document.querySelectorAll("[data-level]")) button.classList.toggle("on", button.dataset.level === band);
  // Delete originals deletes the files a work was given; the dump folders Combine packs, unless
  // the box under it asks for them too, and the NUP files Decompress unpacks stay as they are
  // (#79): the warning tells it for the list shown
  const kept = pickedItems().filter(item => state.mode === "pack" ? item.dump : (state.mode === "decompress" || state.mode === "split") && isNup(item.path)).length;
  const dumps = state.mode === "pack" && kept > 0;
  $("remove-dumps-option").hidden = !dumps || !$("remove").checked;
  // how their NUP is packed, only for dump folders
  $("nup-block").hidden = !dumps;
  const keptText = dumps ? ($("remove-dumps").checked ? "removeDumpsGo" : "removeKeepsDumps") : "removeKeepsNup";
  $("remove-warning").textContent = [(kept < pickedItems().length || !kept) && t("removeWarning"), kept && t(keptText)].filter(Boolean).join(" ");
  $("remove-warning").hidden = !$("remove").checked;
  $("par2-amount").hidden = !$("par2").checked;
  // the folders a watch looks at (#77): the ones added with Add folder or dropped, not the files
  // added on their own
  const watched = state.sources.filter(path => !/\.(nsp|nsz|xci|xcz|nup)$/i.test(path));
  $("watch-folders").textContent = watched.length ? t("watchFolders")(watched.length, watched.map(path => path.split(/[\\/]/).filter(Boolean).pop() || path).join(", ")) : t("watchNoFolders");
  $("watch-folders").title = watched.join("\n");
  $("watch-folders").hidden = !$("watch").checked;
  // the files that use the extra compression do not go with the installers above as they are; it
  // is for Compress only, where its box is
  $("extra-target-warning").hidden = !$("extra").checked || !["compress", "recompress"].includes(state.mode);
  // with the extra compression the game in sparse storage is compressed, and Optimize for install
  // takes the two together: the box has nothing to do then
  $("skip-sparse-updates").disabled = $("extra").checked || $("optimize").checked;
  $("keys").textContent = t("keys")(state.keys);
  $("keys").title = state.keys;
  $("keys").className = "side-keys" + (state.keys ? "" : " bad");
  $("level-value").textContent = $("level").value;
  // the slider tells its level with the band it falls in
  $("level").setAttribute("aria-valuetext", `${level} · ${t({ 3: "light", 12: "medium", 19: "maximum" }[band])}`);
  $("level-hint").textContent = t("levelHint")(level);
  // what the level trades, as a picture: higher levels make smaller files, more slowly
  $("gauge-size").style.width = (100 - (level - 1) * 1.6) + "%";
  $("gauge-speed").style.width = Math.max(8, 100 - (level - 1) * 5.1) + "%";
  // one file with everything goes next to the files, in the folder of the first one
  const nextTo = t(state.mode === "pack" && !$("pack-games").checked ? "nextToFiles" : "nextTo");
  $("output").textContent = state.output ? ltr(state.output) : nextTo;
  $("output").title = state.output;
  // a path is cut at its start, to show its end; the sentence shown when there is none is read whole
  $("output").classList.toggle("empty", !state.output);
  $("reset-output").hidden = !state.output;
  $("reset-output").title = nextTo;
  $("reset-output").setAttribute("aria-label", nextTo);
  // a SuperNSP per game, or one file with everything: both named by a template
  $("pack-template-title").textContent = t($("pack-games").checked ? "packTemplate" : "packOneName");
  $("pack-games-fields").hidden = !$("pack-games").checked;
  for (const field of [$("template"), $("pack-template")]) if (field.offsetParent) fitTemplate(field);
  $("dat").textContent = state.dat ? ltr(state.dat) : t("noDat");
  $("dat").title = state.dat;
  $("dat").classList.toggle("empty", !state.dat);
  $("reset-dat").hidden = !state.dat;
  const watching = state.busy && state.lastRun && state.lastRun.watch;
  $("start").textContent = state.busy ? (watching ? t("stop") : t("cancel")) : t("start")(state.mode);
  $("start").className = "primary big" + (state.busy ? " stop" : "");
  // while a work starts (the space is looked at) its buttons wait for it
  $("start").disabled = !!state.starting || !state.busy && state.items.length === 0;
  $("estimate").disabled = state.busy || !!state.starting || state.items.length === 0;
  $("compare-levels").disabled = state.busy || !!state.starting || state.items.length === 0;
  // with no file it still answers, telling what it needs
  $("measure").disabled = state.busy || !!state.starting;
  // the computer is named as its system names it
  const system = (state.setup.platform || "").split("/")[0];
  $("measure").textContent = t(system === "darwin" ? "measureMac" : system === "windows" ? "measurePc" : "measureComputer");
  $("pause").hidden = !state.busy;
  $("pause").textContent = state.paused ? t("resume") : t("pause");
  $("report").hidden = state.busy || !state.lastRun || ![...state.rows.values()].some(row => ["ok", "failed", "modified", "skipped"].includes(row.status));
  $("shutdown-option").hidden = watching || state.mode === "rename" || state.mode === "info";
  // hibernate only where the computer can, and forcing only to turn off: hibernating closes no
  // program (#62)
  const canHibernate = !!(state.setup && state.setup.hibernate);
  $("when-done-hibernate").hidden = !canHibernate;
  if (!canHibernate && $("when-done").value === "hibernate") $("when-done").value = "";
  $("shutdown-force-option").hidden = $("shutdown-option").hidden || $("when-done").value !== "off";
  $("shutdown-force-option").title = t("shutdownForceHint");
  $("explorer-group").hidden = !state.setup.sendTo;
  $("send-to").textContent = state.setup.sendToEnabled ? t("sendToOff") : t("sendToOn");
  paintExplorer();
  $("empty").hidden = state.items.length > 0;
  $("files").hidden = state.items.length === 0;
  document.body.classList.toggle("busy", state.busy);
  renderPresets();
  renderSound();
  summary();
  for (const row of state.rows.values()) paint(row);
  sortRows();
  refreshHelp();
}

// summary tells what the list holds, how far the work is, and what it saved.
function summary() {
  renderSpeed();
  renderLifetime();
  const rows = [...state.rows.values()];
  const text = $("summary-text");
  const bar = $("total-bar");
  text.textContent = "";
  bar.hidden = true;
  if (state.busy) {
    // the files of this work, not the ones done by a work before: all the list, the one file
    // verified alone, the files a watch finds; a pack of one file and a measure are one result
    const run = state.lastRun;
    const working = workRows();
    const count = onePack(run) || (run && run.mode === "measure") ? 1 : working.length;
    const ended = row => ["ok", "failed", "modified", "skipped"].includes(row.status);
    const done = Math.min(count, working.filter(ended).length);
    const share = Math.min(1, working.reduce((sum, row) => sum + rowShare(row), 0) / Math.max(1, count));
    text.textContent = (state.paused ? t("paused") + " · " : "") + t("working")(done, count, Math.floor(share * 100));
    if (state.lastRun && state.lastRun.watch) text.textContent = t("watching") + " · " + text.textContent;
    bar.hidden = false;
    bar.firstElementChild.style.width = (share * 100).toFixed(1) + "%";
    return;
  }
  if (!rows.length) {
    text.textContent = t("nothing");
    return;
  }
  const taken = pickedItems();
  const bytes = taken.reduce((sum, item) => sum + (item.size || 0), 0);
  text.textContent = taken.length < state.items.length ? t("pickCount")(taken.length, state.items.length, bytes) : t("selected")(state.items.length, bytes);
  const saved = rows.reduce((sum, row) => sum + (row.status === "ok" && row.after > 0 && row.item.size > row.after ? row.item.size - row.after : 0), 0);
  if (saved > 0) {
    const tag = document.createElement("span");
    tag.className = "saved";
    tag.textContent = " · " + t("saved")(saved);
    text.append(tag);
    if (state.celebrate) countUp(tag, saved, bytes => " · " + t("saved")(bytes));
  }
  state.celebrate = false;
}

// Settings, Sidebar: a switch for each group of the menu and for each of its tools; a group switched
// off hides all its tools and keeps the choice made for each of them
function paintSidebarPrefs() {
  const box = $("sidebar-rows");
  if (!box) return;
  box.textContent = "";
  const row = (name, text, hint, sub) => {
    const item = document.createElement("div");
    item.className = "pref-item" + (sub ? " pref-sub" : " pref-head");
    item.dataset.side = name;
    if (sub) { item.dataset.sideGroup = sub; item.hidden = !openSide.has(sub); }
    item.innerHTML = '<div class="pref-text"><b></b><small></small></div><input type="checkbox" class="switch" role="switch">';
    item.querySelector("b").textContent = text;
    const small = item.querySelector("small");
    if (hint) small.textContent = hint; else small.remove();
    const input = item.querySelector("input");
    input.checked = !hiddenSide.has(name);
    input.setAttribute("aria-label", text);
    input.onchange = () => {
      if (input.checked) hiddenSide.delete(name); else hiddenSide.add(name);
      save("hiddenSide", [...hiddenSide]);
      // the section open is hidden: the first one shown takes its place
      if (!state.busy && state.mode && !sideToolShown(state.mode)) { const next = firstSideMode(); if (next) setMode(next); }
      syncSidebarPrefs();
      render();
    };
    if (!sub) {
      // the name of a group opens and closes its tools
      const fold = document.createElement("button");
      fold.type = "button";
      fold.className = "side-fold quiet";
      fold.setAttribute("aria-label", text);
      const paint = () => { fold.textContent = openSide.has(name) ? "▾" : "▸"; fold.setAttribute("aria-expanded", openSide.has(name)); };
      const toggle = () => {
        if (openSide.has(name)) openSide.delete(name); else openSide.add(name);
        save("openSide", [...openSide]);
        paint();
        for (const tool of document.querySelectorAll(`#sidebar-rows [data-side-group="${name}"]`)) tool.hidden = !openSide.has(name);
      };
      fold.onclick = toggle;
      item.querySelector(".pref-text").onclick = toggle;
      paint();
      item.prepend(fold);
    }
    box.append(item);
  };
  for (const [group] of groups) {
    row(group, t(group), t("prefSidebarGroup"), "");
    for (const tool of sideTools(group)) row(tool, t(tool === "emulator" ? "nEmulator" : tool), "", group);
  }
}
// the tools of a group that is switched off are dimmed and cannot be moved
function syncSidebarPrefs() {
  for (const item of document.querySelectorAll("#sidebar-rows [data-side]")) {
    const name = item.dataset.side, group = groups.some(entry => entry[0] === name) ? name : sideGroupOf(name);
    const off = name !== group && hiddenSide.has(group);
    item.classList.toggle("off", off);
    item.querySelector("input").disabled = off;
  }
}

function setMode(mode) {
  if (state.busy || mode === state.mode) return;
  state.mode = mode;
  save("mode", mode);
  // the list is made again from all it came from, for the new mode: the files it does not take
  // leave, the ones it takes come, and the files taken out by hand stay out
  refill(state.sources.length ? state.sources : state.items.map(item => item.path), true, true);
  render();
}

// refill adds what the paths given hold for the mode, and keeps the paths as the origins of the
// list: the folders, to watch them, and all of them to fill the list again in another section.
// replace starts a new list; with sameOrigins it is the same list, for another section.
async function refill(paths, replace, sameOrigins) {
  const items = paths.length ? await app().Expand(paths, expandMode()) : [];
  if (replace && sameOrigins) clearRows();
  else if (replace) clearList();
  addOrigins(paths);
  for (const item of items) if (!sameOrigins || !state.removed.has(item.path)) add(item);
  if (state.mode === "rename") previewNames();
  previewPacks();
  render();
}
function addOrigins(paths) {
  for (const path of paths) if (!state.sources.includes(path)) state.sources.push(path);
}

// clearRows empties the list; clearList also forgets where its files came from
function clearRows() {
  state.items = [];
  state.rows.clear();
  state.lastRun = null;
  $("files").tBodies[0].innerHTML = "";
  clearLog();
  setStatus();
}
function clearList() {
  state.off = {};
  clearRows();
  state.sources = [];
  state.removed = new Set();
}

// ---- Combine and Split: which files of the list are worked on ----
// Each file of the list has a box at its left; all of them are chosen each time NXPress starts. A
// file left out stays in the list and is not given to the work. The choice is kept for each
// section (Combine and Split have their own), in memory only.
const hasPicks = mode => mode === "pack" || mode === "split";
const offOf = (mode = state.mode) => state.off[mode] || (state.off[mode] = new Set());
// pickedItems are the files of the list the work will take
const pickedItems = () => hasPicks(state.mode) ? state.items.filter(item => !offOf().has(item.path)) : state.items;
function paintPicks() {
  const on = hasPicks(state.mode);
  $("files").classList.toggle("picking", on);
  const off = offOf();
  let total = 0, chosen = 0;
  for (const row of state.rows.values()) {
    const box = row.tr.querySelector(".pick input");
    const left = on && !!box && off.has(row.item.path);
    row.tr.classList.toggle("unpicked", left);
    row.tr.classList.toggle("picked", on && !!box && !left);
    if (!box) continue;
    box.checked = !off.has(row.item.path);
    box.disabled = state.busy;
    if (!row.tr.classList.contains("find-out")) { total++; if (box.checked) chosen++; }
  }
  const all = $("pick-all");
  all.checked = total > 0 && chosen === total;
  all.indeterminate = chosen > 0 && chosen < total;
  all.disabled = state.busy || total === 0;
}
// pickChanged: what depends on the files chosen is told again
function pickChanged() {
  paintPicks();
  summary();
  previewPacks();
}

// add puts a file in the list; the new file of a pack (item.result) shows in it, but is no file
// to work on, so it stays out of the files a work is given
function add(item) {
  if (state.rows.has(item.path)) return;
  if (!item.result) state.items.push(item);
  // a file added again is no longer one taken out
  state.removed.delete(item.path);
  const tr = document.createElement("tr");
  tr.innerHTML = `<td class="name"><div class="game"><label class="pick"><input type="checkbox" checked></label><img class="icon" alt="" hidden><span class="cover" aria-hidden="true"></span><div><div class="title"></div><div class="file"></div></div></div><div class="note"></div></td><td class="size"></td><td class="state"></td><td class="remove"><button class="quiet">✕</button></td>`;
  // until the icon of the game comes, a cover tells the kind of file; a dump folder shows a folder
  const kind = item.dump ? "DUMP" : (item.path.match(/\.(\w+)$/) || ["", ""])[1].toUpperCase();
  tr.querySelector(".cover").textContent = item.dump ? "" : kind;
  if (item.dump) tr.querySelector(".cover").innerHTML = folderIcon;
  tr.querySelector(".cover").dataset.kind = kind;
  tr.querySelector(".title").textContent = item.name;
  tr.querySelector(".title").title = item.path;
  // the new file of a pack is a result, no file to choose
  const pick = tr.querySelector(".pick");
  if (item.result) pick.remove();
  else {
    const box = pick.querySelector("input");
    pick.title = t("pickRow");
    box.setAttribute("aria-label", t("pickRow"));
    box.checked = !offOf().has(item.path);
    tr.classList.toggle("unpicked", hasPicks(state.mode) && !box.checked);
    tr.classList.toggle("picked", hasPicks(state.mode) && box.checked);
    pick.addEventListener("click", event => event.stopPropagation());
    box.addEventListener("change", () => {
      if (!hasPicks(state.mode)) return;
      if (box.checked) offOf().delete(item.path); else offOf().add(item.path);
      pickChanged();
    });
  }
  const game = tr.querySelector(".game");
  // the card of the game comes after the mouse rests on its name; not over the control that chooses
  // the file, nor the room around it
  // the card of the game, as it always was: it comes when the mouse enters the name and rests; it
  // does not come for a file left out of Combine or Split, nor over the control that chooses the
  // file (the strip it fills), where it goes away and comes back when the mouse leaves it
  const wait = () => {
    clearTimeout(popTimer);
    if (tr.classList.contains("unpicked")) return;
    popTimer = setTimeout(() => { const row = state.rows.get(item.path); if (row) showPop(row, game); }, 450);
  };
  game.addEventListener("mouseenter", () => { if (!game.querySelector(".pick:hover")) wait(); });
  game.addEventListener("mouseleave", hidePop);
  const pickArea = tr.querySelector(".pick");
  if (pickArea) {
    pickArea.addEventListener("mouseenter", hidePop);
    pickArea.addEventListener("mouseleave", () => { if (game.matches(":hover")) wait(); });
  }
  // an empty file shows its 0 B; a file a work told about, whose size is not known, nothing
  tr.querySelector(".size").textContent = item.size === null ? "" : size(item.size || 0);
  const remove = () => {
    if (state.busy) return;
    hidePop();
    const gone = state.rows.get(item.path);
    state.items = state.items.filter(other => other.path !== item.path);
    state.rows.delete(item.path);
    // taken out by hand, it stays out when another section fills the list again from its
    // origins, and a watch is not given it
    const origin = state.sources.includes(item.path);
    state.sources = state.sources.filter(path => path !== item.path);
    state.removed.add(item.path);
    // the row folds away before it goes, and comes back with Undo
    tr.classList.add("row-out");
    setTimeout(() => { tr.remove(); render(); }, calm() ? 0 : 180);
    takenOut([item], shownName(gone || { item, game: null }), () => { if (origin) addOrigins([item.path]); });
  };
  tr.querySelector(".remove button").onclick = remove;
  tr.addEventListener("contextmenu", event => {
    event.preventDefault();
    const row = state.rows.get(item.path);
    if (row) showRowMenu(row, remove, event.clientX, event.clientY);
  });
  $("files").tBodies[0].append(tr);
  // the files added together come in one after the other
  tr.classList.add("row-in");
  tr.style.animationDelay = Math.min(state.items.length - 1, 12) * 25 + "ms";
  const row = { item, tr, status: "", step: "", done: 0, total: 0, note: "", output: "", error: "", after: 0, game: null, preview: "" };
  state.rows.set(item.path, row);
  describe(row);
  paint(row);
}

// the list in the order a click on a column asks for: by name, size or state, the other way round
// on a second click, and back to the order the files came in on a third (#17). The work goes in
// the order shown; while it runs, sorting by state waits for its end, so rows do not jump about.
const stateOrder = { work: 0, waiting: 1, failed: 2, modified: 3, skipped: 4, ok: 5 };
function sortValue(row, key) {
  if (key === "size") return row.item.size;
  if (key === "state") return stateOrder[row.status] ?? 6;
  return (row.game && row.game.name) || row.item.name;
}
function sortRows() {
  const { key, dir } = state.sort;
  for (const th of document.querySelectorAll("#files th[data-sort]")) {
    th.classList.toggle("sorted", th.dataset.sort === key);
    th.classList.toggle("desc", th.dataset.sort === key && dir < 0);
    th.setAttribute("aria-sort", th.dataset.sort !== key ? "none" : dir < 0 ? "descending" : "ascending");
  }
  if (state.busy && key === "state") return;
  const rows = [...state.rows.values()];
  if (key) rows.sort((a, b) => {
    const x = sortValue(a, key), y = sortValue(b, key);
    return dir * (typeof x === "string" ? x.localeCompare(y, undefined, { numeric: true, sensitivity: "base" }) : x - y);
  });
  const body = $("files").tBodies[0];
  if (rows.some((row, i) => body.rows[i] !== row.tr)) for (const row of rows) body.append(row.tr);
  if (!state.busy) state.items = rows.filter(row => !row.item.result).map(row => row.item);
}
// the names of the games come in a few at a time: the list is sorted again once they settle
let sortTimer = 0;
function sortSoon() {
  clearTimeout(sortTimer);
  sortTimer = setTimeout(sortRows, 400);
}

// the name and the icon of the game of each row, read a few at a time
const describeQueue = [];
let describing = 0;
function describe(row) {
  if (!state.keys && !isNup(row.item.path) && !row.item.dump) return;
  describeQueue.push(row);
  pumpDescribe();
}
function pumpDescribe() {
  while (describing < 2 && describeQueue.length) {
    const row = describeQueue.shift();
    if (!state.rows.has(row.item.path)) continue;
    describing++;
    app().Describe(row.item.path).then(game => {
      row.game = game;
      // a dump folder has its size once its files are added up
      if (row.item.dump && game && game.size) { row.item.size = game.size; summary(); }
      if (state.sort.key === "name") sortSoon();
    }).catch(() => { }).finally(() => { row.described = true; paint(row); warnSoon(); describing--; pumpDescribe(); });
  }
}

function paint(row) {
  const cell = row.tr.querySelector(".state");
  if (row.painted !== undefined && row.painted !== row.status && ["ok", "failed", "modified", "skipped"].includes(row.status)) {
    replay("ended-" + row.status, row.tr);
    row.endedAt = performance.now();
  }
  row.painted = row.status;
  const note = row.tr.querySelector(".note");
  const title = row.tr.querySelector(".title");
  const file = row.tr.querySelector(".file");
  const icon = row.tr.querySelector(".icon");
  if (row.game && row.game.container === "DUMP") {
    // a dump folder: its game, its newest version, and that it is a folder
    title.textContent = row.game.name;
    file.textContent = `${row.game.titleId} · v${row.game.latest} · ${t("dumpFolder")}`;
    if (row.game.size) row.tr.querySelector(".size").textContent = size(row.game.size);
  } else if (row.game && row.game.name) {
    title.textContent = row.game.name;
    if (["rename", "split", "verify", "info"].includes(state.mode)) {
      // Rename, Split, Verify and Details: the ID of the game, and under it the name of the file with its extension, nothing else
      file.replaceChildren(row.game.titleId, document.createElement("br"), row.item.name);
    } else {
      file.textContent = `${row.game.titleId} · v${row.game.version} · ${t("gameKind")(row.game.type)} · ${row.item.name}`;
    }
  } else {
    // the name of the file without its [title ID][version] tags, which go under it
    const tags = row.item.name.match(/(\s*\[[^\]]*\])+\s*$/);
    title.textContent = tags ? row.item.name.slice(0, tags.index).trim() || row.item.name : row.item.name;
    file.textContent = row.game ? `${row.game.titleId} · v${row.game.version} · ${t("gameKind")(row.game.type)}` : tags ? tags[0].trim() : "";
  }
  // a file in an experimental format of NXPress says so beside its name, "Experimental" and the
  // format, so it is found again before a program that reads plain NSZ only gets it (#58)
  if (row.game && row.game.format) {
    const why = t("experimentalWhy")(row.game.format.replace("+", ", "));
    const mark = document.createElement("span");
    mark.className = "exp-mark";
    mark.textContent = t("experimentalMark");
    mark.title = why;
    const format = document.createElement("span");
    format.className = "exp-format";
    format.textContent = row.game.format.replace("+", " + ");
    format.title = why;
    title.append(mark, format);
  }
  // what a dump folder holds, and the name of its NUP, until it is worked on
  row.tr.querySelector(".dump-plan")?.remove();
  if (row.game && row.game.container === "DUMP" && !row.status && state.mode === "pack") {
    const plan = document.createElement("div");
    plan.className = "dump-plan";
    plan.innerHTML = `<div class="dump-pieces"></div><div class="dump-arrow">→ <b></b></div>`;
    for (const piece of row.game.pieces || []) {
      const chip = document.createElement("i");
      chip.textContent = t("gameKind")(piece.type) + (piece.type === "UPD" ? ` v${piece.version}` : piece.type === "DLC" ? ` ${piece.titleId}` : "");
      plan.querySelector(".dump-pieces").append(chip);
    }
    plan.querySelector("b").textContent = row.game.nupName || "";
    plan.querySelector(".dump-arrow").hidden = !row.game.nupName;
    file.after(plan);
  }
  // a warning of the list shows until the file is worked on; in Verify and Details, which leave
  // the file as it is, it stays after the work too, where it is looked at most (#136)
  let warn = row.tr.querySelector(".list-warn");
  if (row.warn && (!row.status || ["verify", "info"].includes(state.mode))) {
    if (!warn) {
      warn = document.createElement("div");
      warn.className = "list-warn";
      file.after(warn);
    }
    warn.textContent = row.warn;
  } else warn?.remove();
  if (row.game && row.game.icon) {
    icon.src = row.game.icon;
    icon.hidden = false;
    coverTone(row.game);
    row.tr.querySelector(".cover").hidden = true;
  }
  // the cover shimmers while the game is read
  row.tr.querySelector(".cover").classList.toggle("loading", (!!state.keys || isNup(row.item.path) || !!row.item.dump) && !row.described);
  // in the grid, the cover fills up with the progress of its file
  const game = row.tr.querySelector(".game");
  game.classList.toggle("working", row.status === "work");
  row.tr.classList.toggle("waiting", row.status === "waiting");
  if (row.status === "work") {
    const share = row.total ? row.done / row.total : 0;
    // a file in steps (the work, then the check) has its bar split in them, each in its colour:
    // the ones done stay full and only the one being done fills up
    const steps = stepsOf(row);
    const index = steps.indexOf((row.step || "").split(" ")[0]);
    const split = steps.length > 1 && index >= 0;
    // the bar and the cover are kept from one update to the next, so they fill up smoothly
    // instead of jumping; a new step (or file) starts them again at once
    const shape = split ? `segs|${steps.join(",")}|${index}` : `bar|${row.step || ""}`;
    const fresh = cell.firstElementChild?.dataset.shape !== shape;
    if (fresh) {
      cell.innerHTML = split
        ? `<div class="step"><i class="dot"></i><span></span></div><div class="segs">` +
          steps.map((step, i) => `<span class="seg tone-${stepTone(step)}${i < index ? " done" : ""}">${i === index ? "<div></div>" : ""}</span>`).join("") + `</div>`
        : `<div class="step"></div><div class="bar"><div></div></div>`;
      cell.firstElementChild.dataset.shape = shape;
      game.classList.add("refill");
    }
    game.style.setProperty("--share", (share * 100).toFixed(1) + "%");
    game.dataset.share = Math.floor(share * 100) + "%";
    if (fresh) {
      // the start of a step is taken at once, and the cover fills from there on
      void getComputedStyle(game).getPropertyValue("--share");
      game.classList.remove("refill");
    }
    if (split) {
      cell.querySelector(".step .dot").className = "dot tone-" + stepTone(steps[index]);
      cell.querySelector(".step span").textContent = `${state.paused ? t("paused") : stepText(row.step)} · ${index + 1}/${steps.length} · ${Math.floor(share * 100)}%`;
      cell.querySelector(".segs div").style.width = (share * 100).toFixed(1) + "%";
    } else {
      cell.querySelector(".step").textContent = `${state.paused ? t("paused") : stepText(row.step)} · ${Math.floor(share * 100)}%`;
      cell.querySelector(".bar div").style.width = (share * 100).toFixed(1) + "%";
    }
  } else if (row.status === "waiting") {
    cell.innerHTML = `<span class="badge waiting"></span>`;
    cell.firstChild.textContent = t("waiting");
  } else if (row.status === "ok") {
    cell.innerHTML = `<span class="badge ok"></span>`;
    cell.firstChild.textContent = stateWord("ok", row.mode);
    // the space saved: the bar of the original shrinks to the new file, once, as the file ends
    if (row.after > 0 && row.item.size > row.after) {
      const share = row.after / row.item.size;
      const since = performance.now() - (row.endedAt || -1e9);
      const bar = document.createElement("div");
      bar.className = "saving" + (since < 1200 ? " shrink" : "");
      bar.style.setProperty("--to", (share * 100).toFixed(1) + "%");
      bar.style.setProperty("--since", -Math.min(since, 1200) + "ms");
      bar.innerHTML = `<div></div><small></small>`;
      bar.querySelector("small").textContent = "−" + Math.round((1 - share) * 100) + "%";
      bar.title = `${size(row.item.size)} → ${size(row.after)}`;
      cell.append(bar);
    }
  } else if (row.status === "skipped") {
    // left as it is, with why: neither done nor failed
    cell.innerHTML = `<span class="badge skipped"></span>`;
    cell.firstChild.textContent = t("skipped");
  } else if (row.status === "modified") {
    // checked, with NCA files a tool rewrote but nothing damaged: orange, as red is for errors (#42)
    cell.innerHTML = `<span class="badge modified"></span>`;
    cell.firstChild.textContent = t("modified");
  } else if (row.status === "failed") {
    cell.innerHTML = `<span class="badge ${row.error === "cancelled" ? "waiting" : "failed"}"></span>`;
    cell.firstChild.textContent = row.error === "cancelled" ? t("cancelled") : stateWord("failed", row.mode, damaged(row.status, row.error));
  } else {
    cell.innerHTML = "";
  }
  note.className = "note" + (row.status === "failed" ? " bad" : row.status === "skipped" || row.status === "modified" ? " warn" : "");
  note.textContent = noteText(row.status === "failed" ? (row.error === "cancelled" ? "" : row.error) : row.note);
  // what keeps Eglantine and Lichii from opening a NUP checked whole, from starting its game or
  // from showing it right goes on a line of its own, in orange: the note stays grey, as the file
  // is whole
  row.tr.querySelector(".emu-warn")?.remove();
  if (row.status === "ok" && isNup(row.item.path)) {
    const [kept, warn] = splitEmulators(row.note);
    if (warn) {
      note.textContent = noteText(kept);
      const line = document.createElement("div");
      line.className = "emu-warn";
      line.textContent = noteText(warn);
      note.after(line);
    }
  }
  // an error, or a file modified, opens to say what to do, with its detail to copy; it stays
  // open while drawn again
  row.tr.querySelector(".error-more")?.remove();
  const detail = row.status === "modified" ? row.note : row.status === "failed" && row.error !== "cancelled" ? row.error : "";
  if (detail) {
    const more = document.createElement("details");
    more.className = "error-more";
    more.open = !!row.open;
    more.ontoggle = () => { row.open = more.open; };
    more.innerHTML = `<summary></summary><p></p><button class="link small-link"></button>`;
    more.querySelector("summary").textContent = t("errorWhat");
    more.querySelector("p").textContent = t(adviceFor(detail));
    more.querySelector("button").textContent = t("copyDetail");
    more.querySelector("button").onclick = () => { window.runtime.ClipboardSetText(`${row.item.path}\n${detail}`); showToast(t("detailCopied")); };
    note.after(more);
  }
  if (!row.status && state.mode === "rename" && row.preview && row.preview !== row.item.name) {
    note.textContent = "→ " + row.preview;
  }
  renderCard(row);
  // the new file shows in its folder from a button beside the name of its game: it says which game
  // it is for, and adds no line to the row (#61)
  if (row.output) {
    const open = document.createElement("button");
    open.className = "folder-btn";
    open.title = `${t("showFolder")}: ${row.output}`;
    open.setAttribute("aria-label", t("showFolder"));
    open.innerHTML = `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 7.5V18a1.5 1.5 0 0 0 1.5 1.5h15A1.5 1.5 0 0 0 21 18V9a1.5 1.5 0 0 0-1.5-1.5h-7.2L10.6 5.5H4.5A1.5 1.5 0 0 0 3 7.5z"/></svg>`;
    open.onclick = event => { event.stopPropagation(); app().ShowFile(row.output); };
    title.append(open);
  }
  // the button that takes the file out of the list says so, in the language chosen
  const remove = row.tr.querySelector(".remove button");
  remove.title = t("menuRemove");
  remove.setAttribute("aria-label", t("menuRemove"));
}

// rowOf is the row of a file the work tells about, added when the list has none: a file a watch
// found, or the new file of a pack of one file, which is a result and not a file to work on
function rowOf(file) {
  if (!state.rows.has(file)) {
    const run = state.lastRun;
    add({ path: file, name: file.split(/[\\/]/).pop(), size: null, result: onePack(run) && file === run.request.pack });
  }
  return state.rows.get(file);
}

// the packs by game the list would make, before they are made: each game with a box to leave it
// out; a game packed before where its pack would go, and a game of one file, are left out
let packTimer = 0;
function previewPacks() {
  clearTimeout(packTimer);
  packTimer = setTimeout(async () => {
    const list = $("pack-plan");
    if (state.mode !== "pack" || !$("pack-games").checked) return;
    const paths = pickedItems().map(item => item.path);
    const plan = paths.length && state.keys ? await app().PackPlan(paths, state.sources, state.output, state.packTemplate) || [] : [];
    list.innerHTML = "";
    $("pack-plan-empty").hidden = plan.length > 0;
    plan.forEach((game, i) => {
      const item = document.createElement("li");
      item.style.animationDelay = Math.min(i, 12) * 25 + "ms";
      const alone = game.files.length < 2;
      const blocked = alone || (!!game.packed && !$("overwrite").checked);
      item.innerHTML = `<label class="check"><input type="checkbox"><span><b></b><small></small></span></label>`;
      const box = item.querySelector("input");
      box.checked = !blocked && !state.skipGames.has(game.id);
      box.disabled = blocked;
      box.onchange = () => { if (box.checked) state.skipGames.delete(game.id); else state.skipGames.add(game.id); item.classList.toggle("off", !box.checked); };
      item.classList.toggle("off", !box.checked);
      item.querySelector("b").textContent = game.name;
      item.querySelector("small").textContent = game.packed ? t("packPlanPacked")(game.packed) : alone ? t("packPlanOne") : t("packPlanFiles")(game.files.length);
      list.append(item);
    });
  }, 300);
}

// the fields of a template, one to a row: what each writes, an example, and its name as Switch
// Library Manager writes it (#43). The rows are made once; their texts follow the language.
const templateFields = [
  ["{name}", "{TITLE_NAME}", "fieldName", "Moonlit Orchard"],
  ["{id}", "{TITLE_ID}", "fieldId", "0100A1B2C3D40000"],
  ["{version}", "{VERSION}", "fieldVersion", "131072"],
  ["{version_txt}", "{VERSION_TXT}", "fieldVersionTxt", "1.0.5"],
  ["{type}", "{TYPE}", "fieldType", "BASE · UPD · DLC"],
  ["{titles}", "", "fieldTitles", "4"],
  ["{contents}", "{PACKAGE_CONTENTS}", "fieldContents", "1G+1U+2D"],
  ["{size_gb}", "{SIZE_GB}", "fieldSizeGb", "2.7GB"],
  ["{size_mb}", "{SIZE_MB}", "fieldSizeMb", "2650MB"],
  ["{dlc_number}", "{DLC_NUMBER}", "fieldDlcNumber", "1"],
  ["{language}", "{LANGUAGE}", "fieldLanguage", "En,Ja,Fr,De"],
  ["{region}", "{REGION}", "fieldRegion", "World · Japan"],
  ["{experimental}", "", "fieldExperimental", "NCZSECT2"],
];
function fillFields() {
  for (const table of document.querySelectorAll("table.fields")) {
    table.innerHTML = `<thead><tr><th data-t="fieldsField"></th><th data-t="fieldsWrites"></th></tr></thead><tbody></tbody>`;
    for (const [field, upper, key, example] of templateFields) {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td><code></code><small></small></td><td><span></span><small class="example"></small></td>`;
      tr.querySelector("code").textContent = field;
      tr.querySelector("td small").textContent = upper;
      tr.querySelector("span").dataset.t = key;
      tr.querySelector(".example").textContent = example;
      table.tBodies[0].append(tr);
    }
  }
}

// the new names of the files, for the template written
let previewTimer = 0;
function previewNames() {
  clearTimeout(previewTimer);
  previewTimer = setTimeout(async () => {
    const paths = state.items.map(item => item.path);
    if (!paths.length || !state.keys) return;
    const names = await app().NewNames(paths, state.template, $("keep-named").checked);
    paths.forEach((path, i) => { const row = state.rows.get(path); if (row) { row.preview = names[i]; paint(row); } });
    // two files that would get the same name are told before anything is renamed
    warnSoon();
  }, 300);
}

function settings() {
  return {
    level: +$("level").value, block: $("target-block").checked, threads: +$("threads").value, multi: +$("multi").value,
    overwrite: $("overwrite").checked, removeSource: $("remove").checked, removeDumps: $("remove-dumps").checked, output: state.output, dat: state.dat, verifyLevel: state.verifyLevel,
    nupRawBig: $("nup-raw-big").checked,
    watch: $("watch").checked, noCache: $("no-cache").checked, template: state.template, keepNamed: $("keep-named").checked,
    packGames: $("pack-games").checked, packTemplate: state.packTemplate,
    // Convert: NSP and NSZ into game cards instead of game cards into NSP and NSZ (#127)
    toCard: $("convert-to-card").checked,
    lowPriority: $("low-priority").checked,
    // recovery data (.par2) for the percent chosen of each new file, and Verify repairing with it
    par2: $("par2").checked ? +$("par2-percent").value : 0, par2Percent: +$("par2-percent").value,
    repair: $("repair").checked,
    // one box for the two experimental formats: each file gets the one it needs, or none
    sparse: $("extra").checked, archive: $("extra").checked,
    // Optimize for install, in Compress
    optimize: $("optimize").checked,
    // the update of a game in sparse storage left as it is with it, in Compress
    skipSparseUpdates: $("skip-sparse-updates").checked,
  };
}

// Compress with Optimize for install on: the games with their updates, into one NSZ each
const optimizing = () => state.mode === "compress" && $("optimize").checked;
// Convert from NSP and NSZ into game cards
const toCard = () => state.mode === "convert" && $("convert-to-card").checked;
// the files the list takes: Optimize for install takes NSZ and XCZ too, and Convert into game
// cards NSP and NSZ
const expandMode = () => (optimizing() ? "optimize" : toCard() ? "tocard" : state.mode);

// presets: the settings of a compression kept with a name
const builtinPresets = () => [
  { name: t("presetInstallers"), level: 18, block: false, builtin: true },
  { name: t("presetEmulators"), level: 18, block: true, builtin: true },
  { name: t("presetFast"), level: 3, block: false, builtin: true },
  // the smallest files: the highest level, for installers, and with the extra compression too,
  // which asks first, as its box does (#125)
  { name: t("presetMax"), level: 19, block: false, builtin: true, extra: false },
  { name: t("presetMaxExtra"), level: 19, block: false, builtin: true, extra: true },
];
// the preset chosen is kept by what it is, not by its place in the list, which changes when one
// is deleted: "builtin:<n>" for the ones of NXPress (their names follow the language), its name
// for the others
function presetKey(all, index) {
  const preset = all[index];
  return !preset ? "" : preset.builtin ? "builtin:" + index : preset.name;
}

// a template field is as tall as its text, which wraps; a new line is no part of a name
function fitTemplate(field) {
  if (/[\r\n]/.test(field.value)) field.value = field.value.replace(/\s*[\r\n]+\s*/g, " ");
  field.style.height = "auto";
  field.style.height = field.scrollHeight + 2 + "px";
}

function renderPresets() {
  const select = $("preset");
  const all = [...builtinPresets(), ...savedPresets()];
  // when NXPress opens, the preset last chosen is shown again
  let chosen = select.value;
  if (state.restorePreset !== undefined) {
    chosen = String(all.findIndex((preset, i) => presetKey(all, i) === state.restorePreset));
    delete state.restorePreset;
  }
  select.innerHTML = "";
  const none = new Option(t("presetNone"), "");
  select.append(none);
  all.forEach((preset, i) => select.append(new Option(preset.name, String(i))));
  select.value = chosen === "-1" ? "" : chosen;
  $("preset-delete").hidden = !select.value || all[+select.value].builtin;
  $("preset-name").placeholder = t("presetName");
  save("preset", select.value === "" ? "" : presetKey(all, +select.value));
  paintPresetName();
}
// the name typed for saved settings (#93): new, the name of saved settings, which Save replaces,
// or one of the settings of NXPress, which is not taken; told without telling capitals apart
function presetNameState() {
  const name = $("preset-name").value.trim().toLowerCase();
  if (!name) return { kind: "" };
  if (builtinPresets().some(preset => preset.name.toLowerCase() === name)) return { kind: "builtin" };
  const index = savedPresets().findIndex(preset => preset.name.toLowerCase() === name);
  return index >= 0 ? { kind: "replace", index } : { kind: "new" };
}
// Save becomes Replace, in orange, while the name is in use, and a line under it tells why
function paintPresetName() {
  const { kind } = presetNameState();
  $("preset-save").textContent = t(kind === "replace" ? "presetReplace" : "presetSave");
  $("preset-save").classList.toggle("replace", kind === "replace");
  $("preset-save").disabled = kind === "builtin";
  $("preset-warning").textContent = kind === "replace" ? t("presetExists")($("preset-name").value.trim()) : kind === "builtin" ? t("presetBuiltin") : "";
  $("preset-warning").hidden = !$("preset-warning").textContent;
}
function applyPreset() {
  const preset = [...builtinPresets(), ...savedPresets()][+$("preset").value];
  if (!preset) return;
  $("level").value = preset.level;
  $(preset.block ? "target-block" : "target-solid").checked = true;
  if (preset.threads) $("threads").value = preset.threads;
  if (preset.multi) $("multi").value = preset.multi;
  // the presets of NXPress say whether the extra compression goes with them: turned on, it asks
  // first, and stays off if that is not accepted
  if (preset.extra === false && $("extra").checked) $("extra").checked = false;
  if (preset.extra === true && !$("extra").checked) {
    $("extra").checked = true;
    $("extra").onchange();
    return;
  }
  save("settings", settings());
  render();
}

async function start(mode, extra) {
  // a work is starting: a second press waits for it instead of asking for another
  if (state.starting) return;
  if (state.busy) { app().Cancel(); return; }
  mode = typeof mode === "string" ? mode : state.mode;
  // the work shows on its page, with what it needs or what it does: the settings give way to it
  if (state.view) { state.view = ""; render(); }
  // what the work needs is told by its own mode: Verify only this, in a section that needs no
  // keys, still needs them
  if (!state.keys && needsKeys(mode)) { setStatus({ key: "needKeys" }); return; }
  if (!state.items.length) { setStatus({ key: mode === "measure" ? "measureNeedsGame" : "needFiles" }); return; }
  const chosen = settings();
  save("settings", chosen);
  const watches = chosen.watch && ["compress", "decompress", "recompress", "trim", "verify"].includes(mode);
  // a watch looks at the folders added; the files alone are taken too
  let files = watches ? [...new Set([...state.sources, ...state.items.map(item => item.path)])] : state.items.map(item => item.path);
  // the measure takes the first game it can compress among the NSP and XCI of the list; the
  // one it takes shows its progress, the others keep their state
  const measuring = mode === "measure";
  if (measuring) {
    files = state.items.filter(item => /\.(nsp|xci)$/i.test(item.path)).map(item => item.path);
    if (!files.length) { setStatus({ key: "measureNeedsGame" }); return; }
  }
  // the files left out in Combine and Split are not given to the work
  if (hasPicks(mode)) {
    files = files.filter(path => !offOf(mode).has(path));
    if (!files.length) { setStatus({ key: "pickNone" }); return; }
  }
  // origins: the folders dropped or chosen, which a pack of their files goes next to (#45)
  const request = { mode, files, ...chosen, watch: watches, levels: false, skipGames: [...state.skipGames], origins: state.sources, published: $("published").checked, ...(extra || {}) };
  files = request.files;
  // Verify only this asks for no watch, whatever the box says
  const watch = !!request.watch;
  // a new work keeps the computer on: the count of the turning off asked by the work before stops
  if (state.shutdownAt) keepOn();
  // while the space is looked at, the work is starting: its buttons wait (the one pressed gets
  // the focus back after the dialog of the space)
  const opener = document.activeElement;
  state.starting = true;
  render();
  let allowed = false;
  try { allowed = await spaceAllows(request, opener); } finally { state.starting = false; }
  // the button pressed lost the focus while it waited: it gets it back
  if (!allowed) { render(); refocus(opener); return; }
  // the work is marked as running before NXPress is asked to start it, as its first events (and
  // the end of a quick one) can come before the answer; what changes is put back if it does not
  // start, so another work still running keeps its rows
  const before = { lastRun: state.lastRun, paused: state.paused, pending: load("pending", null, isPending), rows: [] };
  // the rows of the work wear its start, to count them and only them
  const started = Date.now();
  const fresh = { status: "waiting", step: "", done: 0, total: 0, note: "", output: "", error: "", after: 0, run: started };
  for (const row of state.rows.values()) if (!measuring && (files.includes(row.item.path) || watch)) {
    before.rows.push([row, Object.fromEntries(Object.keys(fresh).map(key => [key, row[key]]))]);
    Object.assign(row, fresh);
  }
  clearLog();
  setStatus();
  resetSpeed();
  state.busy = true;
  state.paused = false;
  // bytes: the size of the files given, which the new file of a pack weighs in the progress
  const bytes = state.items.filter(item => files.includes(item.path)).reduce((sum, item) => sum + (item.size || 0), 0);
  state.lastRun = { mode, request, started, watch, skipped: 0, bytes };
  // the files left are kept, to go on with them after a close
  if (!watch && ["compress", "decompress", "recompress", "trim", "convert", "fat32"].includes(mode)) save("pending", { mode, request });
  app().SetTaskbar("normal", 0, 1);
  render();
  if (document.activeElement === document.body) refocus(opener);
  try {
    await app().Start(request);
    // a new pack shows its own new file: the one of a pack before leaves the list
    if (mode === "pack") for (const row of [...state.rows.values()]) if (row.item.result && row.run !== started) { row.tr.remove(); state.rows.delete(row.item.path); }
  } catch (error) {
    state.busy = false;
    state.paused = before.paused;
    state.lastRun = before.lastRun;
    save("pending", before.pending);
    for (const [row, fields] of before.rows) Object.assign(row, fields);
    app().SetTaskbar("none", 0, 0);
    setStatus(String(error));
  }
  render();
}

// spaceAllows tells, before anything is written, the drives where the new files may not fit,
// and starts only when asked to; a file that fills its drive fails, and its original stays
function spaceAllows(request, opener) {
  return app().SpaceCheck(request).then(short => {
    if (!short || !short.length) return true;
    const list = $("space-list");
    list.innerHTML = "";
    for (const drive of short) {
      const item = document.createElement("li");
      item.innerHTML = `<b></b><span></span>`;
      item.querySelector("b").textContent = drive.folder;
      item.querySelector("span").textContent = t("spaceLine")(size(drive.need), size(drive.free));
      list.append(item);
    }
    return new Promise(resolve => {
      $("space-start").onclick = () => { resolve(true); $("space-dialog").close(); };
      $("space-dialog").onclose = () => resolve(false);
      openDialog($("space-dialog"), opener);
    });
  }).catch(() => true);
}

// showMeasure tells what a measure found in a dialog: every setting tried, as fast as it went,
// and the one applied
function showMeasure(report) {
  $("measure-intro").textContent = t("measureIntro")(report.processors, report.level, report.block);
  const rows = $("measure-rows");
  rows.innerHTML = "";
  const fastest = Math.max(1, ...report.trials.map(trial => trial.speed));
  for (const trial of report.trials) {
    const tr = document.createElement("tr");
    if (trial.best) tr.className = "best";
    tr.innerHTML = `<td></td><td></td><td><div class="speed"><span></span><div class="bar"><div></div></div></div></td>`;
    tr.cells[0].textContent = trial.threads;
    tr.cells[1].textContent = trial.multi;
    tr.querySelector(".speed span").textContent = `${trial.speed} MB/s` + (trial.best ? ` · ${t("measureApplied")}` : "");
    tr.querySelector(".bar div").style.width = (trial.speed / fastest * 100).toFixed(1) + "%";
    rows.append(tr);
  }
  $("measure-result").textContent = t("measureResult")(report.threads, report.multi, report.speed, time(report.seconds10GB));
  // a heavier setting a little faster was not taken: the window tells why
  $("measure-kept").hidden = !report.faster;
  if (report.faster) $("measure-kept").textContent = t("measureKept")(report.faster.threads, report.faster.multi, report.faster.speed);
  openDialog($("measure-dialog"));
}

// the report of the last work, to paste in an issue or a thread
function report() {
  const run = state.lastRun || {};
  const r = run.request || {};
  const lines = [
    `NXPress ${state.setup.version} · ${state.setup.platform}` + (state.official === "unofficial" ? " · unofficial build" : state.official === "dev" ? " · development build" : ""),
    `${run.mode} · level ${r.level} · ${r.block ? "blocks" : "solid"} · threads ${r.threads} · files at once ${r.multi}` + (r.watch ? " · watch" : ""),
    run.status || "",
    "",
  ];
  for (const row of state.rows.values()) {
    const game = row.game ? ` [${row.game.titleId} v${row.game.version} ${row.game.type}${row.game.titles > 1 ? ", " + row.game.titles + " titles" : ""}]` : "";
    lines.push(`- ${row.item.name}${row.item.size === null ? "" : ` (${size(row.item.size)})`}${game}: ${row.status || "-"}` + (row.error ? ` — ${row.error}` : row.note ? ` — ${row.note}` : ""));
  }
  if ($("log").textContent) lines.push("", $("log").textContent.trim());
  return lines.join("\n");
}

function listen() {
  const events = window.runtime;
  // the installation of a new version, as it goes (#91)
  events.EventsOn("update-progress", progress => { state.installing = progress; paintUpdate(); });
  // files the Explorer gave to another NXPress, which sent them here
  events.EventsOn("opened", takeOpened);
  events.EventsOn("progress", event => {
    const row = rowOf(event.file);
    Object.assign(row, { status: "work", step: event.step, done: event.done, total: event.total, run: state.lastRun && state.lastRun.started });
    paint(row);
    trackSpeed();
    summary();
  });
  events.EventsOn("done", event => {
    // the mode of the work, which is not the section shown when only one file is verified
    const run = state.lastRun;
    const mode = run ? run.mode : state.mode;
    if (onePack(run)) {
      // a pack is one result for all its files: the files it was given
      const given = new Set(run.request.files);
      for (const row of state.rows.values()) if (given.has(row.item.path) && row.item.path !== event.file) Object.assign(row, { status: event.error ? "" : "ok", run: run.started });
    }
    const row = rowOf(event.file);
    if (event.skipped && state.lastRun) state.lastRun.skipped++;
    // the files this work went through, for its report
    row.run = state.lastRun && state.lastRun.started;
    Object.assign(row, { status: event.error ? "failed" : event.modified ? "modified" : event.skipped ? "skipped" : "ok", note: event.note || "", output: event.output || "", error: event.error || "", after: event.size || 0, mode: state.lastRun ? state.lastRun.mode : state.mode });
    // a file renamed keeps its row, under its new name and path
    if (mode === "rename" && row.output && !event.error) {
      const old = row.item.path;
      row.item.name = row.output.split(/[\\/]/).pop();
      row.item.path = row.output;
      state.rows.delete(old);
      state.rows.set(row.output, row);
      state.sources = state.sources.map(path => path === old ? row.output : path);
    }
    const pending = load("pending", null, isPending);
    if (pending && !event.error) {
      pending.request.files = pending.request.files.filter(path => path !== event.file);
      save("pending", pending.request.files.length ? pending : null);
    }
    paint(row);
    if (mode === "info" && !event.error) app().Card(row.item.path).then(card => { row.card = card; renderCard(row); }).catch(() => { });
    trackSpeed();
    summary();
  });
  // what the work tells goes to the log ten times a second at most, added at its end: written again whole and
  // scrolled at each file, a long Details of thousands of files kept the window far behind the
  // work, which had ended long before, so Cancel seemed to do nothing (#135). The log keeps its
  // last megabytes.
  events.EventsOn("say", text => {
    if (state.mode === "info") return; // Details keeps its own cards, no report under the list
    // a NUP given to a work that does not take it: a hint about Combine (once for the files dropped together), then the
    // file with "->"; the lines wrap under their text, not under their mark
    const hang = line => {
      const div = document.createElement("div");
      div.className = "log-hang";
      div.textContent = line + "\n";
      return div;
    };
    const at = text.indexOf(": this work leaves NUP files as they are");
    if (at >= 0) {
      const now = Date.now();
      if (now - nupHintAt > 3000) said.push(hang("** " + t("nupCombineHint")));
      nupHintAt = now;
      said.push(hang("-> " + text.slice(0, at + 1) + "\n" + noteText(text.slice(at + 2))));
    } else {
      said.push(noteText(text) + "\n");
    }
    if (sayFrame) return;
    sayFrame = setTimeout(() => {
      sayFrame = 0;
      const log = $("log");
      log.hidden = false;
      log.append(...said);
      said = [];
      while (log.childNodes.length > 1 && log.textContent.length > logKept) log.firstChild.remove();
      log.scrollTop = log.scrollHeight;
    }, 100);
  });
  // a measure puts the setting it chose in the fields, kept with the settings
  events.EventsOn("measured", event => {
    $("threads").value = event.threads;
    $("multi").value = event.multi;
    $("preset").value = "";
    save("settings", settings());
    // the fields changed are shown: Advanced opens, and they light up for a moment
    $("advanced").open = true;
    $("threads").scrollIntoView({ block: "center", behavior: "smooth" });
    for (const field of [$("threads"), $("multi")]) {
      field.classList.remove("changed");
      void field.offsetWidth;
      field.classList.add("changed");
    }
    showMeasure(event);
  });
  events.EventsOn("finished", event => {
    state.busy = false;
    state.paused = false;
    for (const row of state.rows.values()) if (row.status === "waiting" || row.status === "work") { row.status = "failed"; row.error = "cancelled"; row.run = state.lastRun && state.lastRun.started; }
    const single = onePack(state.lastRun);
    const total = single || (state.lastRun && state.lastRun.mode === "measure") ? 1 : state.lastRun && !state.lastRun.watch ? state.lastRun.request.files.length : state.items.length;
    // the files skipped by this work, not the ones left so by the work before
    const skippedRows = single || !state.lastRun ? 0 : state.lastRun.skipped;
    const status = event.cancelled ? t("stopped") : t("finished")(total, event.failed, event.seconds, skippedRows);
    setStatus(event.cancelled ? { key: "stopped" } : { key: "finished", args: [total, event.failed, event.seconds, skippedRows] });
    if (state.lastRun) state.lastRun.status = status;
    const logged = keepWork(event, status);
    if (event.cancelled || !event.failed) save("pending", null);
    // long work tells it is done, and turns the computer off when asked
    if (!event.cancelled && event.seconds >= 20) app().Notify("NXPress", status);
    if (!event.cancelled && $("sound").checked) playSound();
    if (!event.cancelled && $("when-done").value && !(state.lastRun && state.lastRun.watch)) startShutdown();
    // the taskbar: red while there are errors to look at, else nothing
    app().SetTaskbar(!event.cancelled && event.failed ? "error" : "none", 1, 1);
    // what this work saved goes to the space saved since always
    if (!event.cancelled && state.lastRun && ["compress", "recompress", "trim", "pack"].includes(state.lastRun.mode)) {
      const ran = [...state.rows.values()].filter(row => row.run === state.lastRun.started && row.status === "ok" && row.after > 0 && row.item.size > row.after);
      keepSaved(ran.reduce((sum, row) => sum + row.item.size - row.after, 0), ran.length);
    }
    // the end of the work: the space saved counts up, and the bar glows when nothing failed
    state.celebrate = !event.cancelled;
    render();
    if (!event.cancelled && !event.failed) replay("glow", document.querySelector(".actions"));
    if (!($("when-done").value && !(state.lastRun && state.lastRun.watch))) showDone(event, status);
    // the files the Explorer gave meanwhile come in now; the new version installed opens once
    // they did, the work they start is over too and its log is written (#91)
    const waiting = state.openedWaiting.splice(0);
    setTimeout(async () => {
      for (const opened of waiting) await receiveOpened(opened);
      await logged;
      if (state.restartAfterWork) restartUpdated();
    }, 0);
  });
  // the computer did not turn off or hibernate: told in the window and by a notice, with what can help
  events.EventsOn("shutdownFailed", reason => {
    hideShutdown();
    const after = reason ? ` (${reason})` : "";
    if (state.powerAction === "hibernate") {
      setStatus({ key: "hibernateFailed", after });
      app().Notify("NXPress", t("hibernateFailed") + after);
      return;
    }
    setStatus({ key: "shutdownFailed", args: [$("shutdown-force").checked], after });
    app().Notify("NXPress", t("shutdownFailed")($("shutdown-force").checked) + after);
  });
  // NXPress called the turning off off by itself (a work started): its count goes
  events.EventsOn("shutdownCancelled", hideShutdown);
  // the window is closed with files at work: the page asks, in the theme of the window (#115);
  // No is the answer of Escape and of the button in focus
  events.EventsOn("closeAsked", () => {
    const dialog = $("close-dialog");
    if (dialog.open) return;
    $("close-yes").onclick = () => { dialog.close(); app().ConfirmClose(); };
    $("close-no").onclick = () => dialog.close();
    openDialog(dialog);
    $("close-no").focus();
  });
  events.OnFileDrop((x, y, paths) => { if (!state.busy) refill(paths, false); }, true);
}

/// the log of the works (#69): each work is kept as it ends, by NXPress in the folder of its
// settings with a text file of it, to read what was done again later: above all after the
// computer was turned off when it was done, when the log shows by itself the next time
const worklogKeeps = [100, 500, 1000, 0];
const worklogKeep = () => { const keep = load("worklogKeep", 500); return worklogKeeps.includes(keep) ? keep : 500; };
const workName = mode => mode === "measure" ? t("measureTitle") : mode === "estimate" ? t("estimateButton") : t(mode) || mode;
// a measure and an estimate compress: they wear the tile of Compress
const workTile = mode => icons[mode] ? tile(mode, (groups.find(([, members]) => members.includes(mode)) || ["groupCheck"])[0]) : tile("compress", "groupSpace");
const isWork = value => isReport(value) && Number.isFinite(value.when) && typeof value.mode === "string";
function keepWork(event, status) {
  const run = state.lastRun;
  const started = run && run.started;
  const mode = run ? run.mode : state.mode;
  const request = (run && run.request) || {};
  const ran = [...state.rows.values()].filter(row => started && row.run === started && ["ok", "failed", "modified", "skipped"].includes(row.status));
  // a work stopped before it did anything leaves nothing to read
  if (!ran.length && event.cancelled) return;
  const rows = ran.map(row => ({
    name: row.game && row.game.name ? `${row.game.name} · ${row.item.name}` : row.item.name,
    status: row.status === "failed" && row.error === "cancelled" ? "cancelled" : row.status,
    damaged: damaged(row.status, row.error),
    note: noteText(row.status === "failed" ? (row.error === "cancelled" ? "" : row.error) : row.note),
  }));
  // what the files written saved, as the space saved since always counts it
  const saved = ["compress", "recompress", "trim", "pack"].includes(mode) ? ran.filter(row => row.status === "ok" && row.after > 0 && row.item.size > row.after).reduce((sum, row) => sum + row.item.size - row.after, 0) : 0;
  const detail = ["compress", "recompress", "estimate", "measure"].includes(mode) && request.level ? t("worklogLevel")(request.level, !!request.block)
    : mode === "verify" && ["full", "signature", "both"].includes(request.verifyLevel) ? t("verify" + request.verifyLevel[0].toUpperCase() + request.verifyLevel.slice(1)) : "";
  // whole numbers: NXPress keeps them as such
  const work = { when: Date.now(), mode, cancelled: !!event.cancelled, status, detail, seconds: Math.round(event.seconds || 0), saved: Math.round(saved), rows, text: "" };
  work.text = workText(work);
  return app().AddToLog(work, worklogKeep()).then(works => { state.worklog = (works || []).filter(isWork); if (state.view === "worklog") render(); }).catch(() => { });
}
// how a work reads in the text file of the log: as the window shows it, in its language
function workText(work) {
  const when = new Date(work.when);
  const two = n => String(n).padStart(2, "0");
  const date = `${when.getFullYear()}-${two(when.getMonth() + 1)}-${two(when.getDate())} ${two(when.getHours())}:${two(when.getMinutes())}`;
  const lines = [[date, workName(work.mode), work.detail, `NXPress ${state.setup.version}`].filter(Boolean).join(" · ")];
  lines.push([work.status, work.saved > 0 ? t("worklogSaved")(size(work.saved)) : ""].filter(Boolean).join(" "));
  const words = work.rows.map(row => stateWord(row.status, work.mode, row.damaged));
  const width = Math.max(0, ...words.map(word => word.length)) + 2;
  work.rows.forEach((row, i) => lines.push(`  ${words[i].padEnd(width)}${row.name}${row.note ? " — " + row.note : ""}`));
  return lines.join("\n");
}
// the log as NXPress keeps it, and the last report of an older NXPress put in it the first time
async function loadWorklog() {
  try {
    state.worklog = (await app().Log() || []).filter(isWork);
    state.worklogFile = await app().LogFile() || "";
  } catch { state.worklog = []; }
  const older = load("lastReport", null, isReport);
  if (older && Number.isFinite(older.when) && older.rows.length && !state.worklog.length) {
    const work = { when: older.when, mode: typeof older.mode === "string" ? older.mode : "", cancelled: !!older.cancelled, status: typeof older.status === "string" ? older.status : "", detail: "", seconds: 0, saved: 0, rows: older.rows, text: "" };
    work.text = workText(work);
    try { state.worklog = (await app().AddToLog(work, worklogKeep()) || []).filter(isWork); } catch { }
  }
  try { localStorage.removeItem("nxpress.lastReport"); } catch { }
}
// the log takes the place of the list, and its settings the one of the options; the newest work
// is open, to read it at once
function openWorklog() {
  if (state.busy || state.starting || state.view === "worklog") return;
  state.view = "worklog";
  const newest = state.worklog[state.worklog.length - 1];
  state.worklogOpen = new Set(newest ? [newest.when] : []);
  render();
  replay("enter", document.querySelector("#work .page-head"), $("worklog-list"), $("settings"));
}
// the day of a work, as the log groups them: today, yesterday, then the day of the week
function workDay(when) {
  const day = new Date(when), today = new Date();
  const days = Math.round((new Date(today.getFullYear(), today.getMonth(), today.getDate()) - new Date(day.getFullYear(), day.getMonth(), day.getDate())) / 86400000);
  const year = day.getFullYear() === today.getFullYear() ? {} : { year: "numeric" };
  const date = day.toLocaleDateString(state.language, { day: "numeric", month: "long", ...year });
  if (days === 0) return `${t("worklogToday")} · ${date}`;
  if (days === 1) return `${t("worklogYesterday")} · ${date}`;
  return day.toLocaleDateString(state.language, { weekday: "long", day: "numeric", month: "long", ...year });
}
// the works and the files the log shows: the ones with problems only, when asked, and the ones
// with what is looked for
function worklogFilter() {
  const find = $("worklog-find").value.trim().toLowerCase();
  const problems = !!state.worklogProblems;
  const matches = row => !find || row.name.toLowerCase().includes(find) || (row.note || "").toLowerCase().includes(find);
  return { find, problems, workShown: work => (!problems || work.rows.some(row => row.status !== "ok")) && work.rows.some(matches), rowShown: row => (!problems || row.status !== "ok") && matches(row) };
}
function paintWorklog() {
  const { find, problems, workShown, rowShown } = worklogFilter();
  for (const button of $("worklog-filter").querySelectorAll("button")) {
    const on = !!button.dataset.problems === problems;
    button.classList.toggle("on", on);
    button.setAttribute("aria-checked", on);
  }
  const works = state.worklog.slice().reverse();
  $("worklog-count").textContent = works.length ? t("worklogCount")(works.length, works.reduce((sum, work) => sum + work.rows.length, 0)) : "";
  for (const option of $("worklog-keep").options) if (option.value !== "0") option.textContent = t("worklogKeepN")(Number(option.value));
  $("worklog-keep").value = String(worklogKeep());
  $("worklog-file").textContent = state.worklogFile;
  $("worklog-file").title = state.worklogFile;
  $("worklog-open").disabled = $("worklog-clear").disabled = !works.length;
  $("worklog-show").hidden = !works.length;
  const list = $("worklog-list");
  list.innerHTML = "";
  const shown = works.filter(workShown);
  $("worklog-export").disabled = !shown.length;
  // Expand all and Collapse all act on the works shown, each off when it has nothing to do
  $("worklog-expand").hidden = !shown.length;
  $("worklog-expand-all").onclick = () => {
    for (const work of shown) state.worklogOpen.add(work.when);
    state.worklogFindClosed = false;
    paintWorklog();
  };
  $("worklog-collapse-all").onclick = () => {
    for (const work of shown) state.worklogOpen.delete(work.when);
    state.worklogFindClosed = !!find;
    paintWorklog();
  };
  if (!shown.length) {
    list.innerHTML = `<div class="worklog-empty"><svg class="empty-icon" viewBox="0 0 24 24" aria-hidden="true">${icons.worklog}</svg><div class="big"></div><div class="muted"></div></div>`;
    list.querySelector(".big").textContent = t(works.length ? "worklogNone" : "worklogEmpty");
    list.querySelector(".muted").textContent = t(works.length ? "worklogNoneHint" : "worklogEmptyHint");
    return;
  }
  const order = { failed: 0, cancelled: 1, modified: 2, skipped: 3, ok: 4 };
  let day = "";
  for (const work of shown) {
    const label = workDay(work.when);
    if (label !== day) {
      day = label;
      const head = document.createElement("div");
      head.className = "worklog-day";
      head.textContent = label;
      list.append(head);
    }
    const open = state.worklogOpen.has(work.when) || (!!find && !state.worklogFindClosed);
    const item = document.createElement("div");
    item.className = "worklog-work" + (open ? " open" : "");
    item.innerHTML = `<button class="worklog-row" aria-expanded="${open}"><span class="worklog-time"></span>${workTile(work.mode)}<span class="worklog-what"><b></b><small></small></span><span class="worklog-counts"></span><span class="worklog-saved"></span><svg class="worklog-chevron" viewBox="0 0 24 24" aria-hidden="true"><path d="m9 6 6 6-6 6"/></svg></button><div class="worklog-files"><table><tbody></tbody></table><button class="link small-link worklog-copy"></button></div>`;
    const clock = new Date(work.when).toLocaleTimeString(state.language, { hour: "2-digit", minute: "2-digit" });
    item.querySelector(".worklog-time").textContent = clock;
    item.querySelector(".worklog-what b").textContent = workName(work.mode);
    // on a narrow window the hour goes before the rest, where the column of the hours is gone
    item.querySelector(".worklog-what small").textContent = [t("worklogFiles")(work.rows.length), work.detail, work.seconds ? time(work.seconds) : "", work.cancelled ? t("worklogCancelled") : ""].filter(Boolean).join(" · ");
    item.querySelector(".worklog-what small").dataset.clock = clock;
    // how its files ended: a check counts its damaged files apart, as Corrupted
    const counts = {};
    for (const row of work.rows) { const key = work.mode === "verify" && row.damaged ? "corrupted" : row.status; counts[key] = (counts[key] || 0) + 1; }
    for (const status of ["ok", "modified", "skipped", "corrupted", "failed", "cancelled"]) {
      if (!counts[status]) continue;
      const badge = document.createElement("span");
      badge.className = "badge " + (status === "cancelled" ? "waiting" : status === "corrupted" ? "failed" : status);
      badge.textContent = `${status === "corrupted" ? stateWord("failed", work.mode, true) : stateWord(status, work.mode)} ${counts[status]}`;
      item.querySelector(".worklog-counts").append(badge);
    }
    item.querySelector(".worklog-saved").textContent = work.saved > 0 ? "−" + size(work.saved) : "";
    // its files, the problems first, and only the ones looked for
    const rows = work.rows.filter(rowShown).sort((a, b) => order[a.status] - order[b.status]);
    for (const row of rows) {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td><div class="report-name"></div><div class="report-note"></div></td><td class="state"><span class="badge"></span></td>`;
      tr.querySelector(".report-name").textContent = row.name;
      tr.querySelector(".report-note").textContent = row.note || "";
      const badge = tr.querySelector(".badge");
      badge.classList.add(row.status === "cancelled" ? "waiting" : row.status);
      badge.textContent = stateWord(row.status, work.mode, row.damaged);
      item.querySelector("tbody").append(tr);
    }
    const copy = item.querySelector(".worklog-copy");
    copy.textContent = t("worklogCopy");
    copy.onclick = () => { window.runtime.ClipboardSetText(work.text); showToast(t("worklogCopied")); };
    const button = item.querySelector(".worklog-row");
    button.onclick = () => {
      if (state.worklogOpen.has(work.when)) state.worklogOpen.delete(work.when); else state.worklogOpen.add(work.when);
      const now = state.worklogOpen.has(work.when);
      item.classList.toggle("open", now);
      button.setAttribute("aria-expanded", now);
      syncWorklogExpand();
    };
    list.append(item);
  }
  syncWorklogExpand();
}
// Expand all is off when every work shown is open, Collapse all when none is
function syncWorklogExpand() {
  const works = [...$("worklog-list").querySelectorAll(".worklog-work")];
  $("worklog-expand-all").disabled = !works.some(work => !work.classList.contains("open"));
  $("worklog-collapse-all").disabled = !works.some(work => work.classList.contains("open"));
}

// the turning off, with 60 seconds to keep the computer on
// the sound when the work is done (#32): a chime of NXPress, or a sound file chosen in the menu,
// which NXPress reads since the page cannot open the files of the computer
let soundKept = { path: "", url: "" };
async function playSound() {
  const path = load("soundFile", "");
  if (path) {
    try {
      if (soundKept.path !== path) soundKept = { path, url: await app().Sound(path) };
      await new Audio(soundKept.url).play();
      return;
    } catch {
      addStatus({ key: "soundFailed" });
    }
  }
  chime();
}
// two short notes, made here rather than kept as a file
function chime() {
  const audio = new AudioContext();
  for (const [frequency, at] of [[880, 0], [1318.5, 0.16]]) {
    const note = audio.createOscillator(), volume = audio.createGain();
    note.frequency.value = frequency;
    volume.gain.setValueAtTime(0.0001, audio.currentTime + at);
    volume.gain.exponentialRampToValueAtTime(0.25, audio.currentTime + at + 0.02);
    volume.gain.exponentialRampToValueAtTime(0.0001, audio.currentTime + at + 0.6);
    note.connect(volume).connect(audio.destination);
    note.start(audio.currentTime + at);
    note.stop(audio.currentTime + at + 0.65);
  }
  setTimeout(() => audio.close(), 1500);
}
function renderSound() {
  const path = load("soundFile", "");
  $("sound-row").hidden = !$("sound").checked;
  $("sound-name").textContent = path ? path.split(/[\\/]/).pop() : t("soundChime");
  $("sound-name").title = path;
  $("sound-default").hidden = !path;
}

let shutdownTimer = 0;
function startShutdown() {
  // the log shows by itself the next time NXPress opens
  save("showReport", true);
  state.powerAction = $("when-done").value;
  if (state.powerAction === "hibernate") app().HibernateIn(60);
  else app().ShutDownIn(60, $("shutdown-force").checked);
  state.shutdownAt = Date.now() + 60000;
  $("shutdown-banner").hidden = false;
  const tick = () => {
    const left = Math.max(0, Math.round((state.shutdownAt - Date.now()) / 1000));
    $("shutdown-text").textContent = t(state.powerAction === "hibernate" ? "hibernateIn" : "shutdownIn")(left);
    if (left > 0) shutdownTimer = setTimeout(tick, 1000);
  };
  tick();
}
// the count of the turning off goes away: it was called off here or by NXPress, or it failed
function hideShutdown() {
  clearTimeout(shutdownTimer);
  state.shutdownAt = 0;
  $("shutdown-banner").hidden = true;
  // the computer stays on: a restart that waited for the count happens now (#91)
  if (state.restartAfterWork) setTimeout(restartUpdated, 0);
}
// the computer stays on: with the button, or with a new work started during the count
function keepOn() {
  save("showReport", false);
  app().CancelShutDown();
  hideShutdown();
}

// the files of the list that have what is looked for in their name, the name of their game, their
// title ID, their folder or their note (#105); the others are left out of sight, not of the work
function applyListFind() {
  const find = $("list-find").value.trim().toLowerCase();
  let shown = 0;
  for (const row of state.rows.values()) {
    const game = row.game || {};
    const text = [shownName(row), row.item.name, row.item.path, game.titleId || "", game.gameId || "", row.tr.querySelector(".note")?.textContent || ""].join(" ").toLowerCase();
    const found = !find || text.includes(find);
    row.tr.classList.toggle("find-out", !found);
    if (found) shown++;
  }
  $("list-found").textContent = find ? t("listFound")(shown, state.rows.size) : "";
}

// a program that is not an official version tells it in the side menu (see TRADEMARKS.md), as
// GitHub answers when the window asks for updates
function paintOfficial() {
  const unofficial = state.official === "unofficial";
  $("unofficial").hidden = !unofficial;
  $("unofficial").textContent = t("unofficialBuild");
  $("unofficial").title = t("unofficialBuildHint");
}

async function checkUpdate(asked = false) {
  if (!$("updates").checked && !asked) return;
  app().Official().then(official => { state.official = official && official.state; paintOfficial(); }).catch(() => {});
  const update = await app().CheckUpdate();
  if (asked) {
    // Check now (#106): the new version again, though it was closed, or that this is the newest
    if (update && update.version) save("dismissedUpdate", "");
    $("check-now-status").textContent = !update || !update.checked ? t("checkNowFailed") : update.version ? "" : t("checkNowLatest")(state.setup.version);
    $("check-now-status").classList.toggle("ok", !!(update && update.checked && !update.version));
  }
  if (!update || !update.version || load("dismissedUpdate", "") === update.version) return;
  // kept, to tell it again in another language
  state.update = update;
  $("update-get").onclick = () => app().OpenURL(update.url);
  $("update-close").onclick = () => { save("dismissedUpdate", update.version); $("update-banner").hidden = true; };
  $("update-banner").hidden = false;
  paintUpdate();
}

// the notice of a new version (#91): one signed that NXPress can install opens the window of the
// update; another offers Download, as before; one installed already waits to be opened
function paintUpdate() {
  const update = state.update;
  if (!update) return;
  const installed = state.installed === update.version;
  let text = t(installed ? "updateInstalled" : "updateFound")(update.version);
  if (!installed && !update.installable && update.reason === "write") text += " " + t("updateNoWrite");
  $("update-text").textContent = text;
  $("update-install").hidden = !update.installable;
  $("update-install").textContent = t(installed ? "updateRestart" : "updateInstall");
  $("update-get").hidden = update.installable;
  paintUpdateDialog();
}

// the window of the update: what is new, then how far it is, what kept it from it, or Restart now
function paintUpdateDialog() {
  const update = state.update;
  if (!update) return;
  const going = state.installing;
  const failed = !going && state.installError;
  const ready = !going && !failed && state.installed === update.version;
  $("update-version").textContent = update.version;
  let step = "", percent = "";
  if (going && going.step === "download") {
    step = t("updateDownloading")(update.version, going.total > 0 ? `${size(going.done)} / ${size(going.total)}` : size(going.done));
    if (going.total > 0) percent = Math.floor(going.done / going.total * 100) + " %";
  } else if (going) step = t(going.step === "check" ? "updateChecking" : going.step === "restart" ? "updateRestarting" : "updateReplacing");
  else if (ready) step = t("updateReady")(update.version);
  $("update-step").textContent = step;
  $("update-percent").textContent = ready ? "✓" : percent;
  $("update-progress").hidden = !going && !ready;
  $("update-bar").classList.toggle("done", ready);
  $("update-bar").firstElementChild.style.width = going && going.step === "download" && going.total > 0 ? Math.min(100, going.done / going.total * 100).toFixed(1) + "%" : going ? "100%" : "";
  $("update-error").hidden = !failed;
  $("update-error").textContent = failed ? t("updateError")(failed) : "";
  // in Program Files, Windows asks for the administrator to put it in place
  $("update-hint").textContent = ready ? t("updateListBack") : failed ? "" : t("updateSigned") + (state.update.admin ? " " + t("updateAdmin") : "");
  $("update-cancel").hidden = !(going && going.step === "download");
  $("update-dialog-close").hidden = !failed;
  $("update-page").hidden = !failed;
  $("update-later").hidden = !ready;
  $("update-restart").hidden = !ready;
  $("update-restart").disabled = state.restartAfterWork;
  $("update-restart").textContent = t(state.restartAfterWork ? "updateRestartWaiting" : restartWaits(state, !!startTimer) ? "updateRestartLater" : "updateRestart");
}

// the notes of a version, as GitHub keeps them (the section of the CHANGELOG): its headings, its
// lists and its paragraphs, written as text, `code` as code, links by their text only
function renderNotes(notes, element) {
  element.innerHTML = "";
  const fill = (block, text) => {
    for (const part of text.replace(/\[([^\]]+)\]\([^)]*\)/g, "$1").replace(/\*\*/g, "").split(/(`[^`]+`)/)) {
      if (/^`[^`]+`$/.test(part)) {
        const code = document.createElement("code");
        code.textContent = part.slice(1, -1);
        block.append(code);
      } else if (part) block.append(part);
    }
  };
  let list = null;
  for (const raw of (notes || "").split(/\r?\n/)) {
    const line = raw.trim();
    if (!line) { list = null; continue; }
    if (/^[-*] /.test(line)) {
      if (!list) { list = document.createElement("ul"); element.append(list); }
      const item = document.createElement("li");
      fill(item, line.slice(2));
      list.append(item);
      continue;
    }
    list = null;
    const block = document.createElement(/^#{1,6} /.test(line) ? "h3" : "p");
    fill(block, line.replace(/^#{1,6} /, ""));
    element.append(block);
  }
}

// installs the new version in place of this one, in its window: NXPress keeps working meanwhile,
// and the new one opens with Restart now, or the next time NXPress starts
async function installUpdate() {
  const update = state.update;
  if (!update) return;
  renderNotes(update.notes, $("update-notes"));
  if (state.installed === update.version) { paintUpdate(); openDialog($("update-dialog")); return; }
  state.installError = "";
  state.installing = { step: "download", done: 0, total: 0 };
  paintUpdate();
  openDialog($("update-dialog"));
  try {
    await app().InstallUpdate();
    state.installed = update.version;
  } catch (error) {
    const code = String(error).split(":")[0].trim();
    state.installError = code === "cancelled" ? "" : code;
    if (code === "cancelled") $("update-dialog").close();
  }
  state.installing = null;
  paintUpdate();
}

// what the restart into the new version waits for (#91): the work going on (a watch of folders
// too), one about to start, the files of the Explorer still to come in, and a count to turn the
// computer off (which, when it happens, opens the new version the next time)
function restartWaits(state, startPending) {
  return !!(state.busy || state.starting || startPending || state.openedWaiting.length || state.shutdownAt);
}

// opens the version installed with the list as it is; while anything goes on, once it is over
let restartRetry = 0;
async function restartUpdated() {
  clearTimeout(restartRetry);
  if (restartWaits(state, !!startTimer)) { state.restartAfterWork = true; paintUpdate(); return; }
  state.restartAfterWork = false;
  state.installing = { step: "restart", done: 0, total: 0 };
  paintUpdate();
  try {
    await app().RestartUpdated(state.mode, state.sources.length ? state.sources : state.items.map(item => item.path));
  } catch (error) {
    state.installing = null;
    const code = String(error).split(":")[0].trim();
    // NXPress still had something going on: the restart waits for it, and asks again
    if (code === "work") {
      state.restartAfterWork = true;
      paintUpdate();
      if (!restartWaits(state, !!startTimer)) restartRetry = setTimeout(restartUpdated, 2000);
      return;
    }
    state.installError = code;
    paintUpdate();
    if (!$("update-dialog").open) openDialog($("update-dialog"));
  }
}

// watchDrops shows the layer of a drop (show(true, count)) while files are dragged over the
// window (target), and ends it when they are dropped or the drag leaves it. On macOS NXPress takes
// the drop itself and the page gets neither a drop nor a leave: the files the system hands over
// (wails:file-drop of runtime) end it too, and so does the mouse moving on the layer, as no drag
// is going on then; the layer would else stay over the window, and nothing could be clicked (#46).
//
// Whatever is dragged over the window, the browser of the window never takes it for itself: it
// would open the file in a window of Edge and download it (#120). Files reach NXPress by its own
// drop, and nothing else is dropped in it.
function watchDrops(target, runtime, layer, busy, show) {
  let dragging = 0;
  const end = () => { dragging = 0; show(false); };
  const keep = event => { if (event && event.preventDefault) event.preventDefault(); };
  target.addEventListener("dragover", keep, true);
  target.addEventListener("drop", keep, true);
  target.addEventListener("dragenter", event => {
    if (busy() || !event.dataTransfer || ![...event.dataTransfer.types].includes("Files")) return;
    dragging++;
    show(true, event.dataTransfer.items ? event.dataTransfer.items.length : 0);
  });
  target.addEventListener("dragleave", () => { dragging = Math.max(0, dragging - 1); if (!dragging) show(false); });
  target.addEventListener("drop", end);
  runtime.EventsOn("wails:file-drop", end);
  layer.addEventListener("mousemove", end);
}

async function main() {
  fillFields();
  state.setup = await app().Setup();
  state.keys = state.setup.keys;
  // the theme: the one of the system, or the one chosen
  state.theme = load("theme", "auto", value => ["auto", "light", "dark"].includes(value));
  // how the files of Combine and Split are chosen: a switch, or a round check on the icon
  state.pickStyle = load("pickStyle", "switch", value => ["switch", "check"].includes(value));
  document.documentElement.dataset.pickStyle = state.pickStyle;
  // the × that closes the card of a game: off unless the person turns it on
  state.popClose = load("popClose", "off", value => ["on", "off"].includes(value));
  document.documentElement.dataset.popClose = state.popClose;
  // on Windows 11 the menu shows the Mica of the system, in the theme chosen
  document.documentElement.classList.toggle("mica", !!state.setup.mica);
  const theme = () => {
    if (state.theme === "auto") document.documentElement.removeAttribute("data-theme");
    else document.documentElement.dataset.theme = state.theme;
    if (state.setup.mica && window.runtime) {
      if (state.theme === "dark") window.runtime.WindowSetDarkTheme();
      else if (state.theme === "light") window.runtime.WindowSetLightTheme();
      else window.runtime.WindowSetSystemDefaultTheme();
    }
  };
  theme();
  // the colors: NXPress's own, or the ones chosen in Settings
  state.palette = load("palette", "nxpress", value => palettes.some(palette => palette[0] === value));
  applyPalette();
  for (const button of document.querySelectorAll("button[data-pop-close]")) button.onclick = () => { state.popClose = button.dataset.popClose; save("popClose", state.popClose); document.documentElement.dataset.popClose = state.popClose; render(); };
  for (const button of document.querySelectorAll("button[data-pick-style]")) button.onclick = () => { state.pickStyle = button.dataset.pickStyle; save("pickStyle", state.pickStyle); document.documentElement.dataset.pickStyle = state.pickStyle; render(); };
  for (const button of document.querySelectorAll("button[data-theme]")) button.onclick = () => { state.theme = button.dataset.theme; save("theme", state.theme); theme(); render(); };
  // the language: the one chosen, else the one chosen in the installer of Windows, else the one
  // of the system when NXPress has it
  const system = navigator.language.toLowerCase().slice(0, 2);
  const installed = state.setup.language || "";
  state.language = load("language", texts[installed] ? installed : texts[system] ? system : "en");
  if (!texts[state.language]) state.language = "en";
  for (const [code, name] of languages) if (texts[code]) $("language-select").append(new Option(name, code));
  // another language tells again what is shown: the new version and the games to combine too
  $("language-select").onchange = () => { state.language = $("language-select").value; save("language", state.language); render(); previewPacks(); };
  state.mode = modes.includes(load("mode", "")) ? load("mode", "") : "compress";
  if (!sideToolShown(state.mode)) state.mode = firstSideMode() || state.mode;
  const chosen = load("settings", {});
  // a field of the settings kept is taken only when it is of its kind
  const number = value => Number.isInteger(value) && value > 0 ? value : 0;
  const text = value => typeof value === "string" ? value : "";
  $("level").max = state.setup.maxLevel;
  $("level").value = number(chosen.level) || state.setup.level;
  $(chosen.block ? "target-block" : "target-solid").checked = true;
  $("threads").value = number(chosen.threads) || state.setup.threads;
  state.restorePreset = load("preset", "");
  // the log is read, and shown when the computer was turned off at the end of the last work
  const showLog = load("showReport", false);
  save("showReport", false);
  loadWorklog().then(() => { if (showLog) setTimeout(openWorklog, 300); else if (state.view === "worklog") render(); });
  // Advanced is open unless it was closed, and stays as it was left
  $("advanced").open = load("advancedOpen", true);
  $("advanced").ontoggle = () => save("advancedOpen", $("advanced").open);
  $("threads").max = state.setup.cpus;
  $("multi").value = number(chosen.multi) || 1;
  $("overwrite").checked = !!chosen.overwrite;
  $("remove").checked = !!chosen.removeSource;
  $("remove-dumps").checked = !!chosen.removeDumps;
  // on unless it was turned off: Eglantine and Lichii read the big files of a NUP at once so
  $("nup-raw-big").checked = chosen.nupRawBig !== false;
  $("watch").checked = !!chosen.watch;
  $("no-cache").checked = !!chosen.noCache;
  $("par2").checked = chosen.par2 > 0;
  $("par2-percent").value = String([1, 3, 5, 10].includes(chosen.par2Percent) ? chosen.par2Percent : 3);
  // Verify repairs with the recovery data unless it was turned off
  $("repair").checked = chosen.repair !== false;
  state.verifyLevel = ["full", "signature", "both"].includes(chosen.verifyLevel) ? chosen.verifyLevel : "full";
  $("keep-named").checked = !!chosen.keepNamed;
  $("low-priority").checked = !!chosen.lowPriority;
  // experimental, and off unless it was turned on
  $("extra").checked = !!(chosen.sparse || chosen.archive);
  // experimental too, and not with the extra compression
  $("optimize").checked = !!chosen.optimize && !$("extra").checked;
  $("skip-sparse-updates").checked = !!chosen.skipSparseUpdates;
  $("updates").checked = load("updates", true);
  $("published").checked = load("published", true);
  $("sound").checked = load("sound", false);
  const sort = load("sort", null);
  if (sort && ["", "name", "size", "state"].includes(sort.key)) state.sort = { key: sort.key, dir: sort.dir < 0 ? -1 : 1 };
  state.output = text(chosen.output);
  state.dat = text(chosen.dat);
  state.template = text(chosen.template) || await app().DefaultTemplate();
  $("template").value = state.template;
  // a pack per game unless one file with everything was chosen last
  $(chosen.packGames === false ? "pack-one" : "pack-games").checked = true;
  // Convert into NSP and NSZ unless into game cards was chosen last
  $(chosen.toCard === true ? "convert-to-card" : "convert-to-nsp").checked = true;
  state.packTemplate = migratedPackTemplate(text(chosen.packTemplate)) || await app().DefaultPackTemplate();
  $("pack-template").value = state.packTemplate;

  $("level").oninput = () => { $("preset").value = ""; render(); };
  document.querySelectorAll("[data-level]").forEach(button => { button.onclick = () => { $("level").value = button.dataset.level; $("preset").value = ""; save("settings", settings()); render(); }; });
  // a field of the panel is kept as soon as it changes, not only when a work starts: closing
  // NXPress without working keeps it too, in step with the settings saved with a name
  for (const type of ["input", "change"]) $("settings").addEventListener(type, () => save("settings", settings()));
  $("keys").onclick = async () => {
    try { const path = await app().ChooseKeys(); if (path) { state.keys = path; for (const row of state.rows.values()) describe(row); } } catch (error) { setStatus(String(error)); }
    render();
  };
  // the files and the folder added are kept as origins of the list, to take them again in another
  // section; the folder also to watch it
  const addFiles = async () => {
    if (state.busy) return;
    const items = await app().ChooseFiles(state.mode) || [];
    addOrigins(items.map(item => item.path));
    for (const item of items) add(item);
    if (state.mode === "rename") previewNames();
    previewPacks();
    render();
  };
  const addFolder = async () => {
    if (state.busy) return;
    // the folder dialog of Save to gives the folder chosen itself, not only the files of it this
    // section takes: one in a subfolder would else stand for it
    const folder = await app().ChooseOutput();
    if (!folder) return;
    const items = await app().Expand([folder], expandMode()) || [];
    addOrigins([folder]);
    for (const item of items) add(item);
    if (state.mode === "rename") previewNames();
    previewPacks();
    render();
  };
  $("add-files").onclick = $("empty-files").onclick = addFiles;
  $("add-folder").onclick = $("empty-folder").onclick = addFolder;
  $("keys-choose").onclick = $("keys-change").onclick = () => $("keys").click();
  for (const button of document.querySelectorAll("[data-view]")) button.onclick = () => { save("views", { ...load("views", {}), [state.mode]: button.dataset.view }); render(); };
  for (const button of document.querySelectorAll("[data-verify-level]")) button.onclick = () => { state.verifyLevel = button.dataset.verifyLevel; save("settings", settings()); render(); };
  // the settings wait for the end of a work: they would hide its progress, Cancel and Pause
  $("open-preferences").onclick = () => { if (state.busy || state.starting) return; if (state.view !== "preferences") { state.view = "preferences"; render(); replay("enter", $("preferences")); } };
  $("remove").onchange = $("remove-dumps").onchange = $("watch").onchange = $("par2").onchange = render;
  $("skip-sparse-updates").onchange = () => save("settings", settings());
  // the experimental option tells first, in detail, what it changes: it is on only once that
  // is accepted, and stays off when the window is closed or Esc is pressed
  $("extra").onchange = () => {
    if (!$("extra").checked) return render();
    $("extra").checked = false;
    // the extra compression makes files installers do not read, which Optimize for install is for
    if ($("optimize").checked) setOptimize(false);
    save("settings", settings());
    $("extra-dialog").showModal();
    $("extra-cancel").focus();
  };
  $("extra-on").onclick = () => {
    $("extra").checked = true;
    $("extra-dialog").close();
    save("settings", settings());
    render();
  };
  $("extra-cancel").onclick = () => $("extra-dialog").close();
  wirePop();
  // Optimize for install tells first what it changes, as the extra compression; the list is made
  // again for the files it takes (NSZ and XCZ too) when it is turned on or off
  const setOptimize = on => {
    $("optimize").checked = on;
    if (on && $("extra").checked) $("extra").checked = false;
    save("settings", settings());
    refill(state.sources.length ? state.sources : state.items.map(item => item.path), true, true);
  };
  $("optimize").onchange = () => {
    if (!$("optimize").checked) return setOptimize(false);
    $("optimize").checked = false;
    $("optimize-dialog").showModal();
    $("optimize-cancel").focus();
  };
  $("optimize-on").onclick = () => {
    $("optimize-dialog").close();
    setOptimize(true);
  };
  $("optimize-cancel").onclick = () => $("optimize-dialog").close();
  $("clear").onclick = () => {
    if (state.busy) return;
    const items = state.items.slice(), sources = state.sources.slice(), removed = [...state.removed];
    clearList();
    render();
    if (items.length) takenOut(items, "", () => { addOrigins(sources); for (const path of removed) state.removed.add(path); });
  };
  $("toast-undo").onclick = undo;
  app().SetTaskbar("none", 0, 0);
  $("done-close").onclick = () => $("done-dialog").close();
  const repository = "https://github.com/DeLFuS77/NXPress";
  $("star-yes").onclick = () => { save("starAsked", true); $("done-star").hidden = true; app().OpenURL(repository); };
  $("star-no").onclick = () => { save("starAsked", true); $("done-star").hidden = true; };
  $("star-link").onclick = () => app().OpenURL(repository);
  $("unofficial").onclick = () => app().OpenURL(repository + "/releases/latest");
  $("export").onclick = event => exportMenu(event.currentTarget, exportDetails, true);
  $("worklog-export").onclick = event => exportMenu(event.currentTarget, exportLog);
  // the server NXPress shares with Switch Library Manager Web (#86): its invite does not expire
  $("discord-link").onclick = () => app().OpenURL("https://discord.gg/acQVZSEBR");
  $("space-cancel").onclick = () => $("space-dialog").close();
  // the window to search an image of a NUP in the eShop (#102), and an image pasted (Ctrl+V)
  $("cover-search").onclick = searchCovers;
  $("cover-query").addEventListener("keydown", event => { if (event.key === "Enter") { event.preventDefault(); searchCovers(); } });
  pairSystems("cover", searchCovers);
  $("cover-use").onclick = useCover;
  $("cover-cancel").onclick = () => $("cover-dialog").close();
  $("cover-web").onclick = () => app().SearchCoverOnWeb($("cover-query").value);
  $("cover-dialog").addEventListener("close", () => { cover.round++; });
  document.addEventListener("paste", pasteIcon);
  // 2: a drop answers before it lands: the whole window says how many files it takes
  watchDrops(window, window.runtime, $("drop-overlay"), () => state.busy, (show, count) => {
    if (show) $("drop-text").textContent = t("dropOverlay")(count);
    $("drop-overlay").hidden = !show;
    if (show) replay("enter", $("drop-overlay").firstElementChild);
  });
  // 7: the keys of the window
  // the card over a game and the menu of a file go with Escape and with a scroll; the menu also
  // with a click out of it, and its arrows move between its items
  window.addEventListener("scroll", () => { hidePop(); hideRowMenu(); }, true);
  window.addEventListener("blur", hideRowMenu);
  window.addEventListener("mousedown", event => { if (!$("row-menu").contains(event.target)) hideRowMenu(); });
  $("row-menu").addEventListener("keydown", event => {
    const buttons = [...$("row-menu").querySelectorAll("button:not(:disabled)")];
    const at = buttons.indexOf(document.activeElement);
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      buttons[(at + (event.key === "ArrowDown" ? 1 : buttons.length - 1)) % buttons.length]?.focus();
    }
  });
  window.addEventListener("keydown", event => {
    if (event.key === "Escape") { hidePop(); hideRowMenu(); }
    if (!event.ctrlKey || event.altKey) return;
    const key = event.key.toLowerCase();
    const typing = /^(input|textarea|select)$/i.test(document.activeElement.tagName) && document.activeElement.type !== "checkbox";
    if (key === "o" && !state.busy) { event.preventDefault(); (event.shiftKey ? addFolder : addFiles)(); }
    else if (key === "enter") { event.preventDefault(); start(); }
    else if (key === ",") { event.preventDefault(); if (!state.busy) $("open-preferences").click(); }
    else if (key === "z" && !typing && undoLast) { event.preventDefault(); undo(); }
  });
  $("choose-output").onclick = async () => { const path = await app().ChooseOutput(); if (path) state.output = path; save("settings", settings()); render(); previewPacks(); };
  $("reset-output").onclick = () => { state.output = ""; save("settings", settings()); render(); previewPacks(); };
  $("choose-dat").onclick = async () => { const path = await app().ChooseDat(); if (path) state.dat = path; save("settings", settings()); render(); };
  $("reset-dat").onclick = () => { state.dat = ""; save("settings", settings()); render(); };
  for (const radio of [$("pack-games"), $("pack-one")]) radio.onchange = () => { save("settings", settings()); render(); previewPacks(); };
  // the list is made again for the files of the way chosen, as for Optimize for install
  for (const radio of [$("convert-to-nsp"), $("convert-to-card")]) radio.onchange = () => {
    save("settings", settings());
    refill(state.sources.length ? state.sources : state.items.map(item => item.path), true, true);
  };
  $("overwrite").addEventListener("change", previewPacks);
  for (const field of [$("template"), $("pack-template")]) {
    field.addEventListener("input", () => fitTemplate(field));
    field.addEventListener("keydown", event => { if (event.key === "Enter") event.preventDefault(); });
  }
  $("pack-template").oninput = () => { state.packTemplate = $("pack-template").value; save("settings", settings()); previewPacks(); };
  $("pack-template-reset").onclick = async () => { state.packTemplate = await app().DefaultPackTemplate(); $("pack-template").value = state.packTemplate; save("settings", settings()); };
  // the menu of NXPress in the Explorer, in the language of the page (#88), and the program of
  // its files (#87): added when not there or there for another copy, else taken away
  $("explorer-menu").onclick = () => setExplorer(() => app().SetExplorerMenu(state.explorer.menu !== "here", menuTexts()), () => save("menuLanguage", state.language));
  $("explorer-files").onclick = () => setExplorer(() => app().SetExplorerFiles(state.explorer.files !== "here"));
  $("open-start").onchange = () => save("openStart", $("open-start").checked);
  $("send-to").onclick = async () => {
    try {
      await app().SetSendTo(!state.setup.sendToEnabled);
      state.setup.sendToEnabled = !state.setup.sendToEnabled;
    } catch (error) { setStatus(String(error)); }
    render();
  };
  $("check-now").onclick = async () => {
    $("check-now").disabled = true;
    $("check-now-status").classList.remove("ok");
    $("check-now-status").textContent = t("checkNowChecking");
    try {
      await checkUpdate(true);
    } finally {
      $("check-now").disabled = false;
    }
  };
  $("list-find").oninput = applyListFind;
  $("updates").onchange = () => { save("updates", $("updates").checked); if ($("updates").checked) checkUpdate(); render(); };
  // a new version (#91): its window, which installs it, and what it offers then
  $("update-install").onclick = installUpdate;
  $("update-cancel").onclick = () => app().CancelUpdate();
  $("update-restart").onclick = restartUpdated;
  $("update-later").onclick = () => $("update-dialog").close();
  $("update-dialog-close").onclick = () => $("update-dialog").close();
  $("update-page").onclick = () => { if (state.update) app().OpenURL(state.update.url); };
  // Esc cancels the download, and leaves the window open until it has stopped
  $("update-dialog").addEventListener("cancel", event => {
    if (state.installing) { event.preventDefault(); if (state.installing.step === "download") app().CancelUpdate(); }
  });
  // turned off, the versions published are forgotten, and the copy kept removed
  $("published").onchange = () => {
    save("published", $("published").checked);
    if ($("published").checked) loadVersions();
    else {
      app().ForgetVersions();
      state.versionsDate = null;
      state.published.clear();
      paintPublished();
      warnSoon();
    }
  };
  $("sound").onchange = () => { save("sound", $("sound").checked); renderSound(); };
  // Low priority is in Settings, out of the side panel that saves itself (#60)
  $("low-priority").onchange = () => save("settings", settings());
  $("sound-choose").onclick = async () => { const path = await app().ChooseSound(); if (path) { save("soundFile", path); renderSound(); playSound(); } };
  $("sound-play").onclick = playSound;
  $("sound-default").onclick = () => { save("soundFile", ""); renderSound(); chime(); };
  // a click on a column sorts the list by it (see sortRows); Enter or Space on the button of its
  // header click it too
  for (const th of document.querySelectorAll("#files th[data-sort]")) th.onclick = () => {
    const { key, dir } = state.sort;
    state.sort = key !== th.dataset.sort ? { key: th.dataset.sort, dir: 1 } : dir > 0 ? { key, dir: -1 } : { key: "", dir: 1 };
    save("sort", state.sort);
    sortRows();
  };
  // the box of the header chooses all the files shown, or none; it does not sort the column
  $("pick-all").parentElement.onclick = event => event.stopPropagation();
  $("pick-all").onchange = () => {
    const off = offOf(), left = !$("pick-all").checked;
    for (const row of state.rows.values()) if (!row.item.result && !row.tr.classList.contains("find-out")) { if (left) off.add(row.item.path); else off.delete(row.item.path); }
    pickChanged();
  };
  $("start").onclick = start;
  $("estimate").onclick = () => start("estimate");
  $("compare-levels").onclick = () => start("estimate", { levels: true });
  $("measure").onclick = () => start("measure");
  $("measure-close").onclick = () => $("measure-dialog").close();
  // the log: what is looked for, the works with problems only, the most kept, its text file, and
  // clearing it, which Undo takes back
  $("worklog-find").oninput = () => { state.worklogFindClosed = false; paintWorklog(); };
  $("worklog-filter").onclick = event => {
    const button = event.target.closest("button");
    if (!button) return;
    state.worklogProblems = !!button.dataset.problems;
    paintWorklog();
  };
  $("worklog-keep").onchange = async () => {
    const keep = Number($("worklog-keep").value);
    save("worklogKeep", keep);
    try { state.worklog = (await app().KeepLog(keep) || []).filter(isWork); } catch { }
    render();
  };
  $("worklog-open").onclick = () => app().OpenLog().catch(() => { });
  $("worklog-show").onclick = () => app().ShowFile(state.worklogFile);
  $("worklog-clear").onclick = async () => {
    const kept = state.worklog;
    try { await app().ClearLog(); } catch { return; }
    state.worklog = [];
    render();
    showToast(t("worklogCleared"), async () => {
      try { await app().WriteLog(kept); state.worklog = kept; } catch { }
      render();
    });
  };
  $("pause").onclick = () => {
    state.paused = !state.paused;
    app().SetPaused(state.paused);
    // the taskbar says it at once: no progress comes while the work waits to tell it
    const { done, total } = workDone();
    app().SetTaskbar(state.paused ? "paused" : "normal", done, total);
    render();
  };
  $("report").onclick = () => { window.runtime.ClipboardSetText(report()); setStatus({ key: "reportCopied" }); };
  $("when-done").onchange = () => render();
  $("shutdown-cancel").onclick = keepOn;
  $("preset").onchange = applyPreset;
  $("preset-name").oninput = paintPresetName;
  // saved settings of the same name are replaced where they are, and Undo brings them back (#93)
  $("preset-save").onclick = () => {
    const name = $("preset-name").value.trim();
    if (!name) { $("preset-name").focus(); return; }
    const found = presetNameState();
    if (found.kind === "builtin") return;
    const before = savedPresets();
    const presets = before.slice();
    const now = settings();
    const preset = { name, level: now.level, block: now.block, threads: now.threads, multi: now.multi };
    if (found.kind === "replace") presets[found.index] = preset;
    else presets.push(preset);
    save("presets", presets);
    $("preset-name").value = "";
    render();
    $("preset").value = String(builtinPresets().length + (found.kind === "replace" ? found.index : presets.length - 1));
    render();
    if (found.kind === "replace") showToast(t("presetReplaced")(name), () => { save("presets", before); render(); });
  };
  $("preset-delete").onclick = () => {
    const index = +$("preset").value - builtinPresets().length;
    const presets = savedPresets();
    if (index >= 0) { presets.splice(index, 1); save("presets", presets); }
    $("preset").value = "";
    render();
  };
  $("template").oninput = () => { state.template = $("template").value; save("settings", settings()); previewNames(); };
  $("keep-named").onchange = () => { save("settings", settings()); previewNames(); };
  $("template-reset").onclick = async () => { state.template = await app().DefaultTemplate(); $("template").value = state.template; save("settings", settings()); previewNames(); };
  listen();
  setupHelp();

  // the files left unfinished last time
  const pending = load("pending", null, isPending);
  if (pending && pending.request && pending.request.files && pending.request.files.length) {
    state.pendingCount = pending.request.files.length;
    $("resume-banner").hidden = false;
    $("resume-go").onclick = async () => {
      $("resume-banner").hidden = true;
      state.pendingCount = 0;
      state.mode = pending.mode;
      await refill(pending.request.files, true);
      render();
    };
    $("resume-drop").onclick = () => { save("pending", null); state.pendingCount = 0; $("resume-banner").hidden = true; };
  }
  // the places of NXPress in the Explorer, on Windows (#87, #88)
  if (state.setup.sendTo) state.explorer = await app().ExplorerPlaces().catch(() => state.explorer);
  // the files NXPress was started with: dropped on it, sent to it, or opened from the Explorer
  // with a double click or its menu, in the section asked (#87, #88)
  if (state.setup.files && state.setup.files.length) {
    state.mode = state.setup.mode;
    await refill(state.setup.files, true);
    state.openedAt = Date.now();
    state.openedCount = state.setup.files.length;
    if (state.setup.open && load("openStart", false)) startSoon();
  }
  render();
  // the ones given to other NXPress while this one was opening
  takeOpened();
  checkUpdate();
  loadVersions();
}

main();
