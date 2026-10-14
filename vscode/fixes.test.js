"use strict";

const test = require("node:test");
const assert = require("node:assert");
const { byteToIndex, editsFor, applyEdits } = require("./fixes");

// finding builds a check finding the way the binary reports it, with a byte offset.
function finding(text, match, replacement) {
  const at = text.indexOf(match);
  return { rule: "test", match, replacement, offset: Buffer.byteLength(text.slice(0, at)) };
}

test("byteToIndex counts bytes, not characters", () => {
  const text = "café \u{1F600} done";
  // The e-acute is two bytes and the emoji is four, so "done" starts at byte 11.
  assert.strictEqual(byteToIndex(text, 11), text.indexOf("done"));
  assert.strictEqual(byteToIndex(text, 0), 0);
  assert.strictEqual(byteToIndex(text, 999), text.length);
});

test("editsFor skips flag-only findings", () => {
  const text = "a robust plan";
  assert.deepStrictEqual(editsFor(text, [finding(text, "robust", null)]), []);
  assert.deepStrictEqual(editsFor(text, [{ ...finding(text, "robust", null), replacement: undefined }]), []);
});

test("editsFor keeps an empty replacement, which drops the text", () => {
  const text = "a really good plan";
  const edits = editsFor(text, [finding(text, "really ", "")]);
  assert.strictEqual(edits.length, 1);
  assert.strictEqual(applyEdits(text, edits), "a good plan");
});

test("editsFor orders edits and applies them from multibyte text", () => {
  const text = "été: it is important to note that we delve in";
  const findings = [
    finding(text, "delve in", "look in"),
    finding(text, "it is important to note that ", ""),
  ];
  const edits = editsFor(text, findings);
  assert.deepStrictEqual(
    edits.map((e) => e.start),
    [...edits.map((e) => e.start)].sort((a, b) => a - b),
  );
  assert.strictEqual(applyEdits(text, edits), "été: we look in");
});

test("editsFor absorbs the blank next to a replacement that would double it", () => {
  const spaced = "a cache — and it is fast";
  const spacedEdits = editsFor(spaced, [finding(spaced, "—", ", ")]);
  assert.strictEqual(applyEdits(spaced, spacedEdits), "a cache, and it is fast");

  const tight = "a cache—and it is fast";
  const tightEdits = editsFor(tight, [finding(tight, "—", ", ")]);
  assert.strictEqual(applyEdits(tight, tightEdits), "a cache, and it is fast");
});

test("editsFor leaves indentation alone when a replacement opens with punctuation", () => {
  const text = "x\n    — note";
  const edits = editsFor(text, [finding(text, "—", ", ")]);
  assert.strictEqual(applyEdits(text, edits), "x\n    , note");
});

test("editsFor drops an edit that overlaps an earlier one", () => {
  const text = "a comprehensive and robust plan";
  const findings = [
    finding(text, "comprehensive and robust", "sound"),
    finding(text, "robust", "firm"),
  ];
  const edits = editsFor(text, findings);
  assert.strictEqual(edits.length, 1);
  assert.strictEqual(applyEdits(text, edits), "a sound plan");
});

test("editsFor drops a finding whose text no longer matches", () => {
  const text = "a robust plan";
  const stale = { rule: "test", match: "seamless", replacement: "smooth", offset: 2 };
  assert.deepStrictEqual(editsFor(text, [stale]), []);
});
