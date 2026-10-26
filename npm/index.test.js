/* Parity tests for the npm package: a voice passed as an option must chop and score the way
   the slop-chop binary does with the same voice in its voice file. Run after make
   npm-package and go build -o slop-chop . at the repo root. HOME points at a fresh folder
   that holds only the test's voice file. */
"use strict";

const test = require("node:test");
const assert = require("node:assert");
const fs = require("fs");
const os = require("os");
const path = require("path");
const { execFileSync } = require("child_process");

const slop = require("./index");

const BIN = path.join(__dirname, "..", "slop-chop");

// binary runs the slop-chop binary with text on stdin under a HOME holding voice, and
// returns stdout. check exits 1 on findings, which is an answer, not a failure.
function binary(args, text, voice) {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "slop-chop-npm-"));
  fs.mkdirSync(path.join(home, ".slop-chop"));
  fs.writeFileSync(path.join(home, ".slop-chop", "voice.json"), JSON.stringify(voice));
  try {
    return execFileSync(BIN, args, { input: text, cwd: home, env: { ...process.env, HOME: home } }).toString();
  } catch (err) {
    if (err.status === 1) return err.stdout.toString();
    throw err;
  } finally {
    fs.rmSync(home, { recursive: true, force: true });
  }
}

const TEXT = "We utilize a robust plan to ship the Gizmo and leverage the gizmo.\n";
const VOICE = { keep: ["robust"], prefer: { utilize: "wield" }, avoid: ["gizmo"] };

// Test 0: A voice chops to the same text and reports the same findings as the binary.
test("voice chop matches the binary", async () => {
  const got = await slop.chop(TEXT, { voice: VOICE });
  assert.strictEqual(got.output, binary(["fix", "--preset", "cleaver"], TEXT, VOICE));
  const want = JSON.parse(binary(["check", "--json", "--preset", "cleaver"], TEXT, VOICE));
  assert.deepStrictEqual(got.findings, want.findings);
});

// Test 1: An avoided word is flagged however it is capitalized, as a voice file's is.
test("voice avoid flags a capitalized word", async () => {
  const got = await slop.chop(TEXT, { voice: VOICE });
  const flagged = got.findings.filter((f) => f.rule === "word:gizmo").map((f) => f.match);
  assert.deepStrictEqual(flagged, ["Gizmo", "gizmo"]);
});

// Test 2: A voice scores the same as the binary.
test("voice score matches the binary", async () => {
  const got = await slop.chop(TEXT, { voice: VOICE });
  const want = JSON.parse(binary(["score", "--json", "--preset", "cleaver"], TEXT, VOICE));
  assert.deepStrictEqual(got.score, want);
});

// Test 3: No voice chops the way the binary does with an empty voice file.
test("no voice matches the binary", async () => {
  const got = await slop.chop(TEXT);
  assert.strictEqual(got.output, binary(["fix", "--preset", "cleaver"], TEXT, {}));
  const want = JSON.parse(binary(["check", "--json", "--preset", "cleaver"], TEXT, {}));
  assert.deepStrictEqual(got.findings, want.findings);
});
