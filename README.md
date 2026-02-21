# wn: "What's Next"

CLI for tracking work items locally. Use it from your project directory or from agents (e.g. Cursor, Claude Code) to keep a queue of tasks.

## Install

```bash
go build -o wn ./cmd/wn
# or
go install ./cmd/wn
```

Requires **Go 1.26** or later.

## Quick start

```bash
wn init
wn add -m "introduce feature X"
wn list
wn next
wn done abc123 -m "Completed in git commit ca1f722"
```

## Commands

| Command | Description |
|--------|-------------|
| `wn` | Show current task (or suggest `wn pick` / `wn next`) |
| `wn init` | Create `.wn/` in the current directory |
| `wn add -m "..."` | Add a work item (use `-t tag` for tags; omit `-m` to use `$EDITOR`) |
| `wn rm <id>` | Remove a work item |
| `wn edit <id>` | Edit description in `$EDITOR` |
| `wn tag [id] <tag>` / `wn untag [id] <tag>` | Add or remove a tag. Use `wn tag -i <tag>` to pick items with fzf and toggle the tag on each selected item |
| `wn list` | List items (default: available undone, dependency order; in-progress excluded until expiry). Use `--done`, `--all`, `--tag x`, `--json` for machine-readable output |
| `wn depend [id] --on <id2>` | Mark dependency (rejects cycles). Use `-i` to pick the depended-on item from undone work items (fzf or numbered list) |
| `wn rmdepend [id] --on <id2>` | Remove dependency. Use `-i` to pick which dependency to remove (fzf or numbered list) |
| `wn order [id] --set <n>` / `--unset` | Set or clear optional backlog order (lower = earlier when deps don't define order) |
| `wn done <id> -m "..."` | Mark complete (use `--force` if dependencies not done) |
| `wn undone <id>` | Mark not complete |
| `wn claim [id] --for 30m` | Mark in progress (item leaves undone list until expiry or release; optional `--by` for logging) |
| `wn release [id]` | Clear in progress (return item to undone list) |
| `wn log <id>` | Show history for an item |
| `wn desc [id]` | Print prompt-ready description (use `--json` for machine-readable) |
| `wn show [id]` | Output one work item as JSON (full item; omit id for current) |
| `wn next` | Set “next” task (first available undone in dependency order) as current |
| `wn pick` | Interactively choose current task (fzf if available) |
| `wn settings` | Open `~/.config/wn/settings.json` in `$EDITOR` |
| `wn export [-o file]` | Export all items to JSON (stdout if no `-o`) |
| `wn import <file>` | Import items (use `--replace` if store already has items) |
| `wn help` / `wn completion` | Help and shell completion |
| `wn mcp` | Run MCP server on stdio (for Cursor and other MCP clients) |

Work item IDs are 6-character hex prefixes (e.g. `af1234`). The tool finds the wn root by walking up from the current directory until it finds a `.wn` directory.

## Shell completion

```bash
# zsh
wn completion zsh > "${fpath[1]}/_wn" && compinit

# bash
wn completion bash > /etc/bash_completion.d/wn  # or ~/.local/share/bash-completion/completions/wn
```

## MCP server

To use wn from Cursor (or another MCP client), add an MCP server that runs `wn mcp`. The client spawns the process with the project directory as cwd, so the tools operate on that project's work list. The process runs only while the client is connected—no long-lived daemon.

Tools: `wn_add`, `wn_list`, `wn_done`, `wn_undone`, `wn_desc`, `wn_claim`, `wn_release`, `wn_next`, `wn_order`.

## Optional: fzf for interactive commands

If `fzf` is in your `PATH`:
- **`wn pick`** uses it for fuzzy selection of the current task. Otherwise a numbered list is shown and you type the number.
- **`wn tag -i <tag>`** uses fzf with multi-select (Tab to select, Enter to confirm); the list shows each item’s tags. Selected items have the tag toggled (added if missing, removed if present). Without fzf, a numbered list is shown—enter space-separated numbers to select items.
- **`wn depend -i`** uses fzf to pick the depended-on item from undone work items; the selected item is used as the `--on` target. Without fzf, a numbered list is shown. The current task (or the given id) is the item that will gain the dependency; the current item is excluded from the list.
- **`wn rmdepend -i`** uses fzf to pick which dependency to remove from the current task (or the given id); the list shows the item’s current dependencies. Without fzf, a numbered list is shown.

## Testing

```bash
go test ./...
go test ./internal/wn/... -cover   # aim for 80%+ coverage
```

Development follows red/green TDD: write tests first, see expected failures, then implement.

## License

MIT
