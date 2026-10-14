/* Pure helpers for the slop-chop extension: turning check findings into text edits. They
   hold no VS Code state, so they run under plain node and have their own tests. */
"use strict";

// byteToIndex maps a byte offset in the UTF-8 encoding of text to a JavaScript string index,
// which is what positionAt expects. Findings carry byte offsets.
function byteToIndex(text, byteOff) {
  let bytes = 0;
  for (let i = 0; i < text.length; i++) {
    if (bytes >= byteOff) return i;
    const code = text.codePointAt(i);
    bytes += code < 0x80 ? 1 : code < 0x800 ? 2 : code < 0x10000 ? 3 : 4;
    if (code > 0xffff) i++;
  }
  return text.length;
}

// editsFor turns the fixable findings of one check into replacements against text, as string
// index ranges in order. A finding with no replacement is flag-only and has no edit. One that
// overlaps an earlier edit is skipped, and so is one whose text no longer reads as found, so
// a stale or doubled finding never corrupts the document. The binary's own tidy pass is not
// run on a source file, so an edit takes the blank next to it where the replacement would
// otherwise leave one doubled: " - and" becomes ", and" and not " ,  and".
function editsFor(text, findings) {
  const sorted = findings
    .filter((f) => f.replacement !== null && f.replacement !== undefined)
    .sort((a, b) => a.offset - b.offset);
  const edits = [];
  let lastEnd = -1;
  for (const f of sorted) {
    let start = byteToIndex(text, f.offset);
    let end = start + f.match.length;
    if (text.slice(start, end) !== f.match) continue;
    if (/^[,.;:!?]/.test(f.replacement) && text[start - 1] === " " && !/[\n ]/.test(text[start - 2] || "\n")) {
      start--;
    }
    if (f.replacement.endsWith(" ") && text[end] === " ") end++;
    if (start < lastEnd) continue;
    edits.push({ start, end, replacement: f.replacement, finding: f });
    lastEnd = end;
  }
  return edits;
}

// applyEdits returns text with every edit applied. The edits come from editsFor, in order and
// without overlap.
function applyEdits(text, edits) {
  let out = "";
  let pos = 0;
  for (const e of edits) {
    out += text.slice(pos, e.start) + e.replacement;
    pos = e.end;
  }
  return out + text.slice(pos);
}

module.exports = { byteToIndex, editsFor, applyEdits };
