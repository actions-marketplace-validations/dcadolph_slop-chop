/* slop-chop for VS Code. Runs the slop-chop engine over Markdown, plain text, commit
   messages, and the comments in source files: tells show as diagnostics as you type, each one
   has a quick fix, and a command chops the document in place. Prose is rewritten whole. Source
   files are never reformatted: only the fixable tells inside their comments change, and the
   code is never read. The engine ships inside the extension as wasm and picks up your voice
   and the nearest .slop-chop.json the way the command line does. Setting slop-chop.path runs
   that binary instead. Zero dependencies. */
"use strict";

const vscode = require("vscode");
const path = require("path");
const { execFile } = require("child_process");
const engine = require("./engine");
const { byteToIndex, editsFor } = require("./fixes");

// ENGINE_DIR holds the wasm engine and Go's runtime shim, staged by make vscode.
const ENGINE_DIR = path.join(__dirname, "engine");

// LANGUAGES is the set of prose documents the extension watches.
const LANGUAGES = ["markdown", "plaintext", "git-commit"];

// CODE_LANGUAGES maps a source language id to the file type the binary reads its comments
// by. The extension never sends the file name, so this is how the binary learns it is code.
const CODE_LANGUAGES = {
  go: ".go",
  javascript: ".js",
  javascriptreact: ".jsx",
  typescript: ".ts",
  typescriptreact: ".tsx",
  python: ".py",
  rust: ".rs",
  java: ".java",
  c: ".c",
  cpp: ".cpp",
  csharp: ".cs",
  kotlin: ".kt",
  swift: ".swift",
  php: ".php",
  ruby: ".rb",
  shellscript: ".sh",
  yaml: ".yaml",
  toml: ".toml",
  lua: ".lua",
  sql: ".sql",
  dart: ".dart",
  scala: ".scala",
  perl: ".pl",
  r: ".r",
  terraform: ".tf",
  proto3: ".proto",
  fish: ".fish",
  makefile: "Makefile",
  dockerfile: "Dockerfile",
};

// WATCHED is every language id the extension can act on.
const WATCHED = [...LANGUAGES, ...Object.keys(CODE_LANGUAGES)];

// kindOf says how a document is read: "prose" for a prose language, "code" for a source
// language while comment checking is on, and null when the extension leaves it alone.
function kindOf(doc) {
  if (LANGUAGES.includes(doc.languageId)) return "prose";
  const on = vscode.workspace.getConfiguration("slop-chop").get("codeComments");
  if (CODE_LANGUAGES[doc.languageId] && on !== false) return "code";
  return null;
}

// docDir returns where project file discovery starts for a document: its own folder for a
// file on disk, else the first workspace folder, else null for no project file.
function docDir(doc) {
  if (doc.uri.scheme === "file") return path.dirname(doc.uri.fsPath);
  const folders = vscode.workspace.workspaceFolders;
  return folders && folders.length ? folders[0].uri.fsPath : null;
}

// presetNames splits the preset setting the way the --preset flag splits its value.
function presetNames() {
  const preset = vscode.workspace.getConfiguration("slop-chop").get("preset") || "";
  return preset
    .split(",")
    .map((p) => p.trim())
    .filter(Boolean);
}

// binaryPath returns the binary the user chose to run instead of the bundled engine, or ""
// for the bundled engine.
function binaryPath() {
  return vscode.workspace.getConfiguration("slop-chop").get("path") || "";
}

// check resolves with the findings report for a document's text, naming the file type for
// source so only its comments are read.
async function check(doc, kind, text) {
  const as = kind === "code" ? CODE_LANGUAGES[doc.languageId] : "";
  const bin = binaryPath();
  if (!bin) return engine.run(ENGINE_DIR, text, docDir(doc), { as, presets: presetNames() });
  const args = ["check", "--json"];
  if (as) args.push("--as", as);
  return JSON.parse(await runBinary(bin, args, text, docDir(doc)));
}

// fixProse resolves with the chopped text of a prose document.
async function fixProse(doc, text) {
  const bin = binaryPath();
  if (!bin) {
    const opts = { fix: true, presets: presetNames() };
    const res = await engine.run(ENGINE_DIR, text, docDir(doc), opts);
    return res.output ?? "";
  }
  return runBinary(bin, ["fix"], text, docDir(doc));
}

// runBinary feeds text to one subcommand of the user's binary and resolves with stdout. It
// runs in the document's folder so the binary discovers the same project file the bundled
// engine would. check exits 1 on findings, which is a normal answer here, not a failure.
function runBinary(bin, args, text, cwd) {
  const full = [...args];
  const presets = presetNames();
  if (presets.length) full.push("--preset", presets.join(","));
  const opts = { maxBuffer: 16 * 1024 * 1024 };
  if (cwd) opts.cwd = cwd;
  return new Promise((resolve, reject) => {
    const child = execFile(bin, full, opts, (err, stdout) => {
      if (err && err.code !== 1) {
        reject(new Error(String(err.message || err)));
        return;
      }
      resolve(stdout);
    });
    child.stdin.on("error", () => {});
    child.stdin.end(text);
  });
}

// rangeKey names a diagnostic range, so a quick fix can find the replacement its finding
// carried.
function rangeKey(range) {
  return `${range.start.line}:${range.start.character}-${range.end.line}:${range.end.character}`;
}

// toDiagnostics maps check findings onto VS Code diagnostics, and returns the replacement of
// each fixable one by range. text is the exact snapshot the findings were computed against,
// so their byte offsets line up with what positionAt reads.
function toDiagnostics(doc, text, findings) {
  const replacements = new Map();
  const diagnostics = findings.map((f) => {
    const startIdx = byteToIndex(text, f.offset);
    const start = doc.positionAt(startIdx);
    const end = doc.positionAt(startIdx + f.match.length);
    const msg =
      f.replacement == null
        ? `"${f.match}": ${f.rule}`
        : f.replacement === ""
          ? `"${f.match}": drop`
          : `"${f.match}" -> "${f.replacement}"`;
    const d = new vscode.Diagnostic(
      new vscode.Range(start, end),
      msg,
      vscode.DiagnosticSeverity.Information,
    );
    d.source = "slop-chop";
    d.code = f.rule;
    if (f.replacement != null) replacements.set(rangeKey(d.range), f.replacement);
    return d;
  });
  return { diagnostics, replacements };
}

// activate wires the diagnostics, the quick fixes, the chop command, and the formatter.
function activate(context) {
  const collection = vscode.languages.createDiagnosticCollection("slop-chop");
  context.subscriptions.push(collection);
  const timers = new Map();
  // replacementsByDoc holds, per open document, the fixable findings of its last check.
  const replacementsByDoc = new Map();

  // refresh re-checks one document and repaints its diagnostics. A debounced refresh can
  // land after the document closed, so a closed document is skipped rather than repainted.
  async function refresh(doc) {
    if (doc.isClosed || !WATCHED.includes(doc.languageId)) return;
    const key = doc.uri.toString();
    const kind = kindOf(doc);
    if (!kind) {
      collection.delete(doc.uri);
      replacementsByDoc.delete(key);
      return;
    }
    const version = doc.version;
    const text = doc.getText();
    try {
      const report = await check(doc, kind, text);
      // An edit landed while the check ran: its findings are for the old text, and that edit
      // scheduled its own refresh, so drop these instead of painting offsets onto new text.
      if (doc.isClosed || doc.version !== version) return;
      const { diagnostics, replacements } = toDiagnostics(doc, text, report.findings || []);
      collection.set(doc.uri, diagnostics);
      replacementsByDoc.set(key, replacements);
    } catch (err) {
      // A missing binary or a broken profile should say so once per session, not on every
      // keystroke.
      collection.delete(doc.uri);
      replacementsByDoc.delete(key);
      if (!activate.warned) {
        activate.warned = true;
        vscode.window.showWarningMessage("slop-chop: " + err.message);
      }
    }
  }

  // debounced schedules a refresh shortly after typing stops. The timer entry is removed
  // when it fires, so the map does not grow one dead key per document touched this session.
  function debounced(doc) {
    const key = doc.uri.toString();
    clearTimeout(timers.get(key));
    timers.set(
      key,
      setTimeout(() => {
        timers.delete(key);
        refresh(doc);
      }, 350),
    );
  }

  // forget cancels a document's pending refresh and drops its diagnostics and timer, so a
  // close cannot leave a scheduled refresh to repaint a closed document.
  function forget(doc) {
    const key = doc.uri.toString();
    clearTimeout(timers.get(key));
    timers.delete(key);
    replacementsByDoc.delete(key);
    collection.delete(doc.uri);
  }

  context.subscriptions.push(
    vscode.workspace.onDidOpenTextDocument(refresh),
    vscode.workspace.onDidChangeTextDocument((e) => debounced(e.document)),
    vscode.workspace.onDidCloseTextDocument(forget),
    vscode.workspace.onDidChangeConfiguration((e) => {
      if (e.affectsConfiguration("slop-chop")) vscode.workspace.textDocuments.forEach(refresh);
    }),
  );
  vscode.workspace.textDocuments.forEach(refresh);

  // chop rewrites the whole document with the fix pass. Only prose goes through it.
  function chop(doc) {
    return fixProse(doc, doc.getText());
  }

  // chopComments applies every fixable tell found inside a source file's comments, as one
  // undoable edit, and touches nothing else in the file.
  async function chopComments(editor, kind) {
    const doc = editor.document;
    const version = doc.version;
    const text = doc.getText();
    const report = await check(doc, kind, text);
    if (doc.version !== version) {
      vscode.window.showInformationMessage(
        "slop-chop: the document changed while chopping; nothing applied",
      );
      return;
    }
    const edits = editsFor(text, report.findings || []);
    if (edits.length === 0) {
      vscode.window.showInformationMessage("slop-chop: no fixable tells in the comments");
      return;
    }
    await editor.edit((b) => {
      for (const e of edits) {
        b.replace(new vscode.Range(doc.positionAt(e.start), doc.positionAt(e.end)), e.replacement);
      }
    });
  }

  context.subscriptions.push(
    vscode.commands.registerCommand("slop-chop.chop", async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) return;
      const doc = editor.document;
      const kind = kindOf(doc);
      if (!kind) return;
      const version = doc.version;
      try {
        if (kind === "code") {
          await chopComments(editor, kind);
          return;
        }
        const out = await chop(doc);
        // The chop ran on a snapshot. If the user typed while it ran, replacing the whole
        // buffer with that snapshot's result would silently discard their edits, so bail.
        if (doc.version !== version) {
          vscode.window.showInformationMessage(
            "slop-chop: the document changed while chopping; nothing applied",
          );
          return;
        }
        const full = new vscode.Range(
          doc.positionAt(0),
          doc.positionAt(doc.getText().length),
        );
        await editor.edit((b) => b.replace(full, out));
      } catch (err) {
        vscode.window.showErrorMessage("slop-chop: " + err.message);
      }
    }),
    vscode.languages.registerCodeActionsProvider(
      WATCHED,
      {
        provideCodeActions(doc, _range, ctx) {
          const replacements = replacementsByDoc.get(doc.uri.toString());
          if (!replacements) return [];
          const actions = [];
          for (const d of ctx.diagnostics) {
            if (d.source !== "slop-chop") continue;
            const replacement = replacements.get(rangeKey(d.range));
            if (replacement === undefined) continue;
            const title =
              replacement === "" ? "slop-chop: drop it" : `slop-chop: change to "${replacement}"`;
            const action = new vscode.CodeAction(title, vscode.CodeActionKind.QuickFix);
            action.diagnostics = [d];
            action.isPreferred = true;
            action.edit = new vscode.WorkspaceEdit();
            action.edit.replace(doc.uri, d.range, replacement);
            actions.push(action);
          }
          return actions;
        },
      },
      { providedCodeActionKinds: [vscode.CodeActionKind.QuickFix] },
    ),
    // The formatter is for prose only. Format-on-save must never rewrite code, so source
    // files get diagnostics, quick fixes, and the comment command, and no formatter.
    vscode.languages.registerDocumentFormattingEditProvider(LANGUAGES, {
      async provideDocumentFormattingEdits(doc) {
        const version = doc.version;
        try {
          const out = await chop(doc);
          // Edits computed against a stale version would be applied to changed text.
          if (doc.version !== version || out === doc.getText()) return [];
          const full = new vscode.Range(
            doc.positionAt(0),
            doc.positionAt(doc.getText().length),
          );
          return [vscode.TextEdit.replace(full, out)];
        } catch {
          return [];
        }
      },
    }),
  );
}

// deactivate has nothing to release; the disposables cover it.
function deactivate() {}

module.exports = { activate, deactivate };
