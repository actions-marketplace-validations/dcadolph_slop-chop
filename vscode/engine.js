/* The bundled slop-chop engine. Loads the wasm build staged in engine/ once per extension
   host and runs every check and chop in-process, so the extension needs no binary on the
   PATH. The engine resolves the profile, the project file, the presets, and the voice with
   the command line's own precedence. This file only finds and reads those files. */
"use strict";

const fs = require("fs");
const os = require("os");
const path = require("path");

// PROJECT_FILE is the project profile the command line discovers from its working directory
// upward.
const PROJECT_FILE = ".slop-chop.json";

let ready = null;

// boot loads the wasm engine once. A failed boot clears the cache, so the next call retries
// instead of replaying a stale rejection.
function boot(dir) {
  if (ready) return ready;
  ready = (async () => {
    // wasm_exec.js is Go's runtime shim; it defines Go on the global object.
    (0, eval)(fs.readFileSync(path.join(dir, "wasm_exec.js"), "utf8"));
    const go = new globalThis.Go();
    const bytes = fs.readFileSync(path.join(dir, "slop-chop.wasm"));
    const result = await WebAssembly.instantiate(bytes, go.importObject);
    go.run(result.instance);
    // Give the Go runtime a tick to register its globals.
    await new Promise((r) => setTimeout(r, 0));
  })().catch((err) => {
    ready = null;
    throw err;
  });
  return ready;
}

// readIfFile resolves with a file's text, or null when it does not exist or is not a file.
async function readIfFile(p) {
  try {
    const st = await fs.promises.stat(p);
    return st.isFile() ? await fs.promises.readFile(p, "utf8") : null;
  } catch {
    return null;
  }
}

// findProject walks from dir upward for the nearest project file, the way the command line
// walks from its working directory, and resolves with its text or null.
async function findProject(dir) {
  if (!dir) return null;
  for (let d = path.resolve(dir); ; ) {
    const text = await readIfFile(path.join(d, PROJECT_FILE));
    if (text !== null) return text;
    const parent = path.dirname(d);
    if (parent === d) return null;
    d = parent;
  }
}

// readVoice resolves with the personal voice file's text, or null when there is none.
function readVoice() {
  return readIfFile(path.join(os.homedir(), ".slop-chop", "voice.json"));
}

// run sends one request to the engine and resolves with its parsed result. dir is where
// project discovery starts, and opts carries as, fix, and presets.
async function run(engineDir, text, dir, opts) {
  await boot(engineDir);
  const [project, voice] = await Promise.all([findProject(dir), readVoice()]);
  const req = {
    text,
    as: opts.as || "",
    fix: Boolean(opts.fix),
    project: project || "",
    voice: voice || "",
    presets: opts.presets || [],
  };
  const res = JSON.parse(globalThis.slopEditor(JSON.stringify(req)));
  if (res.error) throw new Error(res.error);
  return res;
}

module.exports = { run, findProject };
