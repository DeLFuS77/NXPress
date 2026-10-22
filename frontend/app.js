"use strict";

const texts = {
  en: {
    compress: "Compress", decompress: "Decompress", recompress: "Recompress", trim: "Trim", verify: "Verify", convert: "Convert", split: "Split", pack: "Pack", info: "Details",
    aboutCompress: "NSP and XCI to NSZ and XCZ. Every file is checked before it gets its name.",
    aboutDecompress: "NSZ to NSP and XCZ to XCI, checked against the hashes of the game.",
    aboutRecompress: "Writes NSZ and XCZ files again, solid or in blocks, at another level: no NSP in between and no keys needed. Each file is replaced once its new copy is checked, unless you choose another folder.",
    aboutTrim: "Takes away what installers do not use: the padding after the data of an XCI, as game cards are dumped, and the delta fragments of updates in NSP and NSZ files. The file stays what it was, smaller, and works where it worked.",
    aboutVerify: "Checks NSP, NSZ, XCI and XCZ files against the hashes of their content.",
    aboutConvert: "XCI and XCZ to NSP and NSZ.",
    aboutSplit: "Writes each title of a multi-title file (SuperNSP) to a file of its own.",
    aboutPack: "Merges a game, its update and its DLC into one multi-title file (SuperNSP): an NSP, uncompressed, when they are all NSP; an NSZ when one of them is an NSZ. Of a title given twice the newest is kept: add a SuperNSP and a newer update to get it updated. With an XCI among them, its updates and DLC go into the game card: an XCI with everything.",
    aboutInfo: "The titles a file holds, how it is stored and what it works with.",
    addFiles: "Add files", addFolder: "Add folder", clear: "Clear list", dropHere: "Drop files or folders here", orChoose: "or choose them",
    compression: "Compression", result: "New files", advanced: "Advanced",
    keysMissing: "NXPress needs your prod.keys to read the games.", keysChoose: "Choose prod.keys",
    tagEmulators: "emulators", removeWarning: "Each original is deleted for good once its new file is checked.",
    selected: (n, bytes) => `${n === 1 ? "1 file" : n + " files"} · ${size(bytes)}`, nothing: "No files yet",
    working: (done, n, share) => `${done} of ${n} done · ${share}%`,
    saved: bytes => `${size(bytes)} saved`,
    file: "File", size: "Size", state: "State", files: n => n === 1 ? "1 file" : `${n} files`,
    level: "Compression level", light: "Light", medium: "Medium", maximum: "Maximum", faster: "faster", smaller: "smaller files",
    levelHint: l => l >= 18 ? "Maximum compression: the smallest files, the slowest." : l >= 10 ? "Medium compression: a good middle." : "Light compression: quick, files a bit larger.",
    groupSpace: "Space", groupOrganize: "Organize", groupCheck: "Check",
    hintCompress: "NSP, XCI → NSZ, XCZ", hintDecompress: "NSZ, XCZ → NSP, XCI", hintRecompress: "solid ↔ blocks, level", hintTrim: "padding and deltas",
    hintPack: "several → one SuperNSP or XCI", hintSplit: "SuperNSP → one per title", hintConvert: "XCI → NSP", hintVerify: "check the hashes", hintInfo: "titles and format",
    target: "Made for",
    forInstallers: "Installers",
    forInstallersHint: "Solid: the smallest files.",
    forEmulators: "Emulators and installers",
    forEmulatorsHint: "In blocks: emulators that read NSZ run them as they are.",
    xczBlock: "XCZ files are always in blocks, as nsz writes them.",
    threads: "Threads per file", multi: "Files at once", output: "Save to", nextTo: "Next to each file",
    choose: "Choose", overwrite: "Replace files already there", remove: "Delete originals once checked",
    packFile: "New file", noPack: "None chosen",
    start: m => ({ compress: "Compress", decompress: "Decompress", recompress: "Recompress", trim: "Trim", verify: "Verify", convert: "Convert", split: "Split", pack: "Pack", info: "Show details" })[m],
    estimateButton: "Estimate", estimating: "Estimating",
    datTitle: "Compare with a DAT", noDat: "None: the hashes of the content only",
    datHint: "A DAT of No-Intro or others lists the size and the hashes of each known dump: each file is told whether it is one of them.",
    language: "Language", theme: "Theme", themeAuto: "Auto", themeLight: "Light", themeDark: "Dark", showFolder: "Show in folder",
    sendToOn: "Add NXPress to “Send to” in the Explorer", sendToOff: "Take NXPress out of “Send to”", languageOther: "Español",
    cancel: "Cancel", waiting: "Waiting", ok: "OK", failed: "Failed", cancelled: "Cancelled", show: "Show",
    keys: k => k ? "Keys: " + k.split(/[\\/]/).pop() : "No prod.keys", needKeys: "Choose your prod.keys first.",
    needFiles: "Add some files first.", needPack: "Choose the name of the new file.",
    finished: (n, f, s) => `${n - f} of ${n} done` + (f ? `, ${f} failed` : "") + ` in ${time(s)}.`,
    stopped: "Cancelled.",
  },
  es: {
    compress: "Comprimir", decompress: "Descomprimir", recompress: "Recomprimir", trim: "Recortar", verify: "Verificar", convert: "Convertir", split: "Dividir", pack: "Juntar", info: "Detalles",
    aboutCompress: "NSP y XCI a NSZ y XCZ. Cada archivo se comprueba antes de darle su nombre.",
    aboutDecompress: "NSZ a NSP y XCZ a XCI, comprobado con los hashes del juego.",
    aboutRecompress: "Vuelve a escribir archivos NSZ y XCZ, sólidos o por bloques, con otro nivel: sin pasar por NSP y sin claves. Cada archivo se sustituye cuando su copia nueva está comprobada, salvo que elijas otra carpeta.",
    aboutTrim: "Quita lo que los instaladores no usan: el relleno tras los datos de un XCI, como se vuelcan los cartuchos, y los fragmentos delta de las actualizaciones en NSP y NSZ. El archivo sigue siendo el mismo tipo, más pequeño, y funciona donde funcionaba.",
    aboutVerify: "Comprueba archivos NSP, NSZ, XCI y XCZ con los hashes de su contenido.",
    aboutConvert: "XCI y XCZ a NSP y NSZ.",
    aboutSplit: "Escribe cada título de un archivo multi-título (SuperNSP) en un archivo propio.",
    aboutPack: "Junta un juego, su actualización y sus DLC en un solo archivo multi-título (SuperNSP): un NSP sin comprimir si todos son NSP; un NSZ si alguno es NSZ. Si un título aparece dos veces se queda el más nuevo: añade un SuperNSP y una actualización nueva para ponerlo al día. Con un XCI entre ellos, sus actualizaciones y DLC van dentro del cartucho: un XCI con todo.",
    aboutInfo: "Los títulos que tiene un archivo, cómo está guardado y con qué funciona.",
    addFiles: "Añadir archivos", addFolder: "Añadir carpeta", clear: "Vaciar lista", dropHere: "Suelta aquí archivos o carpetas", orChoose: "o elígelos",
    compression: "Compresión", result: "Archivos nuevos", advanced: "Avanzado",
    keysMissing: "NXPress necesita tu prod.keys para leer los juegos.", keysChoose: "Elegir prod.keys",
    tagEmulators: "emuladores", removeWarning: "Cada original se borra definitivamente cuando su archivo nuevo está comprobado.",
    selected: (n, bytes) => `${n === 1 ? "1 archivo" : n + " archivos"} · ${size(bytes)}`, nothing: "Aún no hay archivos",
    working: (done, n, share) => `${done} de ${n} hechos · ${share}%`,
    saved: bytes => `${size(bytes)} ahorrados`,
    file: "Archivo", size: "Tamaño", state: "Estado", files: n => n === 1 ? "1 archivo" : `${n} archivos`,
    level: "Nivel de compresión", light: "Ligera", medium: "Media", maximum: "Máxima", faster: "más rápido", smaller: "más pequeño",
    levelHint: l => l >= 18 ? "Compresión máxima: los archivos más pequeños, lo más lento." : l >= 10 ? "Compresión media: un buen término medio." : "Compresión ligera: rápido, archivos algo mayores.",
    groupSpace: "Espacio", groupOrganize: "Organizar", groupCheck: "Revisar",
    hintCompress: "NSP, XCI → NSZ, XCZ", hintDecompress: "NSZ, XCZ → NSP, XCI", hintRecompress: "sólido ↔ bloques, nivel", hintTrim: "relleno y deltas",
    hintPack: "varios → un SuperNSP o XCI", hintSplit: "SuperNSP → uno por título", hintConvert: "XCI → NSP", hintVerify: "comprobar los hashes", hintInfo: "títulos y formato",
    target: "Pensado para",
    forInstallers: "Instaladores",
    forInstallersHint: "Sólido: los archivos más pequeños.",
    forEmulators: "Emuladores e instaladores",
    forEmulatorsHint: "Por bloques: los emuladores que leen NSZ los ejecutan tal cual.",
    xczBlock: "Los XCZ siempre van por bloques, como los escribe nsz.",
    threads: "Hilos por archivo", multi: "Archivos a la vez", output: "Guardar en", nextTo: "Junto a cada archivo",
    choose: "Elegir", overwrite: "Reemplazar archivos existentes", remove: "Borrar originales tras comprobar",
    packFile: "Archivo nuevo", noPack: "Sin elegir",
    start: m => ({ compress: "Comprimir", decompress: "Descomprimir", recompress: "Recomprimir", trim: "Recortar", verify: "Verificar", convert: "Convertir", split: "Dividir", pack: "Juntar", info: "Ver detalles" })[m],
    estimateButton: "Calcular ahorro", estimating: "Calculando",
    datTitle: "Comparar con un DAT", noDat: "Ninguno: solo los hashes del contenido",
    datHint: "Un DAT de No-Intro u otros da el tamaño y los hashes de cada volcado conocido: de cada archivo se dice si es uno de ellos.",
    language: "Idioma", theme: "Tema", themeAuto: "Auto", themeLight: "Claro", themeDark: "Oscuro", showFolder: "Mostrar en la carpeta",
    sendToOn: "Añadir NXPress a «Enviar a» del Explorador", sendToOff: "Quitar NXPress de «Enviar a»", languageOther: "English",
    cancel: "Cancelar", waiting: "En espera", ok: "OK", failed: "Error", cancelled: "Cancelado", show: "Mostrar",
    keys: k => k ? "Claves: " + k.split(/[\\/]/).pop() : "Sin prod.keys", needKeys: "Elige antes tu prod.keys.",
    needFiles: "Añade antes algún archivo.", needPack: "Elige el nombre del archivo nuevo.",
    finished: (n, f, s) => `${n - f} de ${n} hechos` + (f ? `, ${f} con error` : "") + ` en ${time(s)}.`,
    stopped: "Cancelado.",
  },
};
const modes = ["compress", "decompress", "recompress", "trim", "verify", "convert", "split", "pack", "info"];
// the menu, by what the modes are for
const groups = [
  ["groupSpace", ["compress", "decompress", "recompress", "trim"]],
  ["groupOrganize", ["pack", "split", "convert"]],
  ["groupCheck", ["verify", "info"]],
];
const steps = {
  es: { Comparing: "Comparando", Estimating: "Calculando", Trimming: "Recortando", Recompressing: "Recomprimiendo", Compressing: "Comprimiendo", Verifying: "Verificando", Decompressing: "Descomprimiendo", Converting: "Convirtiendo", Packing: "Juntando", Splitting: "Dividiendo" },
};

const $ = id => document.getElementById(id);
const app = () => window.go.main.App;
const state = { theme: "auto", mode: "compress", language: "en", items: [], rows: new Map(), busy: false, keys: "", output: "", pack: "", setup: null };

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
    [/ smaller/g, " menos"], [/^solid, /, "sólido, "], [/in the DAT: /g, "en el DAT: "], [/not en el DAT: no dump of its size/g, "no está en el DAT: ningún volcado de su tamaño"], [/not en el DAT: (\d+) dumps of its size, with other hashes/g, "no está en el DAT: $1 volcados de su tamaño, con otros hashes"], [/, solid and in blocks: /g, ", sólido y por bloques: "], [/, solid: /g, ", sólido: "], [/, in blocks: /g, ", por bloques: "], [/, uncompressed: /g, ", sin comprimir: "], [/ uncompressed \(/g, " sin comprimir ("], [/works with: every installer \(DBI, Tinfoil, Awoo, CyberFoil, Sphaira\); emulators need it decompressed/g, "funciona con: todos los instaladores (DBI, Tinfoil, Awoo, CyberFoil, Sphaira); los emuladores lo necesitan descomprimido"], [/works with: every installer but Awoo; emulators that read NSZ run it as it is/g, "funciona con: todos los instaladores salvo Awoo; los emuladores que leen NSZ lo ejecutan tal cual"], [/works with: every installer and emulator/g, "funciona con: todos los instaladores y emuladores"], [/padding: (.+?) that trimming takes away/g, "relleno: $1 que Recortar quita"], [/delta fragments: (.+?) that trimming takes away/g, "fragmentos delta: $1 que Recortar quita"], [/^about /, "unos "], [/, (\S+ \S+) at level (\d+)$/, ", $1 con nivel $2"], [/^nothing in it can be compressed$/, "No hay nada que comprimir"], [/ of padding taken away/, " de relleno quitados"], [/^nothing to trim, it ends where its data ends$/, "Nada que recortar: acaba donde acaban sus datos"], [/^nothing to trim, it holds no delta fragments$/, "Nada que recortar: no tiene fragmentos delta"], [/ of delta fragments taken away/, " de fragmentos delta quitados"], [/data follows the last partition: it is not padding, the XCI is left as it is/, "hay datos tras la última partición: no es relleno, el XCI se deja como está"], [/^in blocks, /, "por bloques, "], [/(\d+) NCA rewritten by a tool, their data checked/, "$1 NCA reescritos por una herramienta, sus datos comprobados"],
    [/^nothing in it can be compressed, it is left as it is$/, "No hay nada que comprimir: se queda como está"],
    [/^it holds one title, nothing to split$/, "Tiene un solo título: no hay nada que dividir"],
    [/^(\d+) files: /, "$1 archivos: "], [/; some of its files are in no part, so it is kept/, "; algunos de sus archivos no van en ninguna parte, así que se conserva"],
    [/^(\d+) files merged/, "$1 archivos juntados"], [/, newer versions in place of /, ", versiones nuevas en lugar de "], [/ of delta fragments left out/, " de fragmentos delta descartados"],
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
  for (const [group, members] of groups) {
    const title = document.createElement("div");
    title.className = "group";
    title.textContent = t(group);
    $("modes").append(title);
    for (const mode of members) {
      const button = document.createElement("button");
      button.innerHTML = "<b></b><small></small>";
      button.querySelector("b").textContent = t(mode);
      button.querySelector("small").textContent = t("hint" + mode[0].toUpperCase() + mode.slice(1));
      button.className = mode === state.mode ? "on" : "";
      button.onclick = () => setMode(mode);
      $("modes").append(button);
    }
  }
  document.querySelectorAll("[data-for]").forEach(element => { element.hidden = !element.dataset.for.split(" ").includes(state.mode); });
  $("title").textContent = t(state.mode);
  $("about").textContent = t("about" + state.mode[0].toUpperCase() + state.mode.slice(1));
  $("takes").innerHTML = "";
  for (const type of { compress: "NSP XCI", decompress: "NSZ XCZ", recompress: "NSZ XCZ", trim: "XCI NSP NSZ", verify: "NSP NSZ XCI XCZ", convert: "XCI XCZ", split: "NSP NSZ XCI XCZ", pack: "NSP NSZ XCI XCZ", info: "NSP NSZ XCI XCZ" }[state.mode].split(" ")) {
    const tag = document.createElement("i");
    tag.textContent = type;
    $("takes").append(tag);
  }
  for (const button of document.querySelectorAll("[data-language]")) button.classList.toggle("on", button.dataset.language === state.language);
  for (const button of document.querySelectorAll("button[data-theme]")) button.classList.toggle("on", button.dataset.theme === state.theme);
  $("keys-banner").hidden = !!state.keys || state.mode === "recompress";
  $("toolbar").hidden = state.items.length === 0;
  $("drop").classList.toggle("filled", state.items.length > 0);
  // the level falls in light (1 to 6), medium (7 to 14) or maximum (15 to 19)
  const level = +$("level").value;
  const band = level <= 6 ? "3" : level <= 14 ? "12" : "19";
  for (const button of document.querySelectorAll("[data-level]")) button.classList.toggle("on", button.dataset.level === band);
  $("remove-warning").hidden = !$("remove").checked;
  summary();
  $("keys").textContent = t("keys")(state.keys);
  $("keys").title = state.keys;
  $("keys").className = "chip" + (state.keys ? "" : " bad");
  $("level-value").textContent = $("level").value;
  $("level-hint").textContent = t("levelHint")(+$("level").value);
  $("output").textContent = state.output || t("nextTo");
  $("output").title = state.output;
  $("reset-output").hidden = !state.output;
  $("pack").textContent = state.pack || t("noPack");
  $("dat").textContent = state.dat || t("noDat");
  $("dat").title = state.dat;
  $("reset-dat").hidden = !state.dat;
  $("pack").title = state.pack;
  $("start").textContent = state.busy ? t("cancel") : t("start")(state.mode);
  $("start").className = "primary big" + (state.busy ? " stop" : "");
  $("start").disabled = !state.busy && state.items.length === 0;
  $("estimate").disabled = state.busy || state.items.length === 0;
  $("send-to").hidden = !state.setup.sendTo;
  $("reset-output").title = t("nextTo");
  $("send-to").textContent = state.setup.sendToEnabled ? t("sendToOff") : t("sendToOn");
  $("empty").hidden = state.items.length > 0;
  $("files").hidden = state.items.length === 0;
  document.body.classList.toggle("busy", state.busy);
  for (const row of state.rows.values()) paint(row);
}

// summary tells what the list holds, how far the work is, and what it saved.
function summary() {
  const rows = [...state.rows.values()];
  const text = $("summary-text");
  const bar = $("total-bar");
  text.textContent = "";
  bar.hidden = true;
  if (state.busy) {
    const done = rows.filter(row => row.status === "ok" || row.status === "failed").length;
    const share = rows.reduce((sum, row) => sum + (row.status === "ok" || row.status === "failed" ? 1 : row.total ? row.done / row.total : 0), 0) / Math.max(1, rows.length);
    text.textContent = t("working")(done, rows.length, Math.floor(share * 100));
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
  }
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
  const row = { item, tr, status: "", step: "", done: 0, total: 0, note: "", output: "", error: "", after: 0 };
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
    cell.innerHTML = `<span class="badge waiting"></span>`;
    cell.firstChild.textContent = t("waiting");
  } else if (row.status === "ok") {
    cell.innerHTML = `<span class="badge ok"></span>`;
    cell.firstChild.textContent = t("ok");
  } else if (row.status === "failed") {
    cell.innerHTML = `<span class="badge ${row.error === "cancelled" ? "waiting" : "failed"}"></span>`;
    cell.firstChild.textContent = row.error === "cancelled" ? t("cancelled") : t("failed");
  } else {
    cell.innerHTML = "";
  }
  note.className = "note" + (row.status === "failed" ? " bad" : "");
  note.textContent = noteText(row.status === "failed" ? (row.error === "cancelled" ? "" : row.error) : row.note);
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

function settings() {
  return {
    level: +$("level").value, block: $("target-block").checked, threads: +$("threads").value, multi: +$("multi").value,
    overwrite: $("overwrite").checked, removeSource: $("remove").checked, output: state.output, dat: state.dat,
  };
}

async function start(mode) {
  if (state.busy) { app().Cancel(); return; }
  mode = typeof mode === "string" ? mode : state.mode;
  if (!state.keys && state.mode !== "recompress") { $("status").textContent = t("needKeys"); $("keys-banner").scrollIntoView(); return; }
  if (!state.items.length) { $("status").textContent = t("needFiles"); return; }
  if (state.mode === "pack" && !state.pack) { $("status").textContent = t("needPack"); return; }
  const chosen = settings();
  save("settings", chosen);
  for (const row of state.rows.values()) Object.assign(row, { status: "waiting", step: "", done: 0, total: 0, note: "", output: "", error: "", after: 0 });
  $("log").textContent = "";
  $("log").hidden = true;
  $("status").textContent = "";
  try {
    await app().Start({ mode, files: state.items.map(item => item.path), pack: state.pack, ...chosen });
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
    summary();
  });
  events.EventsOn("done", event => {
    if (state.mode === "pack") {
      // a pack is one result for all its files
      for (const row of state.rows.values()) if (row.item.path !== event.file) Object.assign(row, { status: event.error ? "" : "ok" });
    }
    const row = rowOf(event.file);
    Object.assign(row, { status: event.error ? "failed" : "ok", note: event.note || "", output: event.output || "", error: event.error || "", after: event.size || 0 });
    paint(row);
    summary();
  });
  events.EventsOn("say", text => {
    $("log").hidden = false;
    $("log").textContent += noteText(text) + "\n";
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
  // the theme: the one of Windows, or the one chosen
  state.theme = load("theme", "auto");
  const theme = () => {
    if (state.theme === "auto") document.documentElement.removeAttribute("data-theme");
    else document.documentElement.dataset.theme = state.theme;
  };
  theme();
  for (const button of document.querySelectorAll("button[data-theme]")) button.onclick = () => { state.theme = button.dataset.theme; save("theme", state.theme); theme(); render(); };
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
  state.dat = chosen.dat || "";

  $("level").oninput = render;
  document.querySelectorAll("[data-level]").forEach(button => { button.onclick = () => { $("level").value = button.dataset.level; render(); }; });
  for (const button of document.querySelectorAll("[data-language]")) button.onclick = () => { state.language = button.dataset.language; save("language", state.language); render(); };
  $("keys").onclick = async () => {
    try { const path = await app().ChooseKeys(); if (path) state.keys = path; } catch (error) { $("status").textContent = String(error); }
    render();
  };
  const addFiles = async () => { for (const item of await app().ChooseFiles(state.mode) || []) add(item); render(); };
  const addFolder = async () => { for (const item of await app().ChooseFolder(state.mode) || []) add(item); render(); };
  $("add-files").onclick = $("empty-files").onclick = addFiles;
  $("add-folder").onclick = $("empty-folder").onclick = addFolder;
  $("keys-choose").onclick = () => $("keys").click();
  $("remove").onchange = render;
  // the menu closes once used, or on a click elsewhere
  document.addEventListener("click", event => { if (!$("menu").contains(event.target)) $("menu").open = false; });
  $("clear").onclick = () => { clearList(); render(); };
  $("choose-output").onclick = async () => { const path = await app().ChooseOutput(); if (path) state.output = path; save("settings", settings()); render(); };
  $("reset-output").onclick = () => { state.output = ""; save("settings", settings()); render(); };
  $("choose-dat").onclick = async () => { const path = await app().ChooseDat(); if (path) state.dat = path; save("settings", settings()); render(); };
  $("reset-dat").onclick = () => { state.dat = ""; save("settings", settings()); render(); };
  $("choose-pack").onclick = async () => { const path = await app().ChoosePack(); if (path) state.pack = path; render(); };
  $("send-to").onclick = async () => {
    try {
      await app().SetSendTo(!state.setup.sendToEnabled);
      state.setup.sendToEnabled = !state.setup.sendToEnabled;
    } catch (error) { $("status").textContent = String(error); }
    $("menu").open = false;
    render();
  };
  $("start").onclick = start;
  $("estimate").onclick = () => start("estimate");
  listen();
  // the files NXPress was started with: dropped on it, or sent to it
  if (state.setup.files && state.setup.files.length) {
    state.mode = state.setup.mode;
    await refill(state.setup.files, true);
  }
  render();
}

main();
