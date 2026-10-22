"use strict";

const texts = {
  en: {
    compress: "Compress", decompress: "Decompress", verify: "Verify", convert: "Convert", split: "Split", pack: "Pack", info: "Info",
    aboutCompress: "NSP and XCI to NSZ and XCZ. Every file is checked before it gets its name.",
    aboutDecompress: "NSZ to NSP and XCZ to XCI, checked against the hashes of the game.",
    aboutVerify: "Checks NSP, NSZ, XCI and XCZ files against the hashes of their content.",
    aboutConvert: "XCI and XCZ to NSP and NSZ.",
    aboutSplit: "Writes each title of a multi-title file (SuperNSP) to a file of its own.",
    aboutPack: "Merges a game, its update and its DLC into one multi-title file (SuperNSP): an NSP, uncompressed, when they are all NSP; an NSZ when one of them is an NSZ.",
    aboutInfo: "Lists the titles a file holds.",
    addFiles: "Add files", addFolder: "Add folder", clear: "Clear", dropHere: "Drop files or folders here",
    file: "File", size: "Size", state: "State", files: n => n === 1 ? "1 file" : `${n} files`,
    level: "Level", fast: "Fast", balanced: "Balanced", small: "Smallest",
    levelHint: l => l >= 18 ? "Smallest files, slowest." : l >= 10 ? "A good middle." : "Quick, files a bit larger.",
    target: "Made for",
    forInstallers: "Installers",
    forInstallersHint: "Solid: the smallest, and every installer takes it (DBI, Tinfoil, Awoo, CyberFoil, Sphaira).",
    forEmulators: "Emulators and installers",
    forEmulatorsHint: "In blocks: emulators that read NSZ run it as it is, without installing it. Every installer takes it but Awoo.",
    xczBlock: "XCZ files are always in blocks, as nsz writes them.",
    threads: "Threads per file", multi: "Files at once", output: "Save to", nextTo: "Next to each file",
    choose: "Choose", overwrite: "Replace files already there", remove: "Delete originals once checked",
    packFile: "New file", noPack: "None chosen",
    start: m => ({ compress: "Compress", decompress: "Decompress", verify: "Verify", convert: "Convert", split: "Split", pack: "Pack", info: "Show titles" })[m],
    cancel: "Cancel", waiting: "Waiting", ok: "OK", failed: "Failed", cancelled: "Cancelled", show: "Show",
    keys: k => k ? "Keys: " + k.split(/[\\/]/).pop() : "No prod.keys", needKeys: "Choose your prod.keys first.",
    needFiles: "Add some files first.", needPack: "Choose the name of the new file.",
    finished: (n, f, s) => `${n - f} of ${n} done` + (f ? `, ${f} failed` : "") + ` in ${time(s)}.`,
    stopped: "Cancelled.",
  },
  es: {
    compress: "Comprimir", decompress: "Descomprimir", verify: "Verificar", convert: "Convertir", split: "Dividir", pack: "Juntar", info: "Info",
    aboutCompress: "NSP y XCI a NSZ y XCZ. Cada archivo se comprueba antes de darle su nombre.",
    aboutDecompress: "NSZ a NSP y XCZ a XCI, comprobado con los hashes del juego.",
    aboutVerify: "Comprueba archivos NSP, NSZ, XCI y XCZ con los hashes de su contenido.",
    aboutConvert: "XCI y XCZ a NSP y NSZ.",
    aboutSplit: "Escribe cada título de un archivo multi-título (SuperNSP) en un archivo propio.",
    aboutPack: "Junta un juego, su actualización y sus DLC en un solo archivo multi-título (SuperNSP): un NSP sin comprimir si todos son NSP; un NSZ si alguno es NSZ.",
    aboutInfo: "Muestra los títulos que tiene un archivo.",
    addFiles: "Añadir archivos", addFolder: "Añadir carpeta", clear: "Vaciar", dropHere: "Suelta aquí archivos o carpetas",
    file: "Archivo", size: "Tamaño", state: "Estado", files: n => n === 1 ? "1 archivo" : `${n} archivos`,
    level: "Nivel", fast: "Rápido", balanced: "Equilibrado", small: "Mínimo",
    levelHint: l => l >= 18 ? "Archivos más pequeños, más lento." : l >= 10 ? "Un buen término medio." : "Rápido, archivos algo mayores.",
    target: "Pensado para",
    forInstallers: "Instaladores",
    forInstallersHint: "Sólido: el más pequeño, y lo aceptan todos los instaladores (DBI, Tinfoil, Awoo, CyberFoil, Sphaira).",
    forEmulators: "Emuladores e instaladores",
    forEmulatorsHint: "Por bloques: los emuladores que leen NSZ lo ejecutan tal cual, sin instalarlo. Lo aceptan todos los instaladores salvo Awoo.",
    xczBlock: "Los XCZ siempre van por bloques, como los escribe nsz.",
    threads: "Hilos por archivo", multi: "Archivos a la vez", output: "Guardar en", nextTo: "Junto a cada archivo",
    choose: "Elegir", overwrite: "Reemplazar archivos existentes", remove: "Borrar originales tras comprobar",
    packFile: "Archivo nuevo", noPack: "Sin elegir",
    start: m => ({ compress: "Comprimir", decompress: "Descomprimir", verify: "Verificar", convert: "Convertir", split: "Dividir", pack: "Juntar", info: "Ver títulos" })[m],
    cancel: "Cancelar", waiting: "En espera", ok: "OK", failed: "Error", cancelled: "Cancelado", show: "Mostrar",
    keys: k => k ? "Claves: " + k.split(/[\\/]/).pop() : "Sin prod.keys", needKeys: "Elige antes tu prod.keys.",
    needFiles: "Añade antes algún archivo.", needPack: "Elige el nombre del archivo nuevo.",
    finished: (n, f, s) => `${n - f} de ${n} hechos` + (f ? `, ${f} con error` : "") + ` en ${time(s)}.`,
    stopped: "Cancelado.",
  },
};
const modes = ["compress", "decompress", "verify", "convert", "split", "pack", "info"];
const steps = {
  es: { Compressing: "Comprimiendo", Verifying: "Verificando", Decompressing: "Descomprimiendo", Converting: "Convirtiendo", Packing: "Juntando", Splitting: "Dividiendo" },
};

const $ = id => document.getElementById(id);
const app = () => window.go.main.App;
const state = { mode: "compress", language: "en", items: [], rows: new Map(), busy: false, keys: "", output: "", pack: "", setup: null };

function load(name, fallback) {
  try { const value = localStorage.getItem("nxpress." + name); return value === null ? fallback : JSON.parse(value); } catch { return fallback; }
}
function save(name, value) {
  try { localStorage.setItem("nxpress." + name, JSON.stringify(value)); } catch { }
}
const t = key => texts[state.language][key];

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
// the notes of the results come in English; the usual ones are told in Spanish too
const notes = {
  en: [[/ \(use -w to replace it\)/, " (tick “Replace files already there”)"]],
  es: [
    [/ smaller/g, " menos"], [/(\d+) NCA rewritten by a tool, their data checked/, "$1 NCA reescritos por una herramienta, sus datos comprobados"],
    [/^nothing in it can be compressed, it is left as it is$/, "No hay nada que comprimir: se queda como está"],
    [/^it holds one title, nothing to split$/, "Tiene un solo título: no hay nada que dividir"],
    [/^(\d+) files: /, "$1 archivos: "], [/; some of its files are in no part, so it is kept/, "; algunos de sus archivos no van en ninguna parte, así que se conserva"],
    [/^(\d+) files merged/, "$1 archivos juntados"], [/ of delta fragments left out/, " de fragmentos delta descartados"],
    [/^1 title$/, "1 título"], [/^(\d+) titles$/, "$1 títulos"], [/ is already there \(use -w to replace it\)/, " ya existe (marca «Reemplazar archivos existentes»)"],
  ],
};
function noteText(note) {
  for (const [pattern, replacement] of notes[state.language] || []) note = note.replace(pattern, replacement);
  return note;
}
function stepText(step) {
  const table = steps[state.language] || {};
  const [first, ...rest] = step.split(" ");
  return [table[first] || first, ...rest].join(" ");
}

function render() {
  document.documentElement.lang = state.language;
  document.querySelectorAll("[data-t]").forEach(element => { element.textContent = t(element.dataset.t); });
  $("modes").innerHTML = "";
  for (const mode of modes) {
    const button = document.createElement("button");
    button.textContent = t(mode);
    button.className = mode === state.mode ? "on" : "";
    button.onclick = () => setMode(mode);
    $("modes").append(button);
  }
  document.querySelectorAll("[data-for]").forEach(element => { element.hidden = !element.dataset.for.split(" ").includes(state.mode); });
  $("about").textContent = t("about" + state.mode[0].toUpperCase() + state.mode.slice(1));
  $("takes").textContent = { compress: "NSP · XCI", decompress: "NSZ · XCZ", verify: "NSP · NSZ · XCI · XCZ", convert: "XCI · XCZ", split: "NSP · NSZ · XCI · XCZ", pack: "NSP · NSZ", info: "NSP · NSZ · XCI · XCZ" }[state.mode];
  $("language").textContent = state.language === "en" ? "Español" : "English";
  $("keys").textContent = t("keys")(state.keys);
  $("keys").title = state.keys;
  $("keys").className = "chip" + (state.keys ? "" : " bad");
  $("level-value").textContent = $("level").value;
  $("level-hint").textContent = t("levelHint")(+$("level").value);
  $("output").textContent = state.output || t("nextTo");
  $("output").title = state.output;
  $("reset-output").hidden = !state.output;
  $("pack").textContent = state.pack || t("noPack");
  $("pack").title = state.pack;
  $("start").textContent = state.busy ? t("cancel") : t("start")(state.mode);
  $("start").className = "primary" + (state.busy ? " stop" : "");
  $("count").textContent = state.items.length ? t("files")(state.items.length) + " · " + size(state.items.reduce((sum, item) => sum + item.size, 0)) : "";
  $("empty").hidden = state.items.length > 0;
  $("files").hidden = state.items.length === 0;
  document.body.classList.toggle("busy", state.busy);
  for (const row of state.rows.values()) paint(row);
}

function setMode(mode) {
  if (state.busy || mode === state.mode) return;
  state.mode = mode;
  save("mode", mode);
  // the files the new mode does not take leave the list
  refill(state.items.map(item => item.path), true);
  render();
}

async function refill(paths, replace) {
  const items = paths.length ? await app().Expand(paths, state.mode) : [];
  if (replace) clearList();
  for (const item of items) add(item);
  render();
}

function clearList() {
  state.items = [];
  state.rows.clear();
  $("files").tBodies[0].innerHTML = "";
  $("log").textContent = "";
  $("log").hidden = true;
  $("status").textContent = "";
}

function add(item) {
  if (state.rows.has(item.path)) return;
  state.items.push(item);
  const tr = document.createElement("tr");
  tr.innerHTML = `<td class="name"><div></div><div class="note"></div></td><td class="size"></td><td class="state"></td><td class="remove"><button class="quiet">✕</button></td>`;
  tr.querySelector(".name div").textContent = item.name;
  tr.querySelector(".name div").title = item.path;
  tr.querySelector(".size").textContent = item.size ? size(item.size) : "";
  tr.querySelector(".remove button").onclick = () => {
    if (state.busy) return;
    state.items = state.items.filter(other => other.path !== item.path);
    state.rows.delete(item.path);
    tr.remove();
    render();
  };
  $("files").tBodies[0].append(tr);
  const row = { item, tr, status: "", step: "", done: 0, total: 0, note: "", output: "", error: "" };
  state.rows.set(item.path, row);
  paint(row);
}

function paint(row) {
  const cell = row.tr.querySelector(".state");
  const note = row.tr.querySelector(".note");
  if (row.status === "work") {
    const share = row.total ? row.done / row.total : 0;
    cell.innerHTML = `<div class="step"></div><div class="bar"><div></div></div>`;
    cell.querySelector(".step").textContent = `${stepText(row.step)} · ${Math.floor(share * 100)}%`;
    cell.querySelector(".bar div").style.width = (share * 100).toFixed(1) + "%";
  } else if (row.status === "waiting") {
    cell.innerHTML = `<span class="step"></span>`;
    cell.firstChild.textContent = t("waiting");
  } else if (row.status === "ok") {
    cell.innerHTML = `<span class="ok"></span>`;
    cell.firstChild.textContent = t("ok");
  } else if (row.status === "failed") {
    cell.innerHTML = `<span class="failed"></span>`;
    cell.firstChild.textContent = row.error === "cancelled" ? t("cancelled") : t("failed");
  } else {
    cell.innerHTML = "";
  }
  note.className = "note" + (row.status === "failed" ? " bad" : "");
  note.textContent = noteText(row.status === "failed" ? (row.error === "cancelled" ? "" : row.error) : row.note);
  if (row.output) {
    if (note.textContent) note.append(" · ");
    const link = document.createElement("a");
    link.textContent = t("show") + " " + row.output.split(/[\\/]/).pop();
    link.onclick = () => app().ShowFile(row.output);
    note.append(link);
  }
}

function rowOf(file) {
  if (!state.rows.has(file)) add({ path: file, name: file.split(/[\\/]/).pop(), size: 0 });
  return state.rows.get(file);
}

function settings() {
  return {
    level: +$("level").value, block: $("target-block").checked, threads: +$("threads").value, multi: +$("multi").value,
    overwrite: $("overwrite").checked, removeSource: $("remove").checked, output: state.output,
  };
}

async function start() {
  if (state.busy) { app().Cancel(); return; }
  if (!state.keys) { $("status").textContent = t("needKeys"); return; }
  if (!state.items.length) { $("status").textContent = t("needFiles"); return; }
  if (state.mode === "pack" && !state.pack) { $("status").textContent = t("needPack"); return; }
  const chosen = settings();
  save("settings", chosen);
  for (const row of state.rows.values()) Object.assign(row, { status: "waiting", step: "", done: 0, total: 0, note: "", output: "", error: "" });
  $("log").textContent = "";
  $("log").hidden = true;
  $("status").textContent = "";
  try {
    await app().Start({ mode: state.mode, files: state.items.map(item => item.path), pack: state.pack, ...chosen });
    state.busy = true;
  } catch (error) {
    for (const row of state.rows.values()) row.status = "";
    $("status").textContent = String(error);
  }
  render();
}

function listen() {
  const events = window.runtime;
  events.EventsOn("progress", event => {
    const row = rowOf(event.file);
    Object.assign(row, { status: "work", step: event.step, done: event.done, total: event.total });
    paint(row);
  });
  events.EventsOn("done", event => {
    if (state.mode === "pack") {
      // a pack is one result for all its files
      for (const row of state.rows.values()) if (row.item.path !== event.file) Object.assign(row, { status: event.error ? "" : "ok" });
    }
    const row = rowOf(event.file);
    Object.assign(row, { status: event.error ? "failed" : "ok", note: event.note || "", output: event.output || "", error: event.error || "" });
    paint(row);
  });
  events.EventsOn("say", text => {
    $("log").hidden = false;
    $("log").textContent += text + "\n";
    $("log").scrollTop = $("log").scrollHeight;
  });
  events.EventsOn("finished", event => {
    state.busy = false;
    for (const row of state.rows.values()) if (row.status === "waiting" || row.status === "work") { row.status = "failed"; row.error = "cancelled"; }
    const total = state.mode === "pack" ? 1 : state.items.length;
    $("status").textContent = event.cancelled ? t("stopped") : t("finished")(total, event.failed, event.seconds);
    render();
  });
  events.OnFileDrop((x, y, paths) => { if (!state.busy) refill(paths, false); }, true);
}

async function main() {
  state.setup = await app().Setup();
  state.keys = state.setup.keys;
  state.language = load("language", navigator.language.toLowerCase().startsWith("es") ? "es" : "en");
  state.mode = modes.includes(load("mode", "")) ? load("mode", "") : "compress";
  const chosen = load("settings", {});
  $("version").textContent = state.setup.version;
  $("level").max = state.setup.maxLevel;
  $("level").value = chosen.level || state.setup.level;
  $(chosen.block ? "target-block" : "target-solid").checked = true;
  $("threads").value = chosen.threads || state.setup.threads;
  $("threads").max = state.setup.cpus;
  $("multi").value = chosen.multi || 1;
  $("overwrite").checked = !!chosen.overwrite;
  $("remove").checked = !!chosen.removeSource;
  state.output = chosen.output || "";

  $("level").oninput = render;
  document.querySelectorAll("[data-level]").forEach(button => { button.onclick = () => { $("level").value = button.dataset.level; render(); }; });
  $("language").onclick = () => { state.language = state.language === "en" ? "es" : "en"; save("language", state.language); render(); };
  $("keys").onclick = async () => {
    try { const path = await app().ChooseKeys(); if (path) state.keys = path; } catch (error) { $("status").textContent = String(error); }
    render();
  };
  $("add-files").onclick = async () => { for (const item of await app().ChooseFiles(state.mode) || []) add(item); render(); };
  $("add-folder").onclick = async () => { for (const item of await app().ChooseFolder(state.mode) || []) add(item); render(); };
  $("clear").onclick = () => { clearList(); render(); };
  $("choose-output").onclick = async () => { const path = await app().ChooseOutput(); if (path) state.output = path; save("settings", settings()); render(); };
  $("reset-output").onclick = () => { state.output = ""; save("settings", settings()); render(); };
  $("choose-pack").onclick = async () => { const path = await app().ChoosePack(); if (path) state.pack = path; render(); };
  $("start").onclick = start;
  listen();
  render();
}

main();
