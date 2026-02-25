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
| `wn list` | List items (default: undone, i.e. both available-for-claim and review-ready—excludes in-progress; dependency order). Status column: undone, claimed, done, review-ready. Use `--review-ready`/`--rr` to list only review-ready items; `--done`, `--all`, `--tag x`, `--json` for machine-readable output; `--sort 'updated:desc,priority,tags'` to sort; `--limit N` and optional `--offset N` for a bounded window. |
| `wn depend [id] --on <id2>` | Mark dependency (rejects cycles). Use `-i` to pick the depended-on item from undone work items (fzf or numbered list) |
| `wn rmdepend [id] --on <id2>` | Remove dependency. Use `-i` to pick which dependency to remove (fzf or numbered list) |
| `wn order [id] --set <n>` / `--unset` | Set or clear optional backlog order (lower = earlier when deps don't define order) |
| `wn done <id> -m "..."` | Mark complete (use `--force` if dependencies not done) |
| `wn undone <id>` | Mark not complete |
| `wn duplicate [id] --of <id>` | Mark a work item as a duplicate of another. Adds the standard note `duplicate-of` (body = original id) and marks the item done so it leaves the queue; the item is kept for reference. Omit id for current task. |
| `wn claim [id] [--for 30m]` | Mark in progress (item leaves undone list until expiry or release). Omit `--for` to use default 1h so you can renew with just `wn claim`; optional `--by` for logging |
| `wn release [id]` | Clear in progress and mark item **review-ready** (excluded from `wn next` and agent claim until you mark done) |
| `wn review-ready [id]` / `wn rr [id]` | Set item to review-ready state directly (excluded from `wn next` until marked done) |
| `wn mark-merged` | Check all review-ready items; mark done if their `branch` note's branch has been merged to current branch. Use `--dry-run` to preview; `-b main` to check against a specific ref. |
| `wn log <id>` | Show history for an item |
| `wn note add <name> [id] -m "..."` | Add or update a note by name (e.g. pr-url, issue-number); omit id for current task, omit `-m` to use `$EDITOR`. Names: alphanumeric, /, _, -, up to 32 chars |
| `wn note list [id]` | List notes on an item (name, created, body), ordered by create time |
| `wn note edit [id] <name> [-m "..."]` | Edit a note by name; omit `-m` to use `$EDITOR` with current body |
| `wn note rm [id] <name>` | Remove a note by name |
| `wn desc [id]` | Print prompt-ready description (use `--json` for machine-readable) |
| `wn prompt [id]` | Output work item wrapped in a prompt template (for pasting into an agent). Omit id for current task. Use `--template` for title-only one-liners, `--template-body` for items with description; placeholder `{}` is replaced by content. |
| `wn show [id]` | Output one work item as JSON (full item; omit id for current) |
| `wn next` | Set “next” task (first **available** undone in dependency order) as current; excludes review-ready and in-progress. Use `--tag <tag>` to only consider items with that tag (enables "next agentic item" without listing the full queue). Use `--claim 30m` to also claim it (optional `--claim-by` for logging) |
| `wn pick [id]` | Interactively choose current task (fzf if available). Pass an id to set current directly. Filter by state: `--undone` (default), `--done`, `--all`, or `--rr`/`--review-ready`. |
| `wn settings` | Open `~/.config/wn/settings.json` in `$EDITOR`. Set `"sort": "updated:desc,priority,tags"` for default list/fzf order |
| `wn export [-o file]` | Export all items to JSON (stdout if no `-o`) |
| `wn import <file>` | Import items from JSON export. When store has items, use `--append` (add/merge) or `--replace` (replace all). When store is empty, either flag is optional. |
| `wn help` / `wn completion` | Help and shell completion |
| `wn mcp` | Run MCP server on stdio (for Cursor and other MCP clients) |
| `wn agent-orch` | Run the agent orchestrator loop: claim next item, create worktree, run subagent (e.g. Cursor/Claude Code), release. See [Agent workflow runner](#agent-workflow-runner-wn-agent-orch) below. |
| `wn do [id]` | Shorthand: run agent orchestrator on a work item and exit. With id: `agent-orch --work-id <id>`. Without id: uses current task (`agent-orch --current`). |

Work item IDs are 6-character hex prefixes (e.g. `af1234`). The tool finds the wn root by walking up from the current directory until it finds a `.wn` directory.

**Review-ready:** When you or an agent runs `wn release`, the item is marked *review-ready*: it stays undone but is excluded from `wn next` and from agent claim (so it won't be picked again); `wn list` and MCP `wn_list` include both undone and review-ready. You can also set an item to review-ready directly with `wn review-ready` (alias `wn rr`). Use `wn list --review-ready` (or `wn list --rr`) to list only review-ready items for human review. Mark it done when the work is merged or accepted (e.g. merge to main). Use `wn mark-merged` to automatically mark done all review-ready items whose branch has been merged to the current branch.

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


Tools: `wn_add`, `wn_list`, `wn_done`, `wn_undone`, `wn_desc`, `wn_show`, `wn_item`, `wn_claim`, `wn_release`, `wn_next`, `wn_order`, `wn_depend`, `wn_rmdepend`, `wn_note_add`, `wn_note_edit`, `wn_note_rm`, `wn_duplicate`. Use `wn_item` with a required id to get full item JSON and notes (e.g. when a subagent only has an item id). For `wn_claim`, omit `for` to use default 1h so agents can renew (extend) without losing context. For `wn_next`, pass optional `tag` (e.g. `agent`) to return/set current to the next undone item that has that tag (dependency order)—enables getting the next agentic item without listing the full queue. Pass optional `claim_for` (e.g. `30m`) to atomically claim the returned item so concurrent workers don't double-assign. For `wn_list`, pass `limit` (max items) and optional `offset` or `cursor` (item id to start after) for a bounded window and smaller context. For `wn_add`, pass optional `depends_on` (array of item IDs) when adding follow-up items so the new item depends on the current task or others and queue order is preserved. Notes: `wn_note_add` adds or updates a note by name (e.g. `pr-url`, `issue-number`); `wn_note_edit` changes an existing note's body; `wn_note_rm` removes a note. All note tools accept optional `id` (omit for current task) and use the same name rules as the CLI (alphanumeric, `/`, `_`, `-`, 1–32 chars). Use `wn_duplicate` to mark an item as a duplicate of another: it adds the standard note `duplicate-of` (body = original item id) and marks the item done so it leaves the queue while keeping the item for reference.

## Agent workflow runner (wn agent-orch)

`wn agent-orch` runs a loop that picks the next available work item, creates a git worktree and branch, runs a configurable CLI subagent (e.g. Cursor or Claude Code) with the item prompt, then releases the claim. You run it from the **main worktree** where `.wn` is initialized. All commands (git, agent CLI) are logged with **timestamps** to stderr for visibility and auditability.

**Worktrees:** By default the worktree base is the **parent** of the main worktree directory (peer directories, not under `.wn`). Each worktree directory is named **`<main-dirname>-<branch>`** (e.g. `main-wn-abc123-add-feature`) so multiple projects under the same parent don't collide. Override with `--worktrees` or `worktrees` in settings.

**Selection:** The runner picks the next work item from the undone queue. **Dependencies** are honored (prerequisites first); within each tier, the **Order** field is the tiebreaker (lower = earlier). You can restrict candidates with an optional **tag**: only items that have that tag are considered. Use `--tag <tag>` or set `agent_orch.tag` in settings (e.g. `"tag": "agent"` to process only items tagged `agent`).

**Flow per item:** (1) Atomically claim the next undone item (subject to tag filter if set). (2) Create a git worktree and branch (e.g. `wn-<id>-<slug>` from the description, or reuse a branch from the item’s `branch` note). (3) Add a `branch` note to the item with the branch name. (4) Run your configured agent command in the worktree with `WN_ROOT` set to the main repo so the subagent’s `wn mcp` uses the same queue. (5) When the subagent exits, the item is released (and marked review-ready so it won’t be claimed again until you merge and mark done). (6) The item is released (and marked review-ready). (7) Optionally leave the worktree for you to open a PR, or remove it with `--cleanup-worktree`. (8) After a configurable delay, the loop continues. When the queue is empty, it waits and polls.

After the subagent exits, any uncommitted changes in the worktree are staged and committed by the runner with message `wn <id>: <first line of description>`.

**Configuration:** Set defaults in `~/.config/wn/settings.json` under `"agent_orch"` (e.g. `claim`, `delay`, `poll`, `agent_cmd`, `prompt_tpl`, `worktrees`, `leave_worktree`, `branch`, `branch_prefix`, `tag`). Use `branch_prefix` (e.g. `"keith/"`) so generated branches are named like `keith/wn-abc123-add-feature`. Flags override settings. Use `--tag <tag>` (or `agent_orch.tag`) to only consider items that have that tag. You must set `agent_cmd` (or `WN_AGENT_CMD`) to a **command template** where `{{.Prompt}}` is replaced by the prompt (e.g. `cursor agent --print --trust "{{.Prompt}}"` or `claude -p "{{.Prompt}}"`). The prompt is produced by a **prompt template** (default `{{.Description}}`); fields include `{{.ItemID}}`, `{{.Description}}`, `{{.FirstLine}}`, `{{.Worktree}}`, `{{.Branch}}`.

**Subagent contract:** The subagent runs in the worktree with `WN_ROOT` pointing at the main repo. It should implement the work, add follow-up items via `wn` MCP if needed, commit on the worktree branch, then call **`wn_release`** (or `wn release`) to mark the item review-ready and clear the claim, and add a note (e.g. `commit` or `commit-info`) with commit hash/message using **MCP `wn_note_add`**. You mark the item done after merging to main.

**Note conventions:** The runner sets the `branch` note on each item with the worktree branch name. To reuse an existing branch (e.g. for a follow-up item), set the item’s `branch` note to that branch name before the runner picks it. The subagent adds `commit` or `commit-info` with the commit details.

**Example:** Run one orchestrator per tmux panel; each will claim one item at a time. In `~/.config/wn/settings.json`: 
```json
"agent_orch": {
  "agent_cmd": "cursor agent --print --trust --approve-mcps \"{{.Prompt}}\"",
  "claim": "2h",
  "delay": "60s",
  "poll": "60s"
}
```
**Limiting runs:** Use `-n` / `--max-tasks N` to process at most N tasks then exit (default 0 = run indefinitely). Handy for demos and testing config changes, e.g. `wn agent-orch -n 1`. To run a **single item** then exit: `wn do` (uses current task) or `wn do <id>` (or `wn agent-orch --work-id <id>`) or `wn agent-orch --current`.

Then: `wn agent-orch` (or `wn agent-orch --claim 1h` to override).

### Priority / order (for agents)

When choosing what to work on, agents should use this convention:

- **Order field:** Lower value = higher priority. The optional `order` field on each item is used when dependencies don't define order (e.g. in `wn next`, list order, and fzf). Set via `wn order [id] --set <n>` or MCP `wn_order`; when adding, use `wn add` then set order, or MCP `wn_add` with the `order` parameter (e.g. `order: 1` for high priority). When adding follow-up items via MCP, use `wn_add` with optional `depends_on` (e.g. the current task id) so the agentic queue order is maintained without a separate `wn_depend` call.
- **Tags (optional):** You can use tags like `priority:high` or `priority:low` for filtering. The tool does not interpret these; use `wn list --tag priority:high` or MCP `wn_list` with `tag: "priority:high"` to show only items with that tag. Useful when multiple agents or humans want to mark items without changing numeric order.

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
