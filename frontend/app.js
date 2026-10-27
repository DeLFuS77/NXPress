"use strict";

const modes = ["compress", "decompress", "recompress", "trim", "verify", "convert", "split", "pack", "info", "fat32", "rename"];
// the menu, by what the modes are for
const groups = [
  ["groupSpace", ["compress", "decompress", "recompress", "trim"]],
  ["groupOrganize", ["pack", "split", "fat32", "convert", "rename"]],
  ["groupCheck", ["verify", "info"]],
];
// what each mode takes, to show it
const takesShown = {
  compress: "NSP XCI", decompress: "NSZ XCZ", recompress: "NSZ XCZ", trim: "XCI NSP/NSZ:takesUpdates", verify: "NSP NSZ XCI XCZ", convert: "XCI XCZ",
  split: "NSP NSZ XCI XCZ", pack: "NSP NSZ XCI XCZ", info: "NSP NSZ XCI XCZ", fat32: "NSP NSZ XCI XCZ", rename: "NSP NSZ XCI XCZ",
};

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
function countUp(element, bytes, format) {
  if (calm()) { element.textContent = format(bytes); return; }
  const start = performance.now(), length = 900;
  const step = now => {
    const share = Math.min(1, (now - start) / length);
    element.textContent = format(bytes * (1 - Math.pow(1 - share, 3)));
    if (share < 1 && element.isConnected) requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
}
const app = () => window.go.main.App;
const state = {
  theme: "auto", mode: "compress", language: "en", items: [], rows: new Map(), busy: false, keys: "", output: "", pack: "", dat: "",
  setup: null, sources: [], paused: false, lastRun: null, shutdownAt: 0, template: "", sort: { key: "", dir: 1 }, skipGames: new Set(),
};

function load(name, fallback) {
  try { const value = localStorage.getItem("nxpress." + name); return value === null ? fallback : JSON.parse(value); } catch { return fallback; }
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
function size(bytes) {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes, unit = 0;
  while (value >= 1024 && unit < units.length - 1) { value /= 1024; unit++; }
  return value.toFixed(unit ? 1 : 0) + " " + units[unit];
}
// the notes of the results, which come in English, in the language chosen
function noteText(note) {
  const own = texts[state.language] && texts[state.language].notes || {};
  const english = texts.en.notes || {};
  for (const [pattern, key] of noteRules) {
    const replacement = own[key] !== undefined ? own[key] : english[key];
    if (replacement !== undefined) note = note.replace(pattern, replacement);
  }
  return note;
}
function stepText(step) {
  const table = texts[state.language] && texts[state.language].steps || {};
  const [first, ...rest] = step.split(" ");
  return [table[first] || first, ...rest].join(" ");
}

function render() {
  document.documentElement.lang = state.language;
  document.querySelectorAll("[data-t]").forEach(element => { element.textContent = t(element.dataset.t); });
  document.querySelectorAll("[data-t-title]").forEach(element => { element.title = t(element.dataset.tTitle); });
  $("modes").innerHTML = "";
  for (const [group, members] of groups) {
    const title = document.createElement("div");
    title.className = "group";
    title.textContent = t(group);
    $("modes").append(title);
    for (const mode of members) {
      const button = document.createElement("button");
      button.innerHTML = tile(mode, group) + "<span><b></b><small></small></span>";
      button.querySelector("b").textContent = t(mode);
      button.querySelector("small").textContent = t("hint" + mode[0].toUpperCase() + mode.slice(1));
      button.className = mode === state.mode && !state.view ? "on" : "";
      // the tile chosen pops; the menu is drawn again while it does, and the pop goes on where it was
      const since = performance.now() - (state.chosenAt || 0);
      if (mode === state.mode && !state.view && since < 300) { button.classList.add("chosen"); button.style.setProperty("--since", -since + "ms"); }
      button.onclick = () => {
        if (state.busy) return;
        const changed = state.view || mode !== state.mode;
        // a section leaves the page of the settings
        if (state.view) { state.view = ""; render(); }
        if (changed) state.chosenAt = performance.now();
        setMode(mode);
        if (changed) { render(); replay("enter", document.querySelector("#work .page-head"), $("drop"), $("settings")); }
      };
      $("modes").append(button);
    }
  }
  document.querySelectorAll("[data-for]").forEach(element => { element.hidden = !element.dataset.for.split(" ").includes(state.mode); });
  // the page of the settings takes the place of the section and its options
  const preferences = state.view === "preferences";
  $("work").hidden = preferences;
  $("settings").hidden = preferences;
  $("preferences").hidden = !preferences;
  $("open-preferences").classList.toggle("on", preferences);
  $("keys-path").textContent = state.keys || t("keys")("");
  $("title").textContent = t(state.mode);
  $("about").textContent = t("about" + state.mode[0].toUpperCase() + state.mode.slice(1));
  $("takes").innerHTML = "";
  for (const entry of takesShown[state.mode].split(" ")) {
    // "NSP/NSZ:takesUpdates": the kinds of file, and which of them it works on
    const [type, which] = entry.split(":");
    const tag = document.createElement("i");
    tag.textContent = which ? `${type} (${t(which)})` : type;
    $("takes").append(tag);
  }
  $("language-select").value = state.language;
  if (state.pendingCount) $("resume-text").textContent = t("resumeFound")(state.pendingCount);
  for (const button of document.querySelectorAll("button[data-theme]")) button.classList.toggle("on", button.dataset.theme === state.theme);
  $("keys-banner").hidden = !!state.keys || state.mode === "recompress" || state.mode === "fat32";
  $("toolbar").hidden = state.items.length === 0;
  $("drop").classList.toggle("filled", state.items.length > 0);
  // the level falls in light (1 to 6), medium (7 to 14) or maximum (15 to 19)
  const level = +$("level").value;
  const band = level <= 6 ? "3" : level <= 14 ? "12" : "19";
  for (const button of document.querySelectorAll("[data-level]")) button.classList.toggle("on", button.dataset.level === band);
  $("remove-warning").hidden = !$("remove").checked;
  $("keys").textContent = t("keys")(state.keys);
  $("keys").title = state.keys;
  $("keys").className = "side-keys" + (state.keys ? "" : " bad");
  $("level-value").textContent = $("level").value;
  $("level-hint").textContent = t("levelHint")(level);
  $("output").textContent = state.output || t("nextTo");
  $("output").title = state.output;
  // a path is cut at its start, to show its end; the sentence shown when there is none is read whole
  $("output").classList.toggle("empty", !state.output);
  $("reset-output").hidden = !state.output;
  $("reset-output").title = t("nextTo");
  // a SuperNSP per game, or one file with everything
  $("pack-one-fields").hidden = $("pack-games").checked;
  $("pack-games-fields").hidden = !$("pack-games").checked;
  for (const field of [$("template"), $("pack-template")]) if (field.offsetParent) fitTemplate(field);
  // one file with everything is saved where it is chosen; a pack per game where Save in says
  if (state.mode === "pack" && !$("pack-games").checked) $("output-field").hidden = true;
  $("pack").textContent = state.pack || t("noPack");
  $("pack").title = state.pack;
  $("pack").classList.toggle("empty", !state.pack);
  $("dat").textContent = state.dat || t("noDat");
  $("dat").title = state.dat;
  $("dat").classList.toggle("empty", !state.dat);
  $("reset-dat").hidden = !state.dat;
  const watching = state.busy && state.lastRun && state.lastRun.watch;
  $("start").textContent = state.busy ? (watching ? t("stop") : t("cancel")) : t("start")(state.mode);
  $("start").className = "primary big" + (state.busy ? " stop" : "");
  $("start").disabled = !state.busy && state.items.length === 0;
  $("estimate").disabled = state.busy || state.items.length === 0;
  $("compare-levels").disabled = state.busy || state.items.length === 0;
  // with no file it still answers, telling what it needs
  $("measure").disabled = state.busy;
  // the computer is named as its system names it
  const system = (state.setup.platform || "").split("/")[0];
  $("measure").textContent = t(system === "darwin" ? "measureMac" : system === "windows" ? "measurePc" : "measureComputer");
  $("pause").hidden = !state.busy;
  $("pause").textContent = state.paused ? t("resume") : t("pause");
  $("report").hidden = state.busy || !state.lastRun || ![...state.rows.values()].some(row => ["ok", "failed", "skipped"].includes(row.status));
  $("shutdown-option").hidden = watching || state.mode === "rename" || state.mode === "info";
  $("shutdown-force-option").hidden = $("shutdown-option").hidden || !$("shutdown").checked;
  $("shutdown-force-option").title = t("shutdownForceHint");
  $("send-to").hidden = !state.setup.sendTo;
  $("send-to").textContent = state.setup.sendToEnabled ? t("sendToOff") : t("sendToOn");
  $("empty").hidden = state.items.length > 0;
  $("files").hidden = state.items.length === 0;
  document.body.classList.toggle("busy", state.busy);
  renderPresets();
  renderSound();
  summary();
  for (const row of state.rows.values()) paint(row);
  sortRows();
}

// summary tells what the list holds, how far the work is, and what it saved.
function summary() {
  const rows = [...state.rows.values()];
  const text = $("summary-text");
  const bar = $("total-bar");
  text.textContent = "";
  bar.hidden = true;
  if (state.busy) {
    const ended = row => ["ok", "failed", "skipped"].includes(row.status);
    const done = rows.filter(ended).length;
    const share = rows.reduce((sum, row) => sum + (ended(row) ? 1 : row.total ? row.done / row.total : 0), 0) / Math.max(1, rows.length);
    text.textContent = (state.paused ? t("paused") + " · " : "") + t("working")(done, rows.length, Math.floor(share * 100));
    if (state.lastRun && state.lastRun.watch) text.textContent = t("watching") + " · " + text.textContent;
    bar.hidden = false;
    bar.firstElementChild.style.width = (share * 100).toFixed(1) + "%";
    return;
  }
  if (!rows.length) {
    text.textContent = t("nothing");
    return;
  }
  text.textContent = t("selected")(rows.length, state.items.reduce((sum, item) => sum + item.size, 0));
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

function setMode(mode) {
  if (state.busy || mode === state.mode) return;
  state.mode = mode;
  save("mode", mode);
  // the files the new mode does not take leave the list
  refill(state.sources.length ? state.sources : state.items.map(item => item.path), true);
  render();
}

// refill adds what the paths given hold for the mode; the folders are kept, to watch them.
async function refill(paths, replace) {
  const items = paths.length ? await app().Expand(paths, state.mode) : [];
  if (replace) clearList();
  for (const path of paths) if (!state.sources.includes(path)) state.sources.push(path);
  for (const item of items) add(item);
  if (state.mode === "rename") previewNames();
  previewPacks();
  render();
}

function clearList() {
  state.items = [];
  state.rows.clear();
  state.sources = [];
  state.lastRun = null;
  $("files").tBodies[0].innerHTML = "";
  $("log").textContent = "";
  $("log").hidden = true;
  $("status").textContent = "";
}

function add(item) {
  if (state.rows.has(item.path)) return;
  state.items.push(item);
  const tr = document.createElement("tr");
  tr.innerHTML = `<td class="name"><div class="game"><img class="icon" alt="" hidden><span class="cover" aria-hidden="true"></span><div><div class="title"></div><div class="file"></div></div></div><div class="note"></div></td><td class="size"></td><td class="state"></td><td class="remove"><button class="quiet">✕</button></td>`;
  // until the icon of the game comes, a cover tells the kind of file
  const kind = (item.path.match(/\.(\w+)$/) || ["", ""])[1].toUpperCase();
  tr.querySelector(".cover").textContent = kind;
  tr.querySelector(".cover").dataset.kind = kind;
  tr.querySelector(".title").textContent = item.name;
  tr.querySelector(".title").title = item.path;
  tr.querySelector(".size").textContent = item.size ? size(item.size) : "";
  tr.querySelector(".remove button").onclick = () => {
    if (state.busy) return;
    state.items = state.items.filter(other => other.path !== item.path);
    state.rows.delete(item.path);
    // the row folds away before it goes
    tr.classList.add("row-out");
    setTimeout(() => { tr.remove(); render(); }, calm() ? 0 : 180);
  };
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
const stateOrder = { work: 0, waiting: 1, failed: 2, skipped: 3, ok: 4 };
function sortValue(row, key) {
  if (key === "size") return row.item.size;
  if (key === "state") return stateOrder[row.status] ?? 5;
  return (row.game && row.game.name) || row.item.name;
}
function sortRows() {
  const { key, dir } = state.sort;
  for (const th of document.querySelectorAll("#files th[data-sort]")) {
    th.classList.toggle("sorted", th.dataset.sort === key);
    th.classList.toggle("desc", th.dataset.sort === key && dir < 0);
  }
  if (state.busy && key === "state") return;
  const rows = [...state.rows.values()];
  if (key) rows.sort((a, b) => {
    const x = sortValue(a, key), y = sortValue(b, key);
    return dir * (typeof x === "string" ? x.localeCompare(y, undefined, { numeric: true, sensitivity: "base" }) : x - y);
  });
  const body = $("files").tBodies[0];
  if (rows.some((row, i) => body.rows[i] !== row.tr)) for (const row of rows) body.append(row.tr);
  if (!state.busy) state.items = rows.map(row => row.item);
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
  if (!state.keys) return;
  describeQueue.push(row);
  pumpDescribe();
}
function pumpDescribe() {
  while (describing < 2 && describeQueue.length) {
    const row = describeQueue.shift();
    if (!state.rows.has(row.item.path)) continue;
    describing++;
    app().Describe(row.item.path).then(game => { row.game = game; paint(row); if (state.sort.key === "name") sortSoon(); }).catch(() => { }).finally(() => { describing--; pumpDescribe(); });
  }
}

function paint(row) {
  const cell = row.tr.querySelector(".state");
  if (row.painted !== undefined && row.painted !== row.status && ["ok", "failed", "skipped"].includes(row.status)) replay("ended-" + row.status, row.tr);
  row.painted = row.status;
  const note = row.tr.querySelector(".note");
  const title = row.tr.querySelector(".title");
  const file = row.tr.querySelector(".file");
  const icon = row.tr.querySelector(".icon");
  if (row.game && row.game.name) {
    title.textContent = row.game.name;
    file.textContent = `${row.game.titleId} · v${row.game.version} · ${row.game.type} · ${row.item.name}`;
  } else {
    // the name of the file without its [title ID][version] tags, which go under it
    const tags = row.item.name.match(/(\s*\[[^\]]*\])+\s*$/);
    title.textContent = tags ? row.item.name.slice(0, tags.index).trim() || row.item.name : row.item.name;
    file.textContent = row.game ? `${row.game.titleId} · v${row.game.version} · ${row.game.type}` : tags ? tags[0].trim() : "";
  }
  if (row.game && row.game.icon) {
    icon.src = row.game.icon;
    icon.hidden = false;
    row.tr.querySelector(".cover").hidden = true;
  }
  if (row.status === "work") {
    const share = row.total ? row.done / row.total : 0;
    cell.innerHTML = `<div class="step"></div><div class="bar"><div></div></div>`;
    cell.querySelector(".step").textContent = `${state.paused ? t("paused") : stepText(row.step)} · ${Math.floor(share * 100)}%`;
    cell.querySelector(".bar div").style.width = (share * 100).toFixed(1) + "%";
  } else if (row.status === "waiting") {
    cell.innerHTML = `<span class="badge waiting"></span>`;
    cell.firstChild.textContent = t("waiting");
  } else if (row.status === "ok") {
    cell.innerHTML = `<span class="badge ok"></span>`;
    cell.firstChild.textContent = t("ok");
  } else if (row.status === "skipped") {
    // left as it is, with why: neither done nor failed
    cell.innerHTML = `<span class="badge skipped"></span>`;
    cell.firstChild.textContent = t("skipped");
  } else if (row.status === "failed") {
    cell.innerHTML = `<span class="badge ${row.error === "cancelled" ? "waiting" : "failed"}"></span>`;
    cell.firstChild.textContent = row.error === "cancelled" ? t("cancelled") : t("failed");
  } else {
    cell.innerHTML = "";
  }
  note.className = "note" + (row.status === "failed" ? " bad" : row.status === "skipped" ? " warn" : "");
  note.textContent = noteText(row.status === "failed" ? (row.error === "cancelled" ? "" : row.error) : row.note);
  if (!row.status && state.mode === "rename" && row.preview && row.preview !== row.item.name) {
    note.textContent = "→ " + row.preview;
  }
  if (row.output) {
    if (note.textContent) note.append(" · ");
    const link = document.createElement("a");
    link.textContent = t("showFolder");
    link.title = row.output;
    link.onclick = () => app().ShowFile(row.output);
    note.append(link);
  }
}

function rowOf(file) {
  if (!state.rows.has(file)) add({ path: file, name: file.split(/[\\/]/).pop(), size: 0 });
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
    const paths = state.items.map(item => item.path);
    const plan = paths.length && state.keys ? await app().PackPlan(paths, state.output) || [] : [];
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

// the new names of the files, for the template written
let previewTimer = 0;
function previewNames() {
  clearTimeout(previewTimer);
  previewTimer = setTimeout(async () => {
    const paths = state.items.map(item => item.path);
    if (!paths.length || !state.keys) return;
    const names = await app().NewNames(paths, state.template, $("keep-named").checked);
    paths.forEach((path, i) => { const row = state.rows.get(path); if (row) { row.preview = names[i]; paint(row); } });
  }, 300);
}

function settings() {
  return {
    level: +$("level").value, block: $("target-block").checked, threads: +$("threads").value, multi: +$("multi").value,
    overwrite: $("overwrite").checked, removeSource: $("remove").checked, output: state.output, dat: state.dat,
    watch: $("watch").checked, noCache: $("no-cache").checked, template: state.template, keepNamed: $("keep-named").checked,
    packGames: $("pack-games").checked, packTemplate: state.packTemplate,
  };
}

// presets: the settings of a compression kept with a name
const builtinPresets = () => [
  { name: t("presetInstallers"), level: 18, block: false, builtin: true },
  { name: t("presetEmulators"), level: 18, block: true, builtin: true },
  { name: t("presetFast"), level: 3, block: false, builtin: true },
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
  const all = [...builtinPresets(), ...load("presets", [])];
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
}
function applyPreset() {
  const preset = [...builtinPresets(), ...load("presets", [])][+$("preset").value];
  if (!preset) return;
  $("level").value = preset.level;
  $(preset.block ? "target-block" : "target-solid").checked = true;
  if (preset.threads) $("threads").value = preset.threads;
  if (preset.multi) $("multi").value = preset.multi;
  save("settings", settings());
  render();
}

async function start(mode, extra) {
  if (state.busy) { app().Cancel(); return; }
  mode = typeof mode === "string" ? mode : state.mode;
  if (!state.keys && !["recompress", "fat32"].includes(state.mode)) { $("status").textContent = t("needKeys"); return; }
  if (!state.items.length) { $("status").textContent = t(mode === "measure" ? "measureNeedsGame" : "needFiles"); return; }
  if (state.mode === "pack" && !$("pack-games").checked && !state.pack) { $("status").textContent = t("needPack"); return; }
  const chosen = settings();
  save("settings", chosen);
  const watch = chosen.watch && ["compress", "decompress", "recompress", "trim", "verify"].includes(mode);
  // a watch looks at the folders added; the files alone are taken too
  let files = watch ? [...new Set([...state.sources, ...state.items.map(item => item.path)])] : state.items.map(item => item.path);
  // the measure takes the first game it can compress among the NSP and XCI of the list; the
  // one it takes shows its progress, the others keep their state
  const measuring = mode === "measure";
  if (measuring) {
    files = state.items.filter(item => /\.(nsp|xci)$/i.test(item.path)).map(item => item.path);
    if (!files.length) { $("status").textContent = t("measureNeedsGame"); return; }
  }
  for (const row of state.rows.values()) if (!measuring && (files.includes(row.item.path) || watch)) Object.assign(row, { status: "waiting", step: "", done: 0, total: 0, note: "", output: "", error: "", after: 0 });
  $("log").textContent = "";
  $("log").hidden = true;
  $("status").textContent = "";
  const request = { mode, files, pack: state.pack, ...chosen, watch, levels: false, skipGames: [...state.skipGames], ...(extra || {}) };
  try {
    await app().Start(request);
    state.busy = true;
    state.paused = false;
    state.lastRun = { mode, request, started: Date.now(), watch, skipped: 0 };
    // the files left are kept, to go on with them after a close
    if (!watch && ["compress", "decompress", "recompress", "trim", "convert", "fat32"].includes(mode)) save("pending", { mode, request });
  } catch (error) {
    for (const row of state.rows.values()) row.status = "";
    $("status").textContent = String(error);
  }
  render();
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
  $("measure-dialog").showModal();
}

// the report of the last work, to paste in an issue or a thread
function report() {
  const run = state.lastRun || {};
  const r = run.request || {};
  const lines = [
    `NXPress ${state.setup.version} · ${state.setup.platform}`,
    `${run.mode} · level ${r.level} · ${r.block ? "blocks" : "solid"} · threads ${r.threads} · files at once ${r.multi}` + (r.watch ? " · watch" : ""),
    run.status || "",
    "",
  ];
  for (const row of state.rows.values()) {
    const game = row.game ? ` [${row.game.titleId} v${row.game.version} ${row.game.type}${row.game.titles > 1 ? ", " + row.game.titles + " titles" : ""}]` : "";
    lines.push(`- ${row.item.name} (${size(row.item.size)})${game}: ${row.status || "-"}` + (row.error ? ` — ${row.error}` : row.note ? ` — ${row.note}` : ""));
  }
  if ($("log").textContent) lines.push("", $("log").textContent.trim());
  return lines.join("\n");
}

function listen() {
  const events = window.runtime;
  events.EventsOn("progress", event => {
    const row = rowOf(event.file);
    Object.assign(row, { status: "work", step: event.step, done: event.done, total: event.total });
    paint(row);
    summary();
  });
  events.EventsOn("done", event => {
    if (state.mode === "pack" && !(state.lastRun && state.lastRun.request.packGames)) {
      // a pack is one result for all its files
      for (const row of state.rows.values()) if (row.item.path !== event.file) Object.assign(row, { status: event.error ? "" : "ok", run: state.lastRun && state.lastRun.started });
    }
    const row = rowOf(event.file);
    if (event.skipped && state.lastRun) state.lastRun.skipped++;
    // the files this work went through, for its report
    row.run = state.lastRun && state.lastRun.started;
    Object.assign(row, { status: event.error ? "failed" : event.skipped ? "skipped" : "ok", note: event.note || "", output: event.output || "", error: event.error || "", after: event.size || 0 });
    // a file renamed keeps its row, under its new name and path
    if (state.mode === "rename" && row.output && !event.error) {
      const old = row.item.path;
      row.item.name = row.output.split(/[\\/]/).pop();
      row.item.path = row.output;
      state.rows.delete(old);
      state.rows.set(row.output, row);
      state.sources = state.sources.map(path => path === old ? row.output : path);
    }
    const pending = load("pending", null);
    if (pending && !event.error) {
      pending.request.files = pending.request.files.filter(path => path !== event.file);
      save("pending", pending.request.files.length ? pending : null);
    }
    paint(row);
    summary();
  });
  events.EventsOn("say", text => {
    $("log").hidden = false;
    $("log").textContent += noteText(text) + "\n";
    $("log").scrollTop = $("log").scrollHeight;
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
    const onePack = state.mode === "pack" && !(state.lastRun && state.lastRun.request.packGames);
    const total = onePack || (state.lastRun && state.lastRun.mode === "measure") ? 1 : state.items.length;
    keepReport(event);
    // the files skipped by this work, not the ones left so by the work before
    const skippedRows = onePack || !state.lastRun ? 0 : state.lastRun.skipped;
    const status = event.cancelled ? t("stopped") : t("finished")(total, event.failed, event.seconds, skippedRows);
    $("status").textContent = status;
    if (state.lastRun) state.lastRun.status = status;
    keepReportStatus(status);
    if (event.cancelled || !event.failed) save("pending", null);
    // long work tells it is done, and turns the computer off when asked
    if (!event.cancelled && event.seconds >= 20) app().Notify("NXPress", status);
    if (!event.cancelled && $("sound").checked) playSound();
    if (!event.cancelled && $("shutdown").checked && !(state.lastRun && state.lastRun.watch)) startShutdown();
    // the end of the work: the space saved counts up, and the bar glows when nothing failed
    state.celebrate = !event.cancelled;
    render();
    if (!event.cancelled && !event.failed) replay("glow", document.querySelector(".actions"));
  });
  // the computer did not turn off: told in the window and by a notice, with what can help
  events.EventsOn("shutdownFailed", reason => {
    $("shutdown-banner").hidden = true;
    const text = t("shutdownFailed")($("shutdown-force").checked) + (reason ? ` (${reason})` : "");
    $("status").textContent = text;
    app().Notify("NXPress", text);
  });
  events.OnFileDrop((x, y, paths) => { if (!state.busy) refill(paths, false); }, true);
}

// the report of the last work is kept, so what happened can be read again later: above all after
// the computer was turned off when it was done, when it shows by itself the next time
function keepReport(event) {
  const run = state.lastRun && state.lastRun.started;
  const rows = [...state.rows.values()].filter(row => run && row.run === run && ["ok", "failed", "skipped"].includes(row.status)).map(row => ({
    name: row.game && row.game.name ? `${row.game.name} · ${row.item.name}` : row.item.name,
    status: row.status === "failed" && row.error === "cancelled" ? "cancelled" : row.status,
    note: noteText(row.status === "failed" ? (row.error === "cancelled" ? "" : row.error) : row.note),
  }));
  save("lastReport", { when: Date.now(), mode: state.lastRun ? state.lastRun.mode : state.mode, cancelled: !!event.cancelled, rows, status: "", text: "" });
}
function keepReportStatus(status) {
  const kept = load("lastReport", null);
  if (!kept) return;
  kept.status = status;
  kept.text = report();
  save("lastReport", kept);
}
function showReport() {
  const kept = load("lastReport", null);
  $("report-when").textContent = kept ? `${new Date(kept.when).toLocaleString(state.language)} · ${kept.mode === "measure" ? t("measureTitle") : t(kept.mode) || kept.mode}` : "";
  $("report-status").textContent = kept ? kept.status : t("noReport");
  $("report-copy").hidden = !kept;
  // a long report: how many of each, the problems first, and only them when there are some
  const all = kept ? kept.rows : [];
  const order = { failed: 0, cancelled: 1, skipped: 2, ok: 3 };
  const counts = {};
  for (const row of all) counts[row.status] = (counts[row.status] || 0) + 1;
  const problems = all.length - (counts.ok || 0);
  $("report-counts").innerHTML = "";
  for (const status of ["ok", "skipped", "failed", "cancelled"]) {
    if (!counts[status]) continue;
    const badge = document.createElement("span");
    badge.className = "badge " + (status === "cancelled" ? "waiting" : status);
    badge.textContent = `${t(status)} ${counts[status]}`;
    $("report-counts").append(badge);
  }
  $("report-problems-option").hidden = !problems || problems === all.length;
  $("report-problems").checked = problems > 0 && problems < all.length;
  $("report-problems").onchange = () => paintReport(all, order);
  paintReport(all, order);
  $("report-dialog").showModal();
}
function paintReport(all, order) {
  const onlyProblems = $("report-problems").checked;
  const rows = $("report-rows");
  rows.innerHTML = "";
  const shown = all.filter(row => !onlyProblems || row.status !== "ok").sort((a, b) => order[a.status] - order[b.status]);
  for (const row of shown) {
    const tr = document.createElement("tr");
    tr.innerHTML = `<td><div class="report-name"></div><div class="report-note"></div></td><td class="state"><span class="badge"></span></td>`;
    tr.querySelector(".report-name").textContent = row.name;
    tr.querySelector(".report-note").textContent = row.note;
    const badge = tr.querySelector(".badge");
    badge.classList.add(row.status === "cancelled" ? "waiting" : row.status);
    badge.textContent = t(row.status);
    rows.append(tr);
  }
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
      $("status").textContent += ($("status").textContent ? " · " : "") + t("soundFailed");
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
  // the report shows by itself the next time NXPress opens
  save("showReport", true);
  app().ShutDownIn(60, $("shutdown-force").checked);
  state.shutdownAt = Date.now() + 60000;
  $("shutdown-banner").hidden = false;
  const tick = () => {
    const left = Math.max(0, Math.round((state.shutdownAt - Date.now()) / 1000));
    $("shutdown-text").textContent = t("shutdownIn")(left);
    if (left > 0) shutdownTimer = setTimeout(tick, 1000);
  };
  tick();
}

async function checkUpdate() {
  if (!$("updates").checked) return;
  const update = await app().CheckUpdate();
  if (!update || !update.version || load("dismissedUpdate", "") === update.version) return;
  $("update-text").textContent = t("updateFound")(update.version);
  $("update-get").onclick = () => app().OpenURL(update.url);
  $("update-close").onclick = () => { save("dismissedUpdate", update.version); $("update-banner").hidden = true; };
  $("update-banner").hidden = false;
}

async function main() {
  state.setup = await app().Setup();
  state.keys = state.setup.keys;
  // the theme: the one of the system, or the one chosen
  state.theme = load("theme", "auto");
  const theme = () => {
    if (state.theme === "auto") document.documentElement.removeAttribute("data-theme");
    else document.documentElement.dataset.theme = state.theme;
  };
  theme();
  for (const button of document.querySelectorAll("button[data-theme]")) button.onclick = () => { state.theme = button.dataset.theme; save("theme", state.theme); theme(); render(); };
  // the language: the one chosen, else the one of the system when NXPress has it
  const system = navigator.language.toLowerCase().slice(0, 2);
  state.language = load("language", texts[system] ? system : "en");
  if (!texts[state.language]) state.language = "en";
  for (const [code, name] of languages) if (texts[code]) $("language-select").append(new Option(name, code));
  $("language-select").onchange = () => { state.language = $("language-select").value; save("language", state.language); render(); };
  state.mode = modes.includes(load("mode", "")) ? load("mode", "") : "compress";
  const chosen = load("settings", {});
  $("version").textContent = state.setup.version;
  $("level").max = state.setup.maxLevel;
  $("level").value = chosen.level || state.setup.level;
  $(chosen.block ? "target-block" : "target-solid").checked = true;
  $("threads").value = chosen.threads || state.setup.threads;
  state.restorePreset = load("preset", "");
  if (load("showReport", false)) {
    save("showReport", false);
    setTimeout(showReport, 300);
  }
  // Advanced is open unless it was closed, and stays as it was left
  $("advanced").open = load("advancedOpen", true);
  $("advanced").ontoggle = () => save("advancedOpen", $("advanced").open);
  $("threads").max = state.setup.cpus;
  $("multi").value = chosen.multi || 1;
  $("overwrite").checked = !!chosen.overwrite;
  $("remove").checked = !!chosen.removeSource;
  $("watch").checked = !!chosen.watch;
  $("no-cache").checked = !!chosen.noCache;
  $("keep-named").checked = !!chosen.keepNamed;
  $("updates").checked = load("updates", true);
  $("sound").checked = load("sound", false);
  const sort = load("sort", null);
  if (sort && ["", "name", "size", "state"].includes(sort.key)) state.sort = { key: sort.key, dir: sort.dir < 0 ? -1 : 1 };
  state.output = chosen.output || "";
  state.dat = chosen.dat || "";
  state.template = chosen.template || await app().DefaultTemplate();
  $("template").value = state.template;
  // a pack per game unless one file with everything was chosen last
  $(chosen.packGames === false ? "pack-one" : "pack-games").checked = true;
  state.packTemplate = chosen.packTemplate || await app().DefaultPackTemplate();
  $("pack-template").value = state.packTemplate;

  $("level").oninput = () => { $("preset").value = ""; render(); };
  document.querySelectorAll("[data-level]").forEach(button => { button.onclick = () => { $("level").value = button.dataset.level; $("preset").value = ""; render(); }; });
  $("keys").onclick = async () => {
    try { const path = await app().ChooseKeys(); if (path) { state.keys = path; for (const row of state.rows.values()) describe(row); } } catch (error) { $("status").textContent = String(error); }
    render();
  };
  const addFiles = async () => { for (const item of await app().ChooseFiles(state.mode) || []) add(item); if (state.mode === "rename") previewNames(); previewPacks(); render(); };
  const addFolder = async () => {
    const items = await app().ChooseFolder(state.mode) || [];
    for (const item of items) add(item);
    // the folder itself, to watch it, from where its files are
    if (items.length) {
      const folder = items[0].path.replace(/[\\/][^\\/]+$/, "");
      if (!state.sources.includes(folder)) state.sources.push(folder);
    }
    if (state.mode === "rename") previewNames();
    render();
  };
  $("add-files").onclick = $("empty-files").onclick = addFiles;
  $("add-folder").onclick = $("empty-folder").onclick = addFolder;
  $("keys-choose").onclick = $("keys-change").onclick = () => $("keys").click();
  $("open-preferences").onclick = () => { if (state.view !== "preferences") { state.view = "preferences"; render(); replay("enter", $("preferences")); } };
  $("remove").onchange = render;
  $("clear").onclick = () => { clearList(); render(); };
  $("choose-output").onclick = async () => { const path = await app().ChooseOutput(); if (path) state.output = path; save("settings", settings()); render(); previewPacks(); };
  $("reset-output").onclick = () => { state.output = ""; save("settings", settings()); render(); previewPacks(); };
  $("choose-dat").onclick = async () => { const path = await app().ChooseDat(); if (path) state.dat = path; save("settings", settings()); render(); };
  $("reset-dat").onclick = () => { state.dat = ""; save("settings", settings()); render(); };
  for (const radio of [$("pack-games"), $("pack-one")]) radio.onchange = () => { save("settings", settings()); render(); previewPacks(); };
  $("overwrite").addEventListener("change", previewPacks);
  for (const field of [$("template"), $("pack-template")]) {
    field.addEventListener("input", () => fitTemplate(field));
    field.addEventListener("keydown", event => { if (event.key === "Enter") event.preventDefault(); });
  }
  $("pack-template").oninput = () => { state.packTemplate = $("pack-template").value; save("settings", settings()); };
  $("pack-template-reset").onclick = async () => { state.packTemplate = await app().DefaultPackTemplate(); $("pack-template").value = state.packTemplate; save("settings", settings()); };
  $("choose-pack").onclick = async () => { const path = await app().ChoosePack(); if (path) state.pack = path; render(); };
  $("send-to").onclick = async () => {
    try {
      await app().SetSendTo(!state.setup.sendToEnabled);
      state.setup.sendToEnabled = !state.setup.sendToEnabled;
    } catch (error) { $("status").textContent = String(error); }
    render();
  };
  $("updates").onchange = () => { save("updates", $("updates").checked); if ($("updates").checked) checkUpdate(); };
  $("sound").onchange = () => { save("sound", $("sound").checked); renderSound(); };
  $("sound-choose").onclick = async () => { const path = await app().ChooseSound(); if (path) { save("soundFile", path); renderSound(); playSound(); } };
  $("sound-play").onclick = playSound;
  $("sound-default").onclick = () => { save("soundFile", ""); renderSound(); chime(); };
  // a click on a column sorts the list by it (see sortRows)
  for (const th of document.querySelectorAll("#files th[data-sort]")) th.onclick = () => {
    const { key, dir } = state.sort;
    state.sort = key !== th.dataset.sort ? { key: th.dataset.sort, dir: 1 } : dir > 0 ? { key, dir: -1 } : { key: "", dir: 1 };
    save("sort", state.sort);
    sortRows();
  };
  $("start").onclick = start;
  $("estimate").onclick = () => start("estimate");
  $("compare-levels").onclick = () => start("estimate", { levels: true });
  $("measure").onclick = () => start("measure");
  $("measure-close").onclick = () => $("measure-dialog").close();
  $("last-report").onclick = showReport;
  $("report-close").onclick = () => $("report-dialog").close();
  $("report-copy").onclick = () => { const kept = load("lastReport", null); if (kept) window.runtime.ClipboardSetText(kept.text); $("report-dialog").close(); $("status").textContent = t("reportCopied"); };
  $("pause").onclick = () => { state.paused = !state.paused; app().SetPaused(state.paused); render(); };
  $("report").onclick = () => { window.runtime.ClipboardSetText(report()); $("status").textContent = t("reportCopied"); };
  $("shutdown").onchange = () => render();
  $("shutdown-cancel").onclick = () => {
    save("showReport", false); app().CancelShutDown(); clearTimeout(shutdownTimer); $("shutdown-banner").hidden = true; };
  $("preset").onchange = applyPreset;
  $("preset-save").onclick = () => {
    const name = $("preset-name").value.trim();
    if (!name) { $("preset-name").focus(); return; }
    const presets = load("presets", []).filter(preset => preset.name !== name);
    const now = settings();
    presets.push({ name, level: now.level, block: now.block, threads: now.threads, multi: now.multi });
    save("presets", presets);
    $("preset-name").value = "";
    render();
    $("preset").value = String(builtinPresets().length + presets.length - 1);
    render();
  };
  $("preset-delete").onclick = () => {
    const index = +$("preset").value - builtinPresets().length;
    const presets = load("presets", []);
    if (index >= 0) { presets.splice(index, 1); save("presets", presets); }
    $("preset").value = "";
    render();
  };
  $("template").oninput = () => { state.template = $("template").value; save("settings", settings()); previewNames(); };
  $("keep-named").onchange = () => { save("settings", settings()); previewNames(); };
  $("template-reset").onclick = async () => { state.template = await app().DefaultTemplate(); $("template").value = state.template; save("settings", settings()); previewNames(); };
  listen();

  // the files left unfinished last time
  const pending = load("pending", null);
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
  // the files NXPress was started with: dropped on it, or sent to it
  if (state.setup.files && state.setup.files.length) {
    state.mode = state.setup.mode;
    await refill(state.setup.files, true);
  }
  render();
  checkUpdate();
}

main();
