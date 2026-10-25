/* Parity tests for the bundled engine: on the same input, the wasm engine must report what
   the slop-chop binary reports, byte offsets included, and chop to the same text. Run after
   make vscode and go build -o slop-chop . at the repo root. HOME points at an empty folder so
   neither side picks up the personal voice of the machine running the tests. */
"use strict";

const test = require("node:test");
const assert = require("node:assert");
const fs = require("fs");
const os = require("os");
const path = require("path");
const { execFileSync } = require("child_process");

const ROOT = path.join(__dirname, "..");
const BIN = path.join(ROOT, "slop-chop");
const ENGINE_DIR = path.join(__dirname, "engine");
const HOME = fs.mkdtempSync(path.join(os.tmpdir(), "slop-chop-home-"));
process.env.HOME = HOME;

const engine = require("./engine");

// binary runs the slop-chop binary with text on stdin and returns stdout. check exits 1 on
// findings, which is an answer, not a failure.
function binary(args, text, cwd) {
  try {
    return execFileSync(BIN, args, { input: text, cwd, env: { ...process.env, HOME } }).toString();
  } catch (err) {
    if (err.status === 1) return err.stdout.toString();
    throw err;
  }
}

// read returns a repo file's text.
function read(rel) {
  return fs.readFileSync(path.join(ROOT, rel), "utf8");
}

const PRESETS = ["cleaver"];

// Test 0: Prose findings match the binary on the repo's own docs.
test("prose findings match the binary", async () => {
  for (const rel of ["README.md", "docs/PROFILE.md", "docs/SCORING.md"]) {
    const text = read(rel);
    const got = await engine.run(ENGINE_DIR, text, HOME, { presets: PRESETS });
    const want = JSON.parse(binary(["check", "--json", "--preset", "cleaver"], text, HOME));
    assert.deepStrictEqual(got.findings, want.findings, rel);
  }
});

// Test 1: Source findings match the binary, which reads only the comments.
test("source findings match the binary", async () => {
  const cases = [
    ["sanitize/ignore.go", ".go"],
    ["vscode/extension.js", ".js"],
  ];
  for (const [rel, as] of cases) {
    const text = read(rel);
    const got = await engine.run(ENGINE_DIR, text, HOME, { as, presets: PRESETS });
    const want = JSON.parse(binary(["check", "--json", "--as", as, "--preset", "cleaver"], text, HOME));
    assert.deepStrictEqual(got.findings, want.findings, rel);
    assert.strictEqual(got.scanned, true, rel);
  }
});

// Test 2: Columns match the binary when non-ASCII code sits before a comment on the line.
test("source columns match the binary on non-ASCII code", async () => {
  const text = 'name := "café naïve" // a robust plan\n';
  const got = await engine.run(ENGINE_DIR, text, HOME, { as: ".go" });
  const want = JSON.parse(binary(["check", "--json", "--as", ".go"], text, HOME));
  assert.ok(want.findings.length > 0, "control finding missing");
  assert.deepStrictEqual(got.findings, want.findings);
});

// Test 3: The chopped prose matches the binary's fix byte for byte, non-ASCII included.
test("chop output matches the binary", async () => {
  const text = read("README.md") + "\nIn summary, a robust plan — naïve, café.\n";
  const got = await engine.run(ENGINE_DIR, text, HOME, { fix: true, presets: PRESETS });
  assert.strictEqual(got.output, binary(["fix", "--preset", "cleaver"], text, HOME));
});

// Test 4: The nearest project file above the document applies, as it does for the binary
// run from that folder.
test("project file discovery matches the binary", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "slop-chop-project-"));
  const nested = path.join(dir, "a", "b");
  fs.mkdirSync(nested, { recursive: true });
  fs.writeFileSync(path.join(dir, ".slop-chop.json"), JSON.stringify({ allow: ["robust"] }));
  const text = "A robust plan that will leverage the team.\n";
  const got = await engine.run(ENGINE_DIR, text, nested, {});
  const want = JSON.parse(binary(["check", "--json"], text, nested));
  assert.deepStrictEqual(got.findings, want.findings);
  assert.ok(!got.findings.some((f) => f.match === "robust"), "allowed word was flagged");
  assert.ok(got.findings.some((f) => f.match === "leverage"), "control word was not flagged");
});

// Test 5: The personal voice applies on both sides.
test("voice matches the binary", async () => {
  const voiceDir = path.join(HOME, ".slop-chop");
  fs.mkdirSync(voiceDir, { recursive: true });
  fs.writeFileSync(path.join(voiceDir, "voice.json"), JSON.stringify({ keep: ["leverage"] }));
  try {
    const text = "A robust plan that will leverage the team.\n";
    const got = await engine.run(ENGINE_DIR, text, HOME, { presets: PRESETS });
    const want = JSON.parse(binary(["check", "--json", "--preset", "cleaver"], text, HOME));
    assert.deepStrictEqual(got.findings, want.findings);
    assert.ok(!got.findings.some((f) => f.match === "leverage"), "kept word was flagged");
  } finally {
    fs.rmSync(voiceDir, { recursive: true, force: true });
  }
});

// Test 6: A broken project file is reported, not silently ignored.
test("a broken project file is an error", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "slop-chop-broken-"));
  fs.writeFileSync(path.join(dir, ".slop-chop.json"), "{not json");
  await assert.rejects(engine.run(ENGINE_DIR, "text\n", dir, {}), /\.slop-chop\.json/);
});

// Test 7: A source type with no comment syntax reads nothing.
test("an unscannable type reads nothing", async () => {
  const got = await engine.run(ENGINE_DIR, '{"a": "robust"}', HOME, { as: ".json" });
  assert.deepStrictEqual(got.findings, []);
  assert.strictEqual(got.scanned, false);
});
