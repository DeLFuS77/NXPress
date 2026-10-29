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
  theme: "auto", mode: "compress", language: "en", items: [], rows: new Map(), busy: false, keys: "", output: "", pack: "", dat: "",
  setup: null, sources: [], paused: false, starting: false, lastRun: null, shutdownAt: 0, template: "", sort: { key: "", dir: 1 }, skipGames: new Set(), verifyLevel: "full",
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

// the space saved since always, kept in the window: what every work that wrote files saved
// in the units the window shows sizes in (1 GB = 1024 MB), so they read round
const milestones = [100, 250, 500, 1024, 2048, 5120, 10240].map(gb => gb * 2 ** 30);
function keepSaved(bytes, files) {
  if (bytes <= 0) return;
  const before = load("lifetime", { bytes: 0, files: 0 });
  const after = { bytes: before.bytes + bytes, files: before.files + files };
  save("lifetime", after);
  // crossing a milestone is told once, with the bar of actions glowing
  const crossed = milestones.filter(mark => before.bytes < mark && after.bytes >= mark).pop();
  if (crossed) setTimeout(() => { showToast(t("milestone")(size(crossed))); replay("glow", document.querySelector(".actions")); }, 1600);
}
// the space saved since always, on the empty list; it counts up the first time it shows
let lifetimeShown = false, lifetimeAt = 0;
function renderLifetime() {
  const kept = load("lifetime", { bytes: 0, files: 0 });
  const element = $("lifetime");
  element.hidden = !(kept.bytes > 0);
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
  if (["ok", "failed", "skipped"].includes(row.status)) return 1;
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

// a card for a file in Info: its size beside the one as NSP or XCI, and what it works with
function renderCard(row) {
  row.tr.querySelector(".file-card")?.remove();
  const card = row.card;
  if (!card || state.mode !== "info") return;
  const element = document.createElement("div");
  element.className = "file-card";
  const whole = Math.max(card.decompressed || card.size, card.size);
  const share = whole ? card.size / whole : 1;
  const compressed = card.solid + card.blocks > 0;
  const works = card.solid > 0 ? { DBI: 1, Tinfoil: 1, Awoo: 1, CyberFoil: 1, Sphaira: 1, emulators: 0 }
    : card.blocks > 0 ? { DBI: 1, Tinfoil: 1, Awoo: 0, CyberFoil: 1, Sphaira: 1, emulators: 1 }
    : { DBI: 1, Tinfoil: 1, Awoo: 1, CyberFoil: 1, Sphaira: 1, emulators: 1 };
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
  row.tr.querySelector(".note").after(element);
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
function showPop(row, anchor) {
  const game = row.game;
  if (!game || !game.titleId || !anchor.isConnected) return;
  const pop = $("game-pop");
  const image = pop.querySelector("img");
  image.hidden = !game.icon;
  if (game.icon) image.src = game.icon;
  pop.querySelector(".pop-name").textContent = shownName(row);
  pop.querySelector(".pop-publisher").textContent = game.publisher || "";
  const facts = [
    ["Title ID", game.titleId],
    [t("popKind"), t("gameKind")(game.type) + (game.demo ? " · " + t("popDemo") : "")],
    [t("popVersion"), (game.versionTxt ? game.versionTxt + " · " : "") + "v" + game.latest],
  ];
  if (game.titles > 1) facts.push([t("popHolds"), t("popTitles")(game.bases, game.updates, game.dlcs)]);
  if (game.gameId) facts.push([t("popGame"), game.gameId]);
  if (game.needsGame) facts.push([t("popNeedsGame"), "v" + game.needsGame]);
  if (game.firmware) facts.push([t("popFirmware"), game.firmware]);
  const list = pop.querySelector("dl");
  list.innerHTML = "";
  for (const [name, value] of facts) {
    const term = document.createElement("dt");
    const detail = document.createElement("dd");
    term.textContent = name;
    detail.textContent = value;
    list.append(term, detail);
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
  let left, top;
  if ($("files").classList.contains("grid")) {
    left = box.right + margin + width < innerWidth ? box.right + margin : box.left - margin - width;
    top = box.top;
  } else {
    left = box.left + 44;
    top = box.bottom + 4 + height < innerHeight - margin ? box.bottom + 4 : box.top - 4 - height;
  }
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

// the menu of a file, with a right click: show it in its folder, copy its title ID, its name or
// its path, check only it, take it out of the list
function showRowMenu(row, remove, x, y) {
  hidePop();
  const menu = $("row-menu");
  const copy = text => { window.runtime.ClipboardSetText(text); showToast(t("copied")(text)); };
  const items = [
    ["menuShow", () => app().ShowFile(row.output || row.item.path)],
    row.game && row.game.titleId && ["menuCopyId", () => copy(row.game.titleId)],
    ["menuCopyName", () => copy(shownName(row))],
    ["menuCopyPath", () => copy(row.item.path)],
    /\.(nsp|nsz|xci|xcz)$/i.test(row.item.path) && ["menuVerify", () => start("verify", { files: [row.item.path], removeSource: false, output: "", watch: false }), true],
    ["menuRemove", remove, true],
  ].filter(Boolean);
  menu.innerHTML = "";
  for (const [key, action, needsIdle] of items) {
    const button = document.createElement("button");
    button.setAttribute("role", "menuitem");
    button.textContent = t(key);
    button.disabled = !!needsIdle && state.busy;
    button.onclick = () => { hideRowMenu(); action(); };
    menu.append(button);
  }
  menu.hidden = false;
  menu.style.left = Math.max(8, Math.min(x, innerWidth - menu.offsetWidth - 8)) + "px";
  menu.style.top = Math.max(8, Math.min(y, innerHeight - menu.offsetHeight - 8)) + "px";
  replay("enter", menu);
  menu.querySelector("button:not(:disabled)")?.focus();
}
function hideRowMenu() {
  $("row-menu").hidden = true;
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
// files taken out of the list come back with Undo, where they were
function takenOut(items, name) {
  const kept = items.slice();
  showToast(kept.length === 1 ? t("toastRemoved")(name || kept[0].name) : t("toastCleared")(kept.length), () => {
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
  // the card wears the color of the game that saved the most
  const best = savings.length ? savings[0].row : rows.find(row => row.game && row.game.icon);
  const tone = best && best.game && coverTone(best.game);
  $("done-dialog").style.setProperty("--done-tone", tone || "var(--ok)");
  // after the third work that saved space, once and never again: a star on GitHub
  const works = load("worksSaved", 0) + (saved > 0 ? 1 : 0);
  save("worksSaved", works);
  $("done-star").hidden = works < 3 || load("starAsked", false);
  $("done-dialog").showModal();
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
// onePack tells a work that writes one file with all the files given: its result is that file
const onePack = run => !!run && run.mode === "pack" && !run.request.packGames;

// stepsOf are the steps of a file in the work running, in their order: the work, then the check
// of what it wrote; a trimmed NSP is checked while it is written, so it has one step
function stepsOf(row) {
  const mode = (state.lastRun && state.lastRun.mode) || state.mode;
  // splitting and joining for FAT32 share their mode: the work is the one this file was doing
  if (row.step && !row.step.startsWith("Verifying")) row.work = row.step.split(" ")[0];
  const work = { compress: "Compressing", recompress: "Recompressing", fat32: row.work || "Splitting" }[mode];
  if (work) return [work, "Verifying"];
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
// than the list brings
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
  for (const row of read) {
    const same = read.filter(other => other !== row && other.game.titleId === row.game.titleId && other.game.type === row.game.type);
    const newer = row.game.type === "UPD" ? same.filter(other => other.game.version > row.game.version).sort((a, b) => b.game.version - a.game.version)[0] : null;
    const game = games.get(row.game.type === "BASE" ? row.game.titleId : row.game.gameId);
    if (newer) warnings.set(row, t("warnNewer")(newer.game.versionTxt || "v" + newer.game.version));
    else if (same.some(other => other.game.version === row.game.version)) warnings.set(row, t("warnTwice"));
    else if (state.mode === "pack" && row.game.type !== "BASE" && row.game.titles === 1 && game && !game.base) warnings.set(row, t("warnNoGame"));
    else if (state.mode === "pack" && row.game.type === "DLC" && row.game.needsGame > 0 && game && game.base && game.reach < row.game.needsGame) warnings.set(row, t("warnNeedsGame")(row.game.needsGame, game.reach));
  }
  return warnings;
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

function render() {
  warnSoon();
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
  // the section in the color of its group, its tile beside its title
  const group = (groups.find(([, members]) => members.includes(state.mode)) || [""])[0];
  $("title-tile").className = `tile tone-${tones[group] || "neutral"}`;
  $("title-tile").innerHTML = icon(state.mode);
  $("work").dataset.tone = tones[group] || "neutral";
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
  // the list as rows, or as a grid of covers
  const view = load("view", "list") === "grid" ? "grid" : "list";
  $("files").classList.toggle("grid", view === "grid");
  for (const button of document.querySelectorAll("[data-view]")) { button.classList.toggle("on", button.dataset.view === view); button.setAttribute("aria-checked", button.dataset.view === view); }
  // what Verify checks: the button chosen, and a sentence for it
  for (const button of document.querySelectorAll("[data-verify-level]")) {
    const on = button.dataset.verifyLevel === state.verifyLevel;
    button.classList.toggle("on", on);
    button.setAttribute("aria-checked", on);
  }
  $("verify-level-hint").textContent = t({ full: "verifyFullHint", signature: "verifySignatureHint", both: "verifyBothHint" }[state.verifyLevel]);
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
  // what the level trades, as a picture: higher levels make smaller files, more slowly
  $("gauge-size").style.width = (100 - (level - 1) * 1.6) + "%";
  $("gauge-speed").style.width = Math.max(8, 100 - (level - 1) * 5.1) + "%";
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
    const ended = row => ["ok", "failed", "skipped"].includes(row.status);
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
  text.textContent = t("selected")(state.items.length, state.items.reduce((sum, item) => sum + item.size, 0));
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

// add puts a file in the list; the new file of a pack (item.result) shows in it, but is no file
// to work on, so it stays out of the files a work is given
function add(item) {
  if (state.rows.has(item.path)) return;
  if (!item.result) state.items.push(item);
  const tr = document.createElement("tr");
  tr.innerHTML = `<td class="name"><div class="game"><img class="icon" alt="" hidden><span class="cover" aria-hidden="true"></span><div><div class="title"></div><div class="file"></div></div></div><div class="note"></div></td><td class="size"></td><td class="state"></td><td class="remove"><button class="quiet">✕</button></td>`;
  // until the icon of the game comes, a cover tells the kind of file
  const kind = (item.path.match(/\.(\w+)$/) || ["", ""])[1].toUpperCase();
  tr.querySelector(".cover").textContent = kind;
  tr.querySelector(".cover").dataset.kind = kind;
  tr.querySelector(".title").textContent = item.name;
  tr.querySelector(".title").title = item.path;
  const game = tr.querySelector(".game");
  game.addEventListener("mouseenter", () => {
    clearTimeout(popTimer);
    popTimer = setTimeout(() => { const row = state.rows.get(item.path); if (row) showPop(row, game); }, 450);
  });
  game.addEventListener("mouseleave", hidePop);
  tr.querySelector(".size").textContent = item.size ? size(item.size) : "";
  const remove = () => {
    if (state.busy) return;
    hidePop();
    const gone = state.rows.get(item.path);
    state.items = state.items.filter(other => other.path !== item.path);
    state.rows.delete(item.path);
    // the row folds away before it goes, and comes back with Undo
    tr.classList.add("row-out");
    setTimeout(() => { tr.remove(); render(); }, calm() ? 0 : 180);
    takenOut([item], shownName(gone || { item, game: null }));
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
  if (!state.keys) return;
  describeQueue.push(row);
  pumpDescribe();
}
function pumpDescribe() {
  while (describing < 2 && describeQueue.length) {
    const row = describeQueue.shift();
    if (!state.rows.has(row.item.path)) continue;
    describing++;
    app().Describe(row.item.path).then(game => { row.game = game; if (state.sort.key === "name") sortSoon(); }).catch(() => { }).finally(() => { row.described = true; paint(row); warnSoon(); describing--; pumpDescribe(); });
  }
}

function paint(row) {
  const cell = row.tr.querySelector(".state");
  if (row.painted !== undefined && row.painted !== row.status && ["ok", "failed", "skipped"].includes(row.status)) {
    replay("ended-" + row.status, row.tr);
    row.endedAt = performance.now();
  }
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
  // a warning of the list shows until the file is worked on
  let warn = row.tr.querySelector(".list-warn");
  if (row.warn && !row.status) {
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
  row.tr.querySelector(".cover").classList.toggle("loading", !!state.keys && !row.described);
  // in the grid, the cover fills up with the progress of its file
  const game = row.tr.querySelector(".game");
  game.classList.toggle("working", row.status === "work");
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
    cell.firstChild.textContent = t("ok");
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
  } else if (row.status === "failed") {
    cell.innerHTML = `<span class="badge ${row.error === "cancelled" ? "waiting" : "failed"}"></span>`;
    cell.firstChild.textContent = row.error === "cancelled" ? t("cancelled") : t("failed");
  } else {
    cell.innerHTML = "";
  }
  note.className = "note" + (row.status === "failed" ? " bad" : row.status === "skipped" ? " warn" : "");
  note.textContent = noteText(row.status === "failed" ? (row.error === "cancelled" ? "" : row.error) : row.note);
  // an error opens to say what to do, with its detail to copy; it stays open while drawn again
  row.tr.querySelector(".error-more")?.remove();
  if (row.status === "failed" && row.error && row.error !== "cancelled") {
    const more = document.createElement("details");
    more.className = "error-more";
    more.open = !!row.open;
    more.ontoggle = () => { row.open = more.open; };
    more.innerHTML = `<summary></summary><p></p><button class="link small-link"></button>`;
    more.querySelector("summary").textContent = t("errorWhat");
    more.querySelector("p").textContent = t(adviceFor(row.error));
    more.querySelector("button").textContent = t("copyDetail");
    more.querySelector("button").onclick = () => { window.runtime.ClipboardSetText(`${row.item.path}\n${row.error}`); showToast(t("detailCopied")); };
    note.after(more);
  }
  if (!row.status && state.mode === "rename" && row.preview && row.preview !== row.item.name) {
    note.textContent = "→ " + row.preview;
  }
  renderCard(row);
  if (row.output) {
    if (note.textContent) note.append(" · ");
    const link = document.createElement("a");
    link.textContent = t("showFolder");
    link.title = row.output;
    link.onclick = () => app().ShowFile(row.output);
    note.append(link);
  }
}

// rowOf is the row of a file the work tells about, added when the list has none: a file a watch
// found, or the new file of a pack of one file, which is a result and not a file to work on
function rowOf(file) {
  if (!state.rows.has(file)) {
    const run = state.lastRun;
    add({ path: file, name: file.split(/[\\/]/).pop(), size: 0, result: onePack(run) && file === run.request.pack });
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
    overwrite: $("overwrite").checked, removeSource: $("remove").checked, output: state.output, dat: state.dat, verifyLevel: state.verifyLevel,
    watch: $("watch").checked, noCache: $("no-cache").checked, template: state.template, keepNamed: $("keep-named").checked,
    packGames: $("pack-games").checked, packTemplate: state.packTemplate,
    lowPriority: $("low-priority").checked,
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
  // a work is starting: a second press waits for it instead of asking for another
  if (state.starting) return;
  if (state.busy) { app().Cancel(); return; }
  mode = typeof mode === "string" ? mode : state.mode;
  // the work shows on its page, with what it needs or what it does: the settings give way to it
  if (state.view) { state.view = ""; render(); }
  // what the work needs is told by its own mode: Verify only this, in a section that needs no
  // keys or no pack, still needs the keys and no pack
  if (!state.keys && !["recompress", "fat32"].includes(mode)) { $("status").textContent = t("needKeys"); return; }
  if (!state.items.length) { $("status").textContent = t(mode === "measure" ? "measureNeedsGame" : "needFiles"); return; }
  if (mode === "pack" && !$("pack-games").checked && !state.pack) { $("status").textContent = t("needPack"); return; }
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
    if (!files.length) { $("status").textContent = t("measureNeedsGame"); return; }
  }
  const request = { mode, files, pack: state.pack, ...chosen, watch: watches, levels: false, skipGames: [...state.skipGames], ...(extra || {}) };
  files = request.files;
  // Verify only this asks for no watch, whatever the box says
  const watch = !!request.watch;
  // a new work keeps the computer on: the count of the turning off asked by the work before stops
  if (state.shutdownAt) keepOn();
  // while the space is looked at, the work is starting: its buttons wait
  state.starting = true;
  render();
  let allowed = false;
  try { allowed = await spaceAllows(request); } finally { state.starting = false; }
  if (!allowed) { render(); return; }
  // the work is marked as running before NXPress is asked to start it, as its first events (and
  // the end of a quick one) can come before the answer; what changes is put back if it does not
  // start, so another work still running keeps its rows
  const before = { lastRun: state.lastRun, paused: state.paused, pending: load("pending", null), rows: [] };
  // the rows of the work wear its start, to count them and only them
  const started = Date.now();
  const fresh = { status: "waiting", step: "", done: 0, total: 0, note: "", output: "", error: "", after: 0, run: started };
  for (const row of state.rows.values()) if (!measuring && (files.includes(row.item.path) || watch)) {
    before.rows.push([row, Object.fromEntries(Object.keys(fresh).map(key => [key, row[key]]))]);
    Object.assign(row, fresh);
  }
  $("log").textContent = "";
  $("log").hidden = true;
  $("status").textContent = "";
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
    $("status").textContent = String(error);
  }
  render();
}

// spaceAllows tells, before anything is written, the drives where the new files may not fit,
// and starts only when asked to; a file that fills its drive fails, and its original stays
function spaceAllows(request) {
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
      $("space-dialog").showModal();
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
    Object.assign(row, { status: event.error ? "failed" : event.skipped ? "skipped" : "ok", note: event.note || "", output: event.output || "", error: event.error || "", after: event.size || 0 });
    // a file renamed keeps its row, under its new name and path
    if (mode === "rename" && row.output && !event.error) {
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
    if (mode === "info" && !event.error) app().Card(row.item.path).then(card => { row.card = card; renderCard(row); }).catch(() => { });
    trackSpeed();
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
    const single = onePack(state.lastRun);
    const total = single || (state.lastRun && state.lastRun.mode === "measure") ? 1 : state.lastRun && !state.lastRun.watch ? state.lastRun.request.files.length : state.items.length;
    keepReport(event);
    // the files skipped by this work, not the ones left so by the work before
    const skippedRows = single || !state.lastRun ? 0 : state.lastRun.skipped;
    const status = event.cancelled ? t("stopped") : t("finished")(total, event.failed, event.seconds, skippedRows);
    $("status").textContent = status;
    if (state.lastRun) state.lastRun.status = status;
    keepReportStatus(status);
    if (event.cancelled || !event.failed) save("pending", null);
    // long work tells it is done, and turns the computer off when asked
    if (!event.cancelled && event.seconds >= 20) app().Notify("NXPress", status);
    if (!event.cancelled && $("sound").checked) playSound();
    if (!event.cancelled && $("shutdown").checked && !(state.lastRun && state.lastRun.watch)) startShutdown();
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
    if (!($("shutdown").checked && !(state.lastRun && state.lastRun.watch))) showDone(event, status);
  });
  // the computer did not turn off: told in the window and by a notice, with what can help
  events.EventsOn("shutdownFailed", reason => {
    hideShutdown();
    const text = t("shutdownFailed")($("shutdown-force").checked) + (reason ? ` (${reason})` : "");
    $("status").textContent = text;
    app().Notify("NXPress", text);
  });
  // NXPress called the turning off off by itself (a work started): its count goes
  events.EventsOn("shutdownCancelled", hideShutdown);
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
// the count of the turning off goes away: it was called off here or by NXPress, or it failed
function hideShutdown() {
  clearTimeout(shutdownTimer);
  state.shutdownAt = 0;
  $("shutdown-banner").hidden = true;
}
// the computer stays on: with the button, or with a new work started during the count
function keepOn() {
  save("showReport", false);
  app().CancelShutDown();
  hideShutdown();
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
  state.verifyLevel = ["full", "signature", "both"].includes(chosen.verifyLevel) ? chosen.verifyLevel : "full";
  $("keep-named").checked = !!chosen.keepNamed;
  $("low-priority").checked = !!chosen.lowPriority;
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
  for (const button of document.querySelectorAll("[data-view]")) button.onclick = () => { save("view", button.dataset.view); render(); };
  for (const button of document.querySelectorAll("[data-verify-level]")) button.onclick = () => { state.verifyLevel = button.dataset.verifyLevel; save("settings", settings()); render(); };
  // the settings wait for the end of a work: they would hide its progress, Cancel and Pause
  $("open-preferences").onclick = () => { if (state.busy || state.starting) return; if (state.view !== "preferences") { state.view = "preferences"; render(); replay("enter", $("preferences")); } };
  $("remove").onchange = render;
  $("clear").onclick = () => {
    const items = state.items.slice();
    clearList();
    render();
    if (items.length) takenOut(items);
  };
  $("toast-undo").onclick = undo;
  app().SetTaskbar("none", 0, 0);
  $("done-close").onclick = () => $("done-dialog").close();
  const repository = "https://github.com/DeLFuS77/NXPress";
  $("star-yes").onclick = () => { save("starAsked", true); $("done-star").hidden = true; app().OpenURL(repository); };
  $("star-no").onclick = () => { save("starAsked", true); $("done-star").hidden = true; };
  $("star-link").onclick = () => app().OpenURL(repository);
  $("space-cancel").onclick = () => $("space-dialog").close();
  // 2: a drop answers before it lands: the whole window says how many files it takes
  let dragging = 0;
  const dropShown = show => { $("drop-overlay").hidden = !show; if (show) replay("enter", $("drop-overlay").firstElementChild); };
  window.addEventListener("dragenter", event => {
    if (state.busy || !event.dataTransfer || ![...event.dataTransfer.types].includes("Files")) return;
    dragging++;
    const count = event.dataTransfer.items ? event.dataTransfer.items.length : 0;
    $("drop-text").textContent = t("dropOverlay")(count);
    dropShown(true);
  });
  window.addEventListener("dragleave", () => { dragging = Math.max(0, dragging - 1); if (!dragging) dropShown(false); });
  window.addEventListener("drop", () => { dragging = 0; dropShown(false); });
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
  $("shutdown-cancel").onclick = keepOn;
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
