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
| `wn tag <id> <tag>` / `wn untag <id> <tag>` | Add or remove a tag |
| `wn list` | List items (default: undone, dependency order). Use `--done`, `--all`, `--tag x` |
| `wn depend <id> --on <id2>` | Mark dependency (rejects cycles) |
| `wn rmdepend <id> --on <id2>` | Remove dependency |
| `wn done <id> -m "..."` | Mark complete (use `--force` if dependencies not done) |
| `wn undone <id>` | Mark not complete |
| `wn log <id>` | Show history for an item |
| `wn next` | Set “next” task (first undone in dependency order) as current |
| `wn pick` | Interactively choose current task (fzf if available) |
| `wn settings` | Open `~/.config/wn/settings.json` in `$EDITOR` |
| `wn export [-o file]` | Export all items to JSON (stdout if no `-o`) |
| `wn import <file>` | Import items (use `--replace` if store already has items) |
| `wn help` / `wn completion` | Help and shell completion |

Work item IDs are 6-character hex prefixes (e.g. `af1234`). The tool finds the wn root by walking up from the current directory until it finds a `.wn` directory.

## Shell completion

```bash
# zsh
wn completion zsh > "${fpath[1]}/_wn" && compinit

# bash
wn completion bash > /etc/bash_completion.d/wn  # or ~/.local/share/bash-completion/completions/wn
```

## Optional: fzf for `wn pick`

If `fzf` is in your `PATH`, `wn pick` uses it for fuzzy selection. Otherwise a numbered list is shown and you type the number.

## Testing

```bash
go test ./...
go test ./internal/wn/... -cover   # aim for 80%+ coverage
```

Development follows red/green TDD: write tests first, see expected failures, then implement.

## License

MIT
