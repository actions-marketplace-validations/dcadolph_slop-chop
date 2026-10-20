# Editor plugin (LSP)

slop-chop can run as a Language Server, so any editor that speaks LSP can flag and chop slop
as you write. Tells show up as diagnostics, and the fix pass is offered two ways: a "Chop the
slop" code action, and document formatting, so format-on-save chops the file. Source files work
too, read as their comments and Python docstrings. It uses the same profile, presets, and
[voice](VOICE.md) as the command line.

## Run it

```
slop-chop lsp
```

It speaks the protocol on stdin and stdout. Point your editor's LSP client at that command for
Markdown, plain text, commit messages, and source files. Add `--preset cleaver` for the
aggressive swaps, or `--voice path.json` for a specific voice. A `~/.slop-chop/voice.json` is
picked up on its own.

## JetBrains IDEs

Install the slop-chop plugin from the JetBrains Marketplace: in IntelliJ IDEA, PyCharm,
WebStorm, and the rest, open Settings, Plugins, Marketplace, and search **slop-chop**. It wires
the language server for you, launching `slop-chop lsp` over Markdown, plain text, and source
files. In a source file it reads only comments (and Python docstrings) and offers a quick fix per
tell instead of a whole-file chop. Tells show as inspections. You still need the `slop-chop`
binary on your PATH. To wire it by hand instead, install the LSP4IJ plugin and register a
server that runs `slop-chop lsp`.

## Neovim

With the built-in client:

```lua
vim.api.nvim_create_autocmd("FileType", {
  pattern = { "markdown", "text", "gitcommit", "go" },
  callback = function()
    vim.lsp.start({
      name = "slop-chop",
      cmd = { "slop-chop", "lsp", "--preset", "cleaver" },
      root_dir = vim.fn.getcwd(),
    })
  end,
})
```

Diagnostics appear inline. Run `vim.lsp.buf.code_action` for "Chop the slop", or
`vim.lsp.buf.format` to chop the whole file.

## Helix

In `languages.toml`:

```toml
[language-server.slop-chop]
command = "slop-chop"
args = ["lsp", "--preset", "cleaver"]

[[language]]
name = "markdown"
language-servers = ["slop-chop"]

[[language]]
name = "go"
language-servers = ["gopls", "slop-chop"]
```

Listing a language's servers in Helix replaces its default list, so keep the usual server in it.

## VS Code

A ready-made extension is on the
[Visual Studio Marketplace](https://marketplace.visualstudio.com/items?itemName=dcadolph.slop-chop)
and Open VSX, with its source under
[`vscode/`](https://github.com/dcadolph/slop-chop/tree/main/vscode). It runs the binary
directly: diagnostics as you type, a "Chop the slop" command, and a document formatter so
format-on-save chops the file. It also reads the comments in source files and offers a quick
fix on each fixable tell. Its README covers the install. Rolling your own client
instead works too: point it at `slop-chop lsp` with a document selector for `markdown`,
`plaintext`, and `git-commit`, and add any source languages you want read as comments.

## What it provides

- Diagnostics for every tell, with the rule name and the swap it would make.
- A "Chop the slop" code action that rewrites a prose document whole.
- Formatting that chops a prose document on demand or on save.
- Quick fixes on source files, one per tell with a safe swap, each replacing just that
  span. Tells with no swap stay diagnostics. A source file gets no whole-document chop and
  formatting returns no edits, so format-on-save never touches code. The server picks the
  treatment from the file extension, and data files like `.json` and `.csv` get nothing.
