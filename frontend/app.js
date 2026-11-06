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
  compress: "NSP XCI", decompress: "NSZ XCZ NUP", recompress: "NSZ XCZ", trim: "XCI NSP/NSZ:takesUpdates", verify: "NSP NSZ XCI XCZ NUP", convert: "XCI XCZ",
  split: "NSP NSZ XCI XCZ", pack: "NSP NSZ XCI XCZ :takesDumps", info: "NSP NSZ XCI XCZ NUP", fat32: "NSP NSZ XCI XCZ", rename: "NSP NSZ XCI XCZ NUP",
};

// a NUP holds its game decrypted: it is read and verified without keys
const isNup = path => /\.nup$/i.test(path);
// the cover of a dump folder, until its game is read: a folder
const folderIcon = `<svg class="cover-folder" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 7.5V18a1.5 1.5 0 0 0 1.5 1.5h15A1.5 1.5 0 0 0 21 18V9a1.5 1.5 0 0 0-1.5-1.5h-7.2L10.6 5.5H4.5A1.5 1.5 0 0 0 3 7.5z"/></svg>`;
// whether a work needs the keys: not to recompress or for FAT32, nor for a list of NUP files and
// dump folders only
const needsKeys = mode => !["recompress", "fat32"].includes(mode) && !(state.items.length > 0 && state.items.every(item => isNup(item.path) || item.dump));

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
  setup: null, sources: [], removed: new Set(), paused: false, starting: false, lastRun: null, shutdownAt: 0, template: "", sort: { key: "", dir: 1 }, skipGames: new Set(), verifyLevel: "full",
  // the newest version published of each title ID asked (null when the list does not know it),
  // and the day of the list of versions: null until it is read, "" when there is none
  published: new Map(), versionsDate: null,
  // the log of the works, the oldest first, its text file, the works open in it, and whether it
  // shows only the ones with problems (#69)
  worklog: [], worklogFile: "", worklogOpen: new Set(), worklogProblems: false,
  // the works a search opens by itself stay closed after Collapse all, until the search changes
  worklogFindClosed: false,
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

// a card for a file in Info: its size beside the one as NSP or XCI, and what it works with
function renderCard(row) {
  row.tr.querySelector(".file-card")?.remove();
  const card = row.card;
  if (!card || state.mode !== "info") return;
  const element = document.createElement("div");
  element.className = "file-card";
  if (card.packages) {
    renderNupCard(element, card);
    row.tr.querySelector(".note").after(element);
    return;
  }
  const whole = Math.max(card.decompressed || card.size, card.size);
  const share = whole ? card.size / whole : 1;
  const compressed = card.solid + card.blocks > 0;
  const works = card.format ? { NXPress: 1, DBI: 0, Tinfoil: 0, Awoo: 0, CyberFoil: 0, Sphaira: 0, emulators: 0 }
    : card.solid > 0 ? { DBI: 1, Tinfoil: 1, Awoo: 1, CyberFoil: 1, Sphaira: 1, emulators: 0 }
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

// the card of a NUP: its size beside the one of its files uncompressed, then a line for each of
// its packages (the game, an update, a DLC) with who it is and its files; the size as stored is
// told only where it differs, for the files stored with zstd. NXPress only shows and verifies it.
function renderNupCard(element, card) {
  element.innerHTML = `<div class="card-sizes"><span class="kind"></span><div class="card-bar"><div></div></div><small></small></div><ul class="nup-packages"></ul><div class="card-only"></div>`;
  element.querySelector(".kind").textContent = card.kind;
  const whole = Math.max(card.decompressed, card.size);
  element.querySelector(".card-bar div").style.width = (whole ? card.size / whole * 100 : 100).toFixed(1) + "%";
  element.querySelector("small").textContent = t("cardNup")(size(card.size), size(card.decompressed));
  for (const pkg of card.packages) {
    const line = document.createElement("li");
    line.innerHTML = `<span class="k"></span><span class="who"><b></b><span></span></span><span class="num"></span>`;
    line.querySelector(".k").textContent = t("gameKind")(pkg.type);
    line.querySelector("b").textContent = pkg.name || "";
    line.querySelector("b").hidden = !pkg.name;
    const facts = [pkg.publisher, pkg.titleId, `v${pkg.version}` + (pkg.shown ? ` (${pkg.shown})` : "")].filter(Boolean).join(" · ");
    line.querySelector(".who span").textContent = (pkg.name ? " · " : "") + facts;
    const stored = size(pkg.stored) !== size(pkg.size) ? size(pkg.stored) : "";
    line.querySelector(".num").textContent = t("nupFiles")(pkg.files, size(pkg.size), stored);
    element.querySelector(".nup-packages").append(line);
  }
  element.querySelector(".card-only").textContent = t("nupOnly");
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
  // the newest version published, in blue when the list does not have it
  const online = row.online;
  if (online) {
    const newer = online.version > online.reach;
    facts.push([t("popNewest"), `v${online.version}` + (newer ? (online.date ? " · " + day(online.date) : "") : " · " + t("popNewestHave")), newer ? "pop-newer" : ""]);
  }
  const list = pop.querySelector("dl");
  list.innerHTML = "";
  for (const [name, value, tone] of facts) {
    const term = document.createElement("dt");
    const detail = document.createElement("dd");
    term.textContent = name;
    detail.textContent = value;
    if (tone) detail.className = tone;
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
  const hints = document.querySelectorAll("#about, #settings .hint:not(.always), #settings label.choice small, #preferences .pref-text > small:not(.pref-keys):not(.pref-status)");
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
  // what had the focus gets it back when the menu closes
  if (!menu.contains(document.activeElement)) menuOpener = document.activeElement;
  menu.querySelector("button:not(:disabled)")?.focus();
}
let menuOpener = null;
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
  // the question asked as the window closes with files at work, in this language too
  if (state.closeLanguage !== state.language) {
    state.closeLanguage = state.language;
    app().CloseTexts(t("closeAsk"), t("closeYes"), t("closeNo")).catch(() => { });
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
    const title = document.createElement("div");
    title.className = "group";
    title.textContent = t(group);
    $("modes").append(title);
    for (const mode of members) {
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
  }
  if (focused) $("modes").querySelector(`[data-mode="${focused}"]`)?.focus({ preventScroll: true });
  // the log takes the place of the list of the section, and its settings the one of the options
  const worklog = state.view === "worklog";
  document.querySelectorAll("[data-for]").forEach(element => { element.hidden = worklog ? element.dataset.for !== "worklog" : !element.dataset.for.split(" ").includes(state.mode); });
  $("work").classList.toggle("worklog-view", worklog);
  if (worklog) paintWorklog();
  // the page of the settings takes the place of the section and its options
  const preferences = state.view === "preferences";
  $("work").hidden = preferences;
  // a section without options (Details) gives the room of their column to the list
  $("settings").hidden = preferences || ![...$("settings").children].some(block => !block.hidden);
  $("preferences").hidden = !preferences;
  $("open-preferences").classList.toggle("on", preferences);
  $("keys-path").textContent = state.keys || t("keys")("");
  paintPublished();
  const page = worklog ? "worklog" : state.mode;
  $("title").textContent = t(page);
  // the section in the color of its group, its tile beside its title
  const group = worklog ? "groupCheck" : (groups.find(([, members]) => members.includes(state.mode)) || [""])[0];
  $("title-tile").className = `tile tone-${tones[group] || "neutral"}`;
  $("title-tile").innerHTML = icon(page);
  $("about").textContent = t("about" + page[0].toUpperCase() + page.slice(1));
  $("takes").innerHTML = "";
  for (const entry of takesShown[state.mode].split(" ")) {
    // "NSP/NSZ:takesUpdates": the kinds of file, and which of them it works on; ":takesDumps"
    // alone: what it takes besides files
    const [type, which] = entry.split(":");
    const tag = document.createElement("i");
    tag.textContent = which ? (type ? `${type} (${t(which)})` : t(which)) : type;
    $("takes").append(tag);
  }
  $("language-select").value = state.language;
  if (state.pendingCount) $("resume-text").textContent = t("resumeFound")(state.pendingCount);
  if (state.update) $("update-text").textContent = t("updateFound")(state.update.version);
  for (const button of document.querySelectorAll("button[data-theme]")) button.classList.toggle("on", button.dataset.theme === state.theme);
  paintPalettes();
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
  $("keys-banner").hidden = !!state.keys || !needsKeys(state.mode);
  // in the sections that take NUP files, the banner says they need no keys
  $("keys-nup").hidden = !["verify", "info", "rename"].includes(state.mode);
  $("toolbar").hidden = state.items.length === 0;
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
  const kept = state.items.filter(item => state.mode === "pack" ? item.dump : state.mode === "decompress" && isNup(item.path)).length;
  const dumps = state.mode === "pack" && kept > 0;
  $("remove-dumps-option").hidden = !dumps || !$("remove").checked;
  const keptText = dumps ? ($("remove-dumps").checked ? "removeDumpsGo" : "removeKeepsDumps") : "removeKeepsNup";
  $("remove-warning").textContent = [(kept < state.items.length || !kept) && t("removeWarning"), kept && t(keptText)].filter(Boolean).join(" ");
  $("remove-warning").hidden = !$("remove").checked;
  // the folders a watch looks at (#77): the ones added with Add folder or dropped, not the files
  // added on their own
  const watched = state.sources.filter(path => !/\.(nsp|nsz|xci|xcz|nup)$/i.test(path));
  $("watch-folders").textContent = watched.length ? t("watchFolders")(watched.length, watched.map(path => path.split(/[\\/]/).filter(Boolean).pop() || path).join(", ")) : t("watchNoFolders");
  $("watch-folders").title = watched.join("\n");
  $("watch-folders").hidden = !$("watch").checked;
  // the files that use the extra compression do not go with the installers above as they are; it
  // is for Compress only, where its box is
  $("extra-target-warning").hidden = !$("extra").checked || state.mode !== "compress";
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
  $("send-to-row").hidden = !state.setup.sendTo;
  $("send-to").textContent = state.setup.sendToEnabled ? t("sendToOff") : t("sendToOn");
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
  text.textContent = t("selected")(state.items.length, state.items.reduce((sum, item) => sum + (item.size || 0), 0));
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
  // the list is made again from all it came from, for the new mode: the files it does not take
  // leave, the ones it takes come, and the files taken out by hand stay out
  refill(state.sources.length ? state.sources : state.items.map(item => item.path), true, true);
  render();
}

// refill adds what the paths given hold for the mode, and keeps the paths as the origins of the
// list: the folders, to watch them, and all of them to fill the list again in another section.
// replace starts a new list; with sameOrigins it is the same list, for another section.
async function refill(paths, replace, sameOrigins) {
  const items = paths.length ? await app().Expand(paths, state.mode) : [];
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
  $("log").textContent = "";
  $("log").hidden = true;
  setStatus();
}
function clearList() {
  clearRows();
  state.sources = [];
  state.removed = new Set();
}

// add puts a file in the list; the new file of a pack (item.result) shows in it, but is no file
// to work on, so it stays out of the files a work is given
function add(item) {
  if (state.rows.has(item.path)) return;
  if (!item.result) state.items.push(item);
  // a file added again is no longer one taken out
  state.removed.delete(item.path);
  const tr = document.createElement("tr");
  tr.innerHTML = `<td class="name"><div class="game"><img class="icon" alt="" hidden><span class="cover" aria-hidden="true"></span><div><div class="title"></div><div class="file"></div></div></div><div class="note"></div></td><td class="size"></td><td class="state"></td><td class="remove"><button class="quiet">✕</button></td>`;
  // until the icon of the game comes, a cover tells the kind of file; a dump folder shows a folder
  const kind = item.dump ? "DUMP" : (item.path.match(/\.(\w+)$/) || ["", ""])[1].toUpperCase();
  tr.querySelector(".cover").textContent = item.dump ? "" : kind;
  if (item.dump) tr.querySelector(".cover").innerHTML = folderIcon;
  tr.querySelector(".cover").dataset.kind = kind;
  tr.querySelector(".title").textContent = item.name;
  tr.querySelector(".title").title = item.path;
  const game = tr.querySelector(".game");
  game.addEventListener("mouseenter", () => {
    clearTimeout(popTimer);
    popTimer = setTimeout(() => { const row = state.rows.get(item.path); if (row) showPop(row, game); }, 450);
  });
  game.addEventListener("mouseleave", hidePop);
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
    file.textContent = `${row.game.titleId} · v${row.game.version} · ${t("gameKind")(row.game.type)} · ${row.item.name}`;
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
    const paths = state.items.map(item => item.path);
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
    watch: $("watch").checked, noCache: $("no-cache").checked, template: state.template, keepNamed: $("keep-named").checked,
    packGames: $("pack-games").checked, packTemplate: state.packTemplate,
    lowPriority: $("low-priority").checked,
    // one box for the two experimental formats: each file gets the one it needs, or none
    sparse: $("extra").checked, archive: $("extra").checked,
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
}
function applyPreset() {
  const preset = [...builtinPresets(), ...savedPresets()][+$("preset").value];
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
  $("log").textContent = "";
  $("log").hidden = true;
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
    `NXPress ${state.setup.version} · ${state.setup.platform}`,
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
    // the files skipped by this work, not the ones left so by the work before
    const skippedRows = single || !state.lastRun ? 0 : state.lastRun.skipped;
    const status = event.cancelled ? t("stopped") : t("finished")(total, event.failed, event.seconds, skippedRows);
    setStatus(event.cancelled ? { key: "stopped" } : { key: "finished", args: [total, event.failed, event.seconds, skippedRows] });
    if (state.lastRun) state.lastRun.status = status;
    keepWork(event, status);
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
  app().AddToLog(work, worklogKeep()).then(works => { state.worklog = (works || []).filter(isWork); if (state.view === "worklog") render(); }).catch(() => { });
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
function paintWorklog() {
  const find = $("worklog-find").value.trim().toLowerCase();
  const problems = !!state.worklogProblems;
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
  const matches = row => !find || row.name.toLowerCase().includes(find) || (row.note || "").toLowerCase().includes(find);
  const shown = works.filter(work => (!problems || work.rows.some(row => row.status !== "ok")) && work.rows.some(matches));
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
    const rows = work.rows.filter(row => (!problems || row.status !== "ok") && matches(row)).sort((a, b) => order[a.status] - order[b.status]);
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
  // kept, to tell it again in another language
  state.update = update;
  $("update-text").textContent = t("updateFound")(update.version);
  $("update-get").onclick = () => app().OpenURL(update.url);
  $("update-close").onclick = () => { save("dismissedUpdate", update.version); $("update-banner").hidden = true; };
  $("update-banner").hidden = false;
}

// watchDrops shows the layer of a drop (show(true, count)) while files are dragged over the
// window (target), and ends it when they are dropped or the drag leaves it. On macOS NXPress takes
// the drop itself and the page gets neither a drop nor a leave: the files the system hands over
// (wails:file-drop of runtime) end it too, and so does the mouse moving on the layer, as no drag
// is going on then; the layer would else stay over the window, and nothing could be clicked (#46).
function watchDrops(target, runtime, layer, busy, show) {
  let dragging = 0;
  const end = () => { dragging = 0; show(false); };
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
  for (const button of document.querySelectorAll("button[data-theme]")) button.onclick = () => { state.theme = button.dataset.theme; save("theme", state.theme); theme(); render(); };
  // the language: the one chosen, else the one of the system when NXPress has it
  const system = navigator.language.toLowerCase().slice(0, 2);
  state.language = load("language", texts[system] ? system : "en");
  if (!texts[state.language]) state.language = "en";
  for (const [code, name] of languages) if (texts[code]) $("language-select").append(new Option(name, code));
  // another language tells again what is shown: the new version and the games to combine too
  $("language-select").onchange = () => { state.language = $("language-select").value; save("language", state.language); render(); previewPacks(); };
  state.mode = modes.includes(load("mode", "")) ? load("mode", "") : "compress";
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
  $("watch").checked = !!chosen.watch;
  $("no-cache").checked = !!chosen.noCache;
  state.verifyLevel = ["full", "signature", "both"].includes(chosen.verifyLevel) ? chosen.verifyLevel : "full";
  $("keep-named").checked = !!chosen.keepNamed;
  $("low-priority").checked = !!chosen.lowPriority;
  // experimental, and off unless it was turned on
  $("extra").checked = !!(chosen.sparse || chosen.archive);
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
  state.packTemplate = text(chosen.packTemplate) || await app().DefaultPackTemplate();
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
    const items = await app().Expand([folder], state.mode) || [];
    addOrigins([folder]);
    for (const item of items) add(item);
    if (state.mode === "rename") previewNames();
    previewPacks();
    render();
  };
  $("add-files").onclick = $("empty-files").onclick = addFiles;
  $("add-folder").onclick = $("empty-folder").onclick = addFolder;
  $("keys-choose").onclick = $("keys-change").onclick = () => $("keys").click();
  for (const button of document.querySelectorAll("[data-view]")) button.onclick = () => { save("view", button.dataset.view); render(); };
  for (const button of document.querySelectorAll("[data-verify-level]")) button.onclick = () => { state.verifyLevel = button.dataset.verifyLevel; save("settings", settings()); render(); };
  // the settings wait for the end of a work: they would hide its progress, Cancel and Pause
  $("open-preferences").onclick = () => { if (state.busy || state.starting) return; if (state.view !== "preferences") { state.view = "preferences"; render(); replay("enter", $("preferences")); } };
  $("remove").onchange = $("remove-dumps").onchange = $("watch").onchange = render;
  // the experimental option tells first, in detail, what it changes: it is on only once that
  // is accepted, and stays off when the window is closed or Esc is pressed
  $("extra").onchange = () => {
    if (!$("extra").checked) return render();
    $("extra").checked = false;
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
  $("space-cancel").onclick = () => $("space-dialog").close();
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
  $("overwrite").addEventListener("change", previewPacks);
  for (const field of [$("template"), $("pack-template")]) {
    field.addEventListener("input", () => fitTemplate(field));
    field.addEventListener("keydown", event => { if (event.key === "Enter") event.preventDefault(); });
  }
  $("pack-template").oninput = () => { state.packTemplate = $("pack-template").value; save("settings", settings()); previewPacks(); };
  $("pack-template-reset").onclick = async () => { state.packTemplate = await app().DefaultPackTemplate(); $("pack-template").value = state.packTemplate; save("settings", settings()); };
  $("send-to").onclick = async () => {
    try {
      await app().SetSendTo(!state.setup.sendToEnabled);
      state.setup.sendToEnabled = !state.setup.sendToEnabled;
    } catch (error) { setStatus(String(error)); }
    render();
  };
  $("updates").onchange = () => { save("updates", $("updates").checked); if ($("updates").checked) checkUpdate(); };
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
  $("start").onclick = start;
  $("estimate").onclick = () => start("estimate");
  $("compare-levels").onclick = () => start("estimate", { levels: true });
  $("measure").onclick = () => start("measure");
  $("measure-close").onclick = () => $("measure-dialog").close();
  $("last-report").onclick = openWorklog;
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
  $("preset-save").onclick = () => {
    const name = $("preset-name").value.trim();
    if (!name) { $("preset-name").focus(); return; }
    const presets = savedPresets().filter(preset => preset.name !== name);
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
  // the files NXPress was started with: dropped on it, or sent to it
  if (state.setup.files && state.setup.files.length) {
    state.mode = state.setup.mode;
    await refill(state.setup.files, true);
  }
  render();
  checkUpdate();
  loadVersions();
}

main();
