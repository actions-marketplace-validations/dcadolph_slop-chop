# slop-chop for VS Code

Flag and chop AI writing tells in Markdown, plain text, commit messages, and the comments in
your code. The extension runs your local `slop-chop` binary, so it uses the same profile,
presets, and voice as the command line, and your text never leaves the machine.

## What it does

- Tells show up as diagnostics while you type, with the swap each one would make.
- Each fixable tell has a quick fix. Open the lightbulb on a flagged word and take it.
- **slop-chop: Chop the slop** (command palette) rewrites a prose document in place.
- The document formatter chops prose too, so format-on-save keeps a file clean.

## Code comments

In a source file the extension reads the comments and nothing else. Go, JavaScript and
TypeScript, Python, Rust, Java, C and C++, C#, Kotlin, Swift, PHP, Ruby, shell, YAML, TOML,
Lua, SQL, Terraform, Dockerfiles, and Makefiles are covered. The code is never read, a
tell in a string or an identifier draws nothing, and format-on-save never touches a source
file.

**slop-chop: Chop the slop** on a source file applies every fixable tell inside its comments
as one undoable edit and leaves the rest of the file alone. Tells with no safe swap stay as
diagnostics for you to rewrite. Python docstrings are strings, not comments, so they are not
scanned yet.

## Install

It needs the binary first: `brew install dcadolph/tap/slop-chop`, or
`go install github.com/dcadolph/slop-chop@latest`.

Then install the extension. Search **slop-chop** in the Extensions view and pick the one from
dcadolph, or run `code --install-extension dcadolph.slop-chop`. Cursor and VSCodium find it on
Open VSX.

The extension is plain JavaScript with no dependencies, so building from source is short.
Package a vsix with `npx @vscode/vsce package` from this folder and install it via
"Extensions: Install from VSIX".

## Settings

- `slop-chop.path`: path to the binary when it is not on PATH.
- `slop-chop.preset`: preset to apply (default `cleaver`; empty for the default profile).
- `slop-chop.codeComments`: check the comments in source files (default on). Turn it off to
  keep the extension to prose.

Your `~/.slop-chop/voice.json` and a project's `.slop-chop.json` apply on their own, exactly
as they do on the command line.
