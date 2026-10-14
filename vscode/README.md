# slop-chop for VS Code

Flag and chop AI writing tells in Markdown, plain text, and commit messages. The extension
runs your local `slop-chop` binary, so it uses the same profile, presets, and voice as the
command line, and your text never leaves the machine.

## What it does

- Tells show up as diagnostics while you type, with the swap each one would make.
- **slop-chop: Chop the slop** (command palette) rewrites the document in place.
- The document formatter chops too, so format-on-save keeps a file clean.

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

Your `~/.slop-chop/voice.json` and a project's `.slop-chop.json` apply on their own, exactly
as they do on the command line.
