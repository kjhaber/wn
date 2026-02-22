# wn: "What's Next"

CLI for tracking work items locally. Use it from your project directory or from agents (e.g. Cursor, Claude Code) to keep a queue of tasks.

## About
Lately I've been working more and more with LLM coding agents and refining my workflow for using them effectively. (Hello from early 2026.)  I've been composing prompts and keeping todo lists in a plain Markdown file, managing dependencies and grouping items and tracking what's done manually.  That works OK for a small number of items at a time, but an actual tool for tracking work items seemed like a natural next step.  Using a heavyweight issue tracker like GitHub issues or JIRA would be overkill for that when it's just me.

There are tons of other "lightweight todo list" tools like 'wn' out there, some of which are even geared toward agentic workflows.  But in a fit of NIH and because I thought it'd be fun, I used Cursor to create `wn` to fit the way I think and work.  I'm still learning Golang and LLM coding agents like Cursor, so this kind of project is perfect for that.  (The fact that there are tons of other todo list apps out there makes LLM coding agents very good at this kind of program!)

In its current state I already find `wn` useful.  I have ideas to improve it for both human and LLM agents to work with it, and I use `wn` to track those ideas.  For example `wn` has MCP support and a temporary "claim" feature to allow agents to treat the work item list as a queue.  So consider `wn` experimental, but feedback and PRs are welcome.


## Install

```bash
go build -o wn ./cmd/wn
# or
go install ./cmd/wn
```

Or use the Makefile: `make build` builds the binary to `build/wn`, `make test` runs tests, and `make` (or `make all`) runs format check, lint, coverage, and build.

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
| `wn list` | List items (default: undone including review-ready, dependency order; in-progress excluded until expiry). Status column: undone, review-ready, claimed, done. Use `--sort 'updated:desc,priority,tags'` to sort; `--done`, `--all`, `--tag x`, `--json` for machine-readable output |
| `wn depend [id] --on <id2>` | Mark dependency (rejects cycles). Use `-i` to pick the depended-on item from undone work items (fzf or numbered list) |
| `wn rmdepend [id] --on <id2>` | Remove dependency. Use `-i` to pick which dependency to remove (fzf or numbered list) |
| `wn order [id] --set <n>` / `--unset` | Set or clear optional backlog order (lower = earlier when deps don't define order) |
| `wn done <id> -m "..."` | Mark complete (use `--force` if dependencies not done) |
| `wn undone <id>` | Mark not complete |
| `wn claim [id] --for 30m` | Mark in progress (item leaves undone list until expiry or release; optional `--by` for logging) |
| `wn release [id]` | Clear in progress and mark item **review-ready** (excluded from `wn next` and agent claim until you mark done) |
| `wn log <id>` | Show history for an item |
| `wn note add <name> [id] -m "..."` | Add or update a note by name (e.g. pr-url, issue-number); omit id for current task, omit `-m` to use `$EDITOR`. Names: alphanumeric, /, _, -, up to 32 chars |
| `wn note list [id]` | List notes on an item (name, created, body), ordered by create time |
| `wn note edit [id] <name> [-m "..."]` | Edit a note by name; omit `-m` to use `$EDITOR` with current body |
| `wn note rm [id] <name>` | Remove a note by name |
| `wn desc [id]` | Print prompt-ready description (use `--json` for machine-readable) |
| `wn show [id]` | Output one work item as JSON (full item; omit id for current) |
| `wn next` | Set “next” task (first **available** undone in dependency order) as current; excludes review-ready and in-progress. Use `--claim 30m` to also claim it in the same step (optional `--claim-by` for logging) |
| `wn pick` | Interactively choose current task (fzf if available) |
| `wn settings` | Open `~/.config/wn/settings.json` in `$EDITOR`. Set `"sort": "updated:desc,priority,tags"` for default list/fzf order |
| `wn export [-o file]` | Export all items to JSON (stdout if no `-o`) |
| `wn import <file>` | Import items (use `--replace` if store already has items) |
| `wn help` / `wn completion` | Help and shell completion |
| `wn mcp` | Run MCP server on stdio (for Cursor and other MCP clients) |

Work item IDs are 6-character hex prefixes (e.g. `af1234`). The tool finds the wn root by walking up from the current directory until it finds a `.wn` directory.

**Review-ready:** When you or an agent runs `wn release`, the item is marked *review-ready*: it stays undone but is excluded from `wn next` and from agent claim (and from MCP `wn_next`), so it won't be picked again. It still appears in `wn list` for human review. Mark it done when the work is merged or accepted (e.g. merge to main).

## Shell completion

```bash
# zsh
wn completion zsh > "${fpath[1]}/_wn" && compinit

# bash
wn completion bash > /etc/bash_completion.d/wn  # or ~/.local/share/bash-completion/completions/wn
```

## MCP server

To use wn from Cursor (or another MCP client), add an MCP server that runs `wn mcp`. The process runs only while the client is connected—no long-lived daemon.

**Project root and guardrail:** You can lock the server to a single project so MCP callers cannot access other `.wn` directories:

- **Spawn-time argument:** `wn mcp /path/to/project` — the server uses that path as the project root and ignores the per-request `root` parameter. Use this when your client can pass the workspace path as an argument (e.g. project-level config with a path, or when Cursor adds variable substitution like `${workspaceFolder}`).
- **Environment variable:** Set `WN_ROOT` to the project root before starting (e.g. in a wrapper script or in the MCP config `env` if your client supports it). Same guardrail: the server uses `WN_ROOT` and ignores request `root`.

If neither is set, each tool accepts an optional `root` argument in the request; if omitted, the server finds the wn root from the process cwd.

TL;DR: For Cursor set `~/.cursor/mcp.json` to
```
{
  "mcpServers": {
    "wn": {
      "command": "wn",
      "args": ["mcp", "${workspaceFolder}"]
    }
  }
}
```


Tools: `wn_add`, `wn_list`, `wn_done`, `wn_undone`, `wn_desc`, `wn_show`, `wn_claim`, `wn_release`, `wn_next`, `wn_order`, `wn_depend`, `wn_rmdepend`. For `wn_next`, pass optional `claim_for` (e.g. `30m`) to atomically claim the returned item so concurrent workers don't double-assign.

## Sort order

List order and fzf pick order can be controlled by:

- **`wn list --sort '...'`** — Comma-separated sort keys; each key may be suffixed with `:asc` or `:desc` (default `:asc`). Keys: `created`, `updated`, `priority` (backlog order), `alpha` (description), `tags` (group by tags). Example: `wn list --sort 'updated:desc,priority,tags'`.
- **Default in settings** — In `~/.config/wn/settings.json`, set `"sort": "updated:desc,priority"` (or any valid sort spec). This applies to `wn list` when `--sort` is not given, and to fzf/numbered lists for `wn pick`, `wn tag -i`, `wn depend -i`, and `wn rmdepend -i`.

When no sort preference is set, `wn list` uses dependency order (topological) for undone items.

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
