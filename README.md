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
| `wn rm [id ...]` | Remove work item(s). Omit id to show an interactive list (fzf or numbered) with multi-select; pass one or more ids to remove those directly. |
| `wn edit <id>` | Edit description in `$EDITOR` |
| `wn tag add <tag-name> [--wid <id>]` | Add a tag. Omit `--wid` to use the current task. Use `-i` to pick items with fzf and toggle the tag on each. |
| `wn tag rm <tag-name> [--wid <id>]` | Remove a tag. Omit `--wid` to use the current task. |
| `wn tag list [--wid <id>]` | List tags on the work item (one per line). Omit `--wid` to use the current task. |
| `wn list` | List items (default: undone; dependency order). Status column: undone, claimed, review, done, closed, suspend. Use `--review-ready`/`--rr` to list only review items; `--done`, `--all`, `--tag x`, `--json` for machine-readable output; `--sort 'updated:desc,priority,tags'` to sort; `--limit N` and optional `--offset N` for a bounded window. |
| `wn show [id]` | Show a work item (human-readable by default; `--json` for machine-readable; `--plain` for description text only, suitable for pasting into an agent). Omit id for current task. Control fields with `--fields title,body,status,deps,notes,log` or `--all`. |
| `wn depend add --on <id> [--wid <id>]` | Add dependency (rejects cycles). Omit `--wid` for current task. Use `-i` to pick the depended-on item. |
| `wn depend rm --on <id> [--wid <id>]` | Remove dependency. Omit `--wid` for current task. Use `-i` to pick which dependency to remove. |
| `wn depend list [--wid <id>]` | List dependency ids of the work item, one per line. Omit `--wid` for current task. |
| `wn done <id> -m "..."` | Mark complete (use `--force` if dependencies not done) |
| `wn undone <id>` | Mark not complete |
| `wn status <state> [id]` | Set work item status. State: undone, claimed, review, done, closed, suspend. Omit id for current task. Use `--for 30m` when setting to claimed; `-m "..."` for done/closed/suspend. Use `--duplicate-of <id>` when setting to closed. |
| `wn claim [id] [--for 30m]` | Mark in progress (item leaves undone list until expiry or release). Omit `--for` to use default 1h; optional `--by` for logging. |
| `wn release [id]` | Clear in progress and mark item **review-ready** (excluded from `wn next` and agent claim until you mark done). |
| `wn review-ready [id]` / `wn rr [id]` | Set item to review-ready state directly. |
| `wn next` | Set the first available undone item (dependency order) as current; excludes review-ready and in-progress. Use `--tag <tag>` to filter (or set `next.tag` in settings). Use `--claim 30m` to also claim it. |
| `wn pick [id]` | Interactively choose current task (fzf if available). Pass an id to set current directly. Filter: `--undone` (default), `--done`, `--all`, `--rr`/`--review-ready`. Use `--picker fzf\|numbered` to override picker. |
| `wn worktree [id]` | Claim a work item, create its branch and git worktree, and print the worktree path to stdout. Omit id to use current task; use `--next` to claim next from the queue. See [Worktree workflow](#worktree-workflow). |
| `wn do [id]` | Claim a work item, set up its worktree, run the configured agent command, commit any changes, and release. Omit id to use current task; use `--next` to claim next from the queue; use `--loop` to process items continuously. See [Headless agent runner](#headless-agent-runner-wn-do). |
| `wn cleanup set-merged-review-items-done` | Check all review-ready items; mark done if their `branch` note has been merged to the current branch. Use `--dry-run` to preview; `-b main` to check against a specific ref. |
| `wn cleanup close-done-items [--age 30d]` | Close items that have been in **done** state longer than the configured age. Use `--dry-run` to preview. |
| `wn merge [--wid <id>]` | Merge a review-ready item's branch into main: rebase, merge, validate (e.g. `make`), mark done, delete branch. Omit `--wid` for current task. Use `--main-branch` and `--validate` to override defaults. |
| `wn log <id>` | Show history for an item. |
| `wn note add <name> [id] -m "..."` | Add or update a note by name (e.g. pr-url, issue-number); omit id for current task, omit `-m` to use `$EDITOR`. Names: alphanumeric, /, _, -, up to 32 chars. |
| `wn note list [id]` | List notes on an item (name, created, body), ordered by create time. |
| `wn note show [id] <name>` | Print the raw body of a named note; omit id for current task. Useful for scripting, e.g. `git checkout $(wn note show branch)`. |
| `wn note edit [id] <name> [-m "..."]` | Edit a note by name; omit `-m` to use `$EDITOR` with current body. |
| `wn note rm [id] <name>` | Remove a note by name. |
| `wn settings [--project]` | Open settings in `$EDITOR`. Default: user-level `~/.config/wn/settings.json`. Use `--project` for project-level `.wn/settings.json`. |
| `wn export [-o file]` | Export all items to JSON (stdout if no `-o`). |
| `wn import <file>` | Import items from JSON export. When store has items, use `--append` (add/merge) or `--replace` (replace all). |
| `wn mcp` | Run MCP server on stdio (for Cursor and other MCP clients). |
| `wn help` / `wn completion` | Help and shell completion. |

Work item IDs are 6-character hex prefixes (e.g. `af1234`). The tool finds the wn root by walking up from the current directory until it finds a `.wn` directory.

**Work item status:** Each item has one of six statuses. Use `wn status <state> [id]` to set any state (omit id for current task). `wn done` and `wn undone` are shortcuts for the common cases.

| Status | Description |
|--------|-------------|
| **undone** | Not complete; available for `wn next` and agent claim. Default for new items. |
| **claimed** | In progress—someone has claimed it until a duration expires or they run `wn release`. Excluded from `wn next` and claim until expiry. |
| **review** | Work is done but not yet accepted (e.g. PR open). Excluded from `wn next` and claim; use `wn list --rr` to see review items. Set by `wn release` or `wn review-ready` / `wn rr`. Mark **done** when merged or accepted. |
| **done** | Completed and accepted. Use `wn done` or `wn status done`. |
| **closed** | Completed and closed (e.g. archived). Terminal state. |
| **suspend** | Deferred—not ready to implement or not sure you want to. Like done (excluded from next/claim) but not retired to closed; use for ideas you might revisit. |

**Review-ready:** When you or an agent runs `wn release`, the item is marked *review-ready*: it stays in the list but is excluded from `wn next` and agent claim so it won't be picked again. Use `wn list --rr` to see review-ready items. Mark it done when work is merged (`wn done`, `wn merge`, or `wn cleanup set-merged-review-items-done`).

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

- **Spawn-time argument:** `wn mcp /path/to/project` — the server uses that path as the project root and ignores the per-request `root` parameter.
- **Environment variable:** Set `WN_ROOT` to the project root before starting. Same guardrail.

If neither is set, each tool accepts an optional `root` argument; if omitted, the server finds the wn root from the process cwd.

TL;DR: For Cursor set `~/.cursor/mcp.json` to
```json
{
  "mcpServers": {
    "wn": {
      "command": "wn",
      "args": ["mcp", "${workspaceFolder}"]
    }
  }
}
```

Tools: `wn_add`, `wn_list`, `wn_done`, `wn_undone`, `wn_desc`, `wn_show`, `wn_item`, `wn_claim`, `wn_release`, `wn_next`, `wn_depend`, `wn_rmdepend`, `wn_note_add`, `wn_note_edit`, `wn_note_rm`, `wn_duplicate`. Use `wn_item` with a required id to get full item JSON and notes. For `wn_claim`, omit `for` to use default 1h so agents can renew without losing context. For `wn_next`, pass optional `tag` to return the next undone item with that tag, and optional `claim_for` to atomically claim it. For `wn_list`, pass `limit` and optional `offset` or `cursor` for a bounded window. For `wn_add`, pass optional `depends_on` (array of item IDs) to preserve queue order. Use `wn_duplicate` to mark an item as a duplicate of another (sets status to closed, adds `duplicate-of` note).

## Settings

Settings live in `~/.config/wn/settings.json` (user-level) and optionally `.wn/settings.json` in your project (project settings override user settings field by field). Open with `wn settings` or `wn settings --project`.

```json
{
  "sort": "tags,priority,updated,alpha",
  "picker": "fzf",

  "next": {
    "tag": "agent"
  },

  "worktree": {
    "base": "../worktrees",
    "branch_prefix": "keith/",
    "default_branch": "main",
    "claim": "2h"
  },

  "agent": {
    "cmd": "cursor agent --print --trust --approve-mcps \"{{.Prompt}}\"",
    "prompt": "{{.Description}}",
    "delay": "10s",
    "poll": "60s",
    "leave_worktree": true
  },

  "show": {
    "default_fields": "title,body,deps,notes"
  },

  "cleanup": {
    "close_done_items_age": "30d"
  }
}
```

| Key | Description |
|-----|-------------|
| `sort` | Default sort order for `wn list`, `wn pick`, and interactive lists. See [Sort order](#sort-order). |
| `picker` | Interactive picker: `"fzf"` (always use fzf), `"numbered"` (always use numbered list), or omit for auto-detect (fzf if in PATH). Overridden by `--picker` flag or `WN_PICKER` env var. |
| `next.tag` | Only consider items with this tag when selecting the next item (`wn next`, `wn worktree --next`, `wn do --next/--loop`). Overridden by `--tag` flag. |
| `worktree.base` | Base directory for git worktrees. Default: parent of the main worktree. |
| `worktree.branch_prefix` | Prefix for generated branch names (e.g. `"keith/"` → `keith/wn-abc123-add-feature`). |
| `worktree.default_branch` | Override default branch detection (e.g. `"main"`). |
| `worktree.claim` | How long to claim an item when setting up a worktree (e.g. `"2h"`). |
| `agent.cmd` | Command template for the agent. `{{.Prompt}}` is replaced by the prompt; `{{.Worktree}}` and `{{.Branch}}` are also available. Required for `wn do`. |
| `agent.prompt` | Prompt template (default `{{.Description}}`). Fields: `{{.ItemID}}`, `{{.Description}}`, `{{.FirstLine}}`, `{{.Worktree}}`, `{{.Branch}}`. |
| `agent.delay` | Delay between items in the orchestrator loop (e.g. `"10s"`). |
| `agent.poll` | Poll interval when the queue is empty (e.g. `"60s"`). |
| `agent.leave_worktree` | If true, keep the worktree after the agent finishes (default false). |
| `show.default_fields` | Default fields for `wn show` / bare `wn`. Comma-separated from: `title`, `body`, `status`, `deps`, `notes`, `log`. |
| `cleanup.close_done_items_age` | Default age threshold for `wn cleanup close-done-items` (e.g. `"30d"`). Accepts `d`, `h`, `m`, `s`. |

All `worktree.*` settings are shared by `wn worktree` and `wn do`. CLI flags override settings.

## Worktree workflow

`wn worktree` claims a work item, creates its branch and git worktree, and prints the worktree path to stdout. Human-readable info (item id, title, branch) goes to stderr. This makes it easy to script:

```bash
# Claim a specific item and open it in a new tmux window
WORKTREE=$(wn worktree abc123)
tmux new-window -c "$WORKTREE" "cursor $WORKTREE"

# Claim current task
WORKTREE=$(wn worktree)

# Claim next item from the queue
WORKTREE=$(wn worktree --next)
```

**Flags:** `--next` claims the next undone item (respects `next.tag` from settings; override with `--tag`). `--claim <duration>` overrides `worktree.claim`. `--branch-prefix` and `--worktree-base` override the corresponding settings.

**After the work is done:** run `wn release [id]` to mark the item review-ready (or `wn done` if you want to skip review). The worktree stays until you remove it — `git worktree remove <path>` or `wn merge` (which rebases, merges to main, and marks done in one step).

**Branch notes:** The worktree path is derived from the branch name, which is stored as a `branch` note on the item. On a subsequent run the same branch and worktree are reused. To use a specific branch, set the `branch` note before running: `wn note add branch -m "my-branch-name"`.

## Headless agent runner (`wn do`)

For unattended, automated agent runs. Requires `agent.cmd` to be set in settings (or `--agent-cmd` flag, or `WN_AGENT_CMD` env var).

**`wn do [id]`** runs the full flow for one item then exits: claim → worktree → run agent → commit any uncommitted changes → release. Omit id to use the current task.

**`wn do --next`** claims the next undone item from the queue, runs the full flow, then exits. Fails immediately if the queue is empty.

**`wn do --loop`** loops continuously, picking the next item each time. When the queue is empty it waits and polls. Interrupted by Ctrl-C. Use `-n N` to stop after N items.

**Flow per item:**
1. Atomically claim the next undone item (filtered by `next.tag` if set).
2. Create a git worktree and branch (e.g. `wn-<id>-<slug>`, or reuse the branch from the item's `branch` note).
3. Record the branch name as a `branch` note on the item.
4. Run `agent.cmd` in the worktree with `WN_ROOT` set to the main repo, so the subagent's `wn mcp` uses the same queue.
5. Stage and commit any uncommitted changes with message `wn <id>: <first line of description>`.
6. Release the claim and mark the item review-ready.
7. Optionally remove the worktree (`agent.leave_worktree: false`) or leave it for a PR.
8. Wait `agent.delay`, then loop.

**Configuration example** (in `~/.config/wn/settings.json`):
```json
{
  "next": { "tag": "agent" },
  "worktree": { "claim": "2h", "branch_prefix": "keith/" },
  "agent": {
    "cmd": "cursor agent --print --trust --approve-mcps \"{{.Prompt}}\"",
    "delay": "60s",
    "poll": "60s"
  }
}
```

Then: `wn do --loop` (or `wn do --next` to process one item and exit, or `wn do --loop -n 5` to process at most 5).

**Limiting runs:** `wn do --loop -n N` stops after N items. `wn do [id]` or `wn do --next` both process exactly one item and exit.

**Subagent contract:** The agent runs in the worktree with `WN_ROOT` pointing at the main repo. It should implement the work, optionally add follow-up items via `wn` MCP, and call `wn_release` (or `wn release`) when done. The runner will commit any remaining uncommitted changes automatically.

All git commands and agent invocations are logged with timestamps to stderr.

### Tags and suspend

- **Tags:** Add tags when creating items (`wn add -t priority:high -m "..."`) or after (`wn tag add priority:high`). Filter with `wn list --tag priority:high`, `wn next --tag agent`, or MCP `wn_list` / `wn_next`. Set `next.tag` in settings to permanently scope which items `wn next` and `wn do` consider.
- **Suspend:** For items you might revisit but don't want in the active queue, use `wn status suspend [id] -m "reason"`. Suspended items are excluded from `wn next` and agent claim but stay visible in `wn list`.
- **Dependencies:** When adding follow-up items via MCP, use `wn_add` with `depends_on` (e.g. current task id) to preserve queue order without a separate `wn_depend` call.

## Sort order

List order and fzf pick order are controlled by:

- **`wn list --sort '...'`** — Comma-separated sort keys; each key may be suffixed with `:asc` or `:desc`. Keys: `created`, `updated`, `priority` (backlog order), `alpha` (description), `tags`. Example: `wn list --sort 'updated:desc,priority,tags'`.
- **`sort` in settings** — Applies to `wn list` when `--sort` is not given, and to fzf/numbered lists for `wn pick`, `wn tag add -i`, `wn depend -i`, and `wn rm`.

When no sort preference is set, `wn list` uses dependency order (topological) for undone items.

## Optional: fzf for interactive commands

If `fzf` is in your `PATH`:
- **`wn pick`** uses it for fuzzy selection of the current task.
- **`wn rm`** with no id uses fzf with multi-select (Tab to select, Enter to confirm).
- **`wn tag add -i <tag>`** uses fzf with multi-select; selected items have the tag toggled.
- **`wn depend add -i`** uses fzf to pick the depended-on item.
- **`wn depend rm -i`** uses fzf to pick which dependency to remove.

Without fzf, a numbered list is shown instead. Picker behavior can be controlled at three levels (highest priority wins):

1. **`WN_PICKER` env var** — `WN_PICKER=numbered` forces numbered list; `WN_PICKER=fzf` forces fzf. Useful for CI scripts.
2. **`--picker` flag** — `wn pick --picker numbered` or `wn pick --picker fzf`. Applies to any command for that invocation.
3. **`picker` in settings** — Set `"picker": "fzf"` or `"picker": "numbered"` in `~/.config/wn/settings.json`. Omit (or set to `""`) for auto-detect.

## Testing

```bash
make          # runs fmt, lint, cover, build (cover uses WN_PICKER=numbered)
go test ./...
go test ./internal/wn/... -cover   # aim for 80%+ coverage
```

When running tests, set `WN_PICKER=numbered` (or use `make test` / `make cover`) so interactive pick uses the numbered list and tests do not block on fzf.

Development follows red/green TDD: write tests first, see expected failures, then implement.

## License

MIT
