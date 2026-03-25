# wn: "What's Next"

CLI for tracking work items locally. Use it from your project directory or from agents (e.g. Cursor, Claude Code) to keep a queue of tasks.

## About
Lately I've been working more and more with LLM coding agents and refining my workflow for using them effectively. (Hello from early 2026.)  I've been composing prompts and keeping todo lists in a plain Markdown file, managing dependencies and grouping items and tracking what's done manually.  That works OK for a small number of items at a time, but an actual tool for tracking work items seemed like a natural next step.  Using a heavyweight issue tracker like GitHub issues or JIRA would be overkill for that when it's just me.

There are tons of other "lightweight todo list" tools like 'wn' out there, some of which are even geared toward agentic workflows.  But in a fit of NIH and because I thought it'd be fun, I used Cursor to create `wn` to fit the way I think and work.  I'm still learning Golang and LLM coding agents like Cursor, so this kind of project is perfect for that.  (The fact that there are tons of other todo list apps out there makes LLM coding agents very good at this kind of program!)

In its current state I already find `wn` useful.  I have ideas to improve it for both human and LLM agents to work with it, and I use `wn` to track those ideas.  For example `wn` has MCP support and a temporary "claim" feature to allow agents to treat the work item list as a queue.  So consider `wn` experimental, but feedback and PRs are welcome.


## Install

```bash
brew install kjhaber/tap/wn
```

Or build from source (requires **Go 1.26** or later):

```bash
go install github.com/kjhaber/wn/cmd/wn@latest
# or clone and build
go build -o wn ./cmd/wn
```

Or use the Makefile: `make build` builds the binary to `build/wn`, `make test` runs tests, and `make` (or `make all`) runs format check, lint, coverage, and build.

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
| `wn rm [id ...]` | Remove work item(s). Omit id to remove the current task; pass one or more ids to remove those directly. |
| `wn edit <id>` | Edit description in `$EDITOR` |
| `wn tag add <tag-name> [--wid <id>]` | Add a tag. Omit `--wid` to use the current task. Use `-i` to pick items with fzf and toggle the tag on each. |
| `wn tag rm <tag-name> [--wid <id>]` | Remove a tag. Omit `--wid` to use the current task. |
| `wn tag list [--wid <id>]` | List tags on the work item (one per line). Omit `--wid` to use the current task. |
| `wn summary` | Show a summary dashboard: aggregate counts by status and by tag. Useful for a quick project health check without scrolling through all items. |
| `wn list [@view]` | List items (default: undone; dependency order). Status column: undone, blocked, claimed, review, prompt, done, closed, suspend. Use `--review-ready`/`--rr` to list only review items; `--done`, `--all`, `--tag x`, `--json` for machine-readable output; `--sort 'updated:desc,priority,tags'` to sort; `--limit N` and optional `--offset N` for a bounded window; `--group tags` or `--group status` to display items in labeled sections. Pass `@name` to apply a named view from `settings.json` (e.g. `wn list @agent`). |
| `wn show [id]` | Show a work item (human-readable by default; `--json` for machine-readable; `--plain` for description text only, suitable for pasting into an agent). Omit id for current task. Control fields with `--fields title,body,status,deps,notes,log` or `--all`. |
| `wn depend add --on <id> [--wid <id>]` | Add dependency (rejects cycles). Omit `--wid` for current task. Use `-i` to pick the depended-on item. |
| `wn depend rm --on <id> [--wid <id>]` | Remove dependency. Omit `--wid` for current task. Use `-i` to pick which dependency to remove. |
| `wn depend list [--wid <id>]` | List dependency ids of the work item, one per line. Omit `--wid` for current task. |
| `wn done <id> -m "..."` | Mark complete (use `--force` if dependencies not done) |
| `wn undone <id>` | Mark not complete |
| `wn status <state> [id]` | Set work item status. State: undone, claimed, review, prompt, done, closed, suspend. Omit id for current task. Use `--for 30m` when setting to claimed; `-m "..."` for done/closed/suspend. Use `--duplicate-of <id>` when setting to closed. |
| `wn claim [id] [--for 30m]` | Mark in progress (item leaves undone list until expiry or release). Omit `--for` to use default 1h; optional `--by` for logging. |
| `wn release [id]` | Clear in progress and mark item **review-ready** (excluded from `wn next` and agent claim until you mark done). |
| `wn review-ready [id]` / `wn rr [id]` | Set item to review-ready state directly. |
| `wn next` | Set the first available undone item (dependency order) as current; excludes review-ready and in-progress. Use `--tag <tag>` to filter (or set `next.tag` in settings). Use `--claim 30m` to also claim it. |
| `wn pick [id\|.\|-]` | Interactively choose current task (fzf if available). Pass an id to set current directly. Pass `.` to select the item for the current directory's git branch (useful when switching between worktrees). Pass `-` to switch to the previously selected item (like `git checkout -`). Filter: `--undone` (default), `--done`, `--all`, `--rr`/`--review-ready`. Use `--picker fzf\|numbered` to override picker. |
| `wn worktree [id]` | Claim a work item, create its branch and git worktree, and print the worktree path to stdout. Omit id to use current task; use `--next` to claim next from the queue. See [Worktree workflow](#worktree-workflow). |
| `wn do [runner] [id]` | Claim a work item, set up its worktree, run the configured runner command, commit any changes, and release. Omit id to use current task; specify a runner name (e.g. `wn do claude`) or omit to use `agent.default`. Use `--next` to claim next from the queue; `--loop` to process items continuously. See [Agent runners](#agent-runners-wn-do-wn-launch). |
| `wn launch [runner] [id]` | Dispatch a work item to an async runner (e.g. tmux window, IDE) and return immediately. Worktree is created and item stays claimed; the agent or user releases it later via `wn release`. Uses `agent.default_launch`. Use `--next` to dispatch next queue item; `--loop` to dispatch items continuously; `--loop -n N` to stop after N dispatches. See [Agent runners](#agent-runners-wn-do-wn-launch). |
| `wn cleanup set-merged-review-items-done` | Check all review-ready items; mark done if their `branch` note has been merged to the current branch. Use `--dry-run` to preview; `-b main` to check against a specific ref. |
| `wn cleanup close-done-items [--age 30d]` | Close items that have been in **done** state longer than the configured age. Use `--dry-run` to preview. |
| `wn cleanup worktrees` | Remove git worktrees whose associated wn item is done and whose branch has been merged, and delete their branches. Also finds and deletes orphaned branches (no worktree, but done+merged item). Prompts for confirmation before removing. Use `--worktrees-only` to skip branch deletion; `--force` to skip the prompt; `--dry-run` to preview without removing; `--clean-ignored` to also remove gitignored files (build artifacts, temp config) before removal; `-b <ref>` to check against a specific ref instead of HEAD. |
| `wn log <id>` | Show history for an item. |
| `wn prompt [parent-id] -m "question"` | Create a prompt item (a question for the user) and add it as a dependency of the parent. The parent becomes **blocked** until the user responds with `wn respond`. Omit parent-id for current task; omit `-m` to use `$EDITOR`. See [Agent/human prompt workflow](#agenthuman-prompt-workflow). |
| `wn respond [prompt-id] -m "answer"` | Respond to a prompt item: marks it done and stores the answer as a `response` note. Unblocks the parent item. Omit prompt-id for current task; omit `-m` to use `$EDITOR`. |
| `wn note add <name> [id] -m "..."` | Add or update a note by name (e.g. pr-url, issue-number); omit id for current task, omit `-m` to use `$EDITOR`. Names: alphanumeric, /, _, -, up to 32 chars. Special `wn:` names (e.g. `wn:branch`) are reserved for internal use; `wn:branch` auto-detects the current git branch when `-m` is omitted. |
| `wn note list [id]` | List notes on an item (name, created, body), ordered by create time. |
| `wn note show [id] <name>` | Print the raw body of a named note; omit id for current task. Useful for scripting, e.g. `git checkout $(wn note show branch)`. |
| `wn note edit [id] <name> [-m "..."]` | Edit a note by name; omit `-m` to use `$EDITOR` with current body. |
| `wn note rm [id] <name>` | Remove a note by name. |
| `wn note search <name> [value]` | Search all work items for those having a note named `<name>`. If `<value>` is given, only items where the note body exactly matches are returned. Prints `<id>  <first line>` per match. Use `--first` to return only the oldest match (by created time) or `--latest` for the most recently updated. Use `--id-only` to print just the item ID(s), one per line — useful for scripts (e.g. `wn note search wn:branch <branch> --first --id-only`). |
| `wn settings show` | Print the fully merged effective settings as JSON. |
| `wn settings edit [--user\|--user-local\|--project\|--project-local]` | Interactively pick a settings file to open in `$EDITOR` (fzf or numbered). Use a flag to skip the picker and open a specific file directly. Missing files are created as `{}` before opening. |
| `wn verify` | Run the shell command configured in `settings.verify` (e.g. `make all`, `npm test`). Useful for agents and humans to confirm the build passes. |
| `wn export [-o file] [--undone\|--done\|--all\|--review-ready] [--tag expr] [--sort spec] [--limit N] [--offset N]` | Export items to JSON (stdout if no `-o`). Supports the same selection flags as `wn list`: filter by status, tag (compound `a,b`/`a\|b`), sort, and paginate. |
| `wn import <file>` | Import items from JSON export. When store has items, use `--append` (add/merge) or `--replace` (replace all). |
| `wn mcp` | Run MCP server on stdio (for Cursor and other MCP clients). |
| `wn help` / `wn completion` | Help and shell completion. |

Work item IDs are 6-character hex prefixes (e.g. `af1234`). The tool finds the wn root by walking up from the current directory until it finds a `.wn` directory.

**Work item status:** Each item has one of the following statuses. Use `wn status <state> [id]` to set any state (omit id for current task). `wn done` and `wn undone` are shortcuts for the common cases.

| Status | Description |
|--------|-------------|
| **undone** | Not complete; available for `wn next` and agent claim. Default for new items. |
| **blocked** | Computed—displayed when an undone or claimed item has at least one dependency that is not yet done. Not a stored state; clears automatically when dependencies complete. |
| **claimed** | In progress—someone has claimed it until a duration expires or they run `wn release`. Excluded from `wn next` and claim until expiry. |
| **review** | Work is done but not yet accepted (e.g. PR open). Excluded from `wn next` and claim; use `wn list --rr` to see review items. Set by `wn release` or `wn review-ready` / `wn rr`. Mark **done** when merged or accepted. |
| **prompt** | Awaiting a human response. Set by `wn prompt` (or `wn status prompt`) to create a blocking question for the user. Excluded from `wn next` and agent claim. Resolved by `wn respond`, which marks the item done and stores the answer. |
| **done** | Completed and accepted. Use `wn done` or `wn status done`. |
| **closed** | Completed and closed (e.g. archived). Terminal state. |
| **suspend** | Deferred—not ready to implement or not sure you want to. Like done (excluded from next/claim) but not retired to closed; use for ideas you might revisit. |

**Review-ready:** When you or an agent runs `wn release`, the item is marked *review-ready*: it stays in the list but is excluded from `wn next` and agent claim so it won't be picked again. Use `wn list --rr` to see review-ready items. Mark it done when work is merged (`wn done` or `wn cleanup set-merged-review-items-done`).

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

Tools: `wn_add`, `wn_list`, `wn_done`, `wn_undone`, `wn_desc`, `wn_show`, `wn_item`, `wn_claim`, `wn_release`, `wn_next`, `wn_depend`, `wn_rmdepend`, `wn_note_search`, `wn_note_add`, `wn_note_edit`, `wn_note_rm`, `wn_duplicate`, `wn_prompt`, `wn_respond`. Use `wn_item` with a required id to get full item JSON and notes. For `wn_claim`, omit `for` to use default 1h so agents can renew without losing context. For `wn_next`, pass optional `tag` to return the next undone item with that tag, and optional `claim_for` to atomically claim it. For `wn_list`, pass `limit` and optional `offset` or `cursor` for a bounded window. For `wn_add`, pass optional `depends_on` (array of item IDs) to preserve queue order. Use `wn_note_search` to find items by note name/value (returns JSON array of `{id, description}`; supports `first`/`latest` to limit to one result). Use `wn_duplicate` to mark an item as a duplicate of another (sets status to closed, adds `duplicate-of` note). Use `wn_prompt` to create a blocking question for the user (adds a prompt item as a dep of the parent); use `wn_respond` to answer it and unblock the parent.

## Settings

Settings are loaded in order (later files override earlier ones, field by field):

1. **User settings** — `~/.config/wn/settings.json` (macOS/Linux default, override with `WN_SETTINGS_USER`)
2. **User-local settings** — optional machine-local overrides; set `WN_SETTINGS_USER_LOCAL` to enable
3. **Project settings** — `.wn/settings.json` in your project root
4. **Project-local settings** — `.wn/settings.local.json` (gitignored; personal project overrides)

To use separate shared and machine-local user settings files (e.g. dotfiles + local overrides):

```sh
export WN_SETTINGS_USER="$HOME/.config/wn/settings.json"
export WN_SETTINGS_USER_LOCAL="$HOME/.config-local/wn/settings.json"
```

Missing files are silently skipped. Use `wn settings show` to see the merged effective settings, and `wn settings edit` to open any of these files in `$EDITOR`.

```json
{
  "sort": "tags,priority,updated,alpha",
  "picker": "fzf",
  "verify": "make all",

  "next": {
    "tag": "agent"
  },

  "worktree": {
    "base": "../worktrees",
    "branch_prefix": "keith/",
    "default_branch": "main",
    "claim": "2h"
  },

  "runners": {
    "cursor": {
      "cmd": "cursor agent --print --trust --approve-mcps \"{{.Prompt}}\""
    },
    "claude": {
      "cmd": "claude {{.ResumeFlag}} --print --dangerously-skip-permissions \"{{.Prompt}}\""
    },
    "tmux-claude": {
      "cmd": "tmux new-window -c {{.Worktree}} 'claude -p \"{{.Prompt}}\"'",
      "leave_worktree": true
    }
  },

  "agent": {
    "default": "cursor",
    "default_launch": "tmux-claude",
    "delay": "10s",
    "poll": "60s"
  },

  "show": {
    "default_fields": "title,body,deps,notes"
  },

  "cleanup": {
    "close_done_items_age": "30d"
  },

  "views": {
    "agent": "--tag agent --sort priority",
    "review": "--review-ready --group status",
    "all": "--all --group status"
  }
}
```

| Key | Description |
|-----|-------------|
| `sort` | Default sort order for `wn list`, `wn pick`, and interactive lists. See [Sort order](#sort-order). |
| `picker` | Interactive picker: `"fzf"` (always use fzf), `"numbered"` (always use numbered list), or omit for auto-detect (fzf if in PATH). Overridden by `--picker` flag or `WN_PICKER` env var. |
| `verify` | Shell command to run for `wn verify` (e.g. `"make all"`, `"npm test"`). Set at project level for project-specific build/test commands. |
| `next.tag` | Only consider items with this tag when selecting the next item (`wn next`, `wn worktree --next`, `wn do --next/--loop`). Overridden by `--tag` flag. |
| `worktree.base` | Base directory for git worktrees. Default: parent of the main worktree. |
| `worktree.branch_prefix` | Prefix for generated branch names (e.g. `"keith/"` → `keith/wn-abc123-add-feature`). |
| `worktree.branch_template` | Go template for the branch base name. Vars: `{{.ID}}`, `{{.Slug}}`. Default: `wn-{{.ID}}-{{.Slug}}`. Set to `{{.Slug}}` to omit `wn` and the item ID from branch names entirely. The `branch_prefix` is applied before the result. |
| `worktree.default_branch` | Override default branch detection (e.g. `"main"`). |
| `worktree.claim` | How long to claim an item when setting up a worktree (e.g. `"2h"`). |
| `runners.<name>.cmd` | Command template for a named runner. `{{.Prompt}}`, `{{.Worktree}}`, `{{.Branch}}`, `{{.ItemID}}`, `{{.ResumeFlag}}`, and `{{.SessionID}}` are available. `{{.ResumeFlag}}` expands to `--resume <session-id>` if a `claude-session` note exists on the item, or `""` if not—enabling automatic session resume. |
| `runners.<name>.prompt` | Per-runner prompt template (default `{{.Description}}`). Fields: `{{.ItemID}}`, `{{.Description}}`, `{{.FirstLine}}`, `{{.Worktree}}`, `{{.Branch}}`. |
| `runners.<name>.leave_worktree` | If true, keep the worktree after the runner finishes. Defaults to false; recommended true for async runners. |
| `agent.default` | Default runner name for `wn do` (sync). |
| `agent.default_launch` | Default runner name for `wn launch` (async). |
| `agent.delay` | Delay between items in loop mode (e.g. `"10s"`). |
| `agent.poll` | Poll interval when the queue is empty (e.g. `"60s"`). |
| `agent.commit_template` | Go template for auto-commit messages generated by `wn do`. Vars: `{{.ID}}`, `{{.FirstLine}}`. Default: `wn {{.ID}}: {{.FirstLine}}`. Set to `{{.FirstLine}}` to omit `wn` references from commit messages. |
| `show.default_fields` | Default fields for `wn show` / bare `wn`. Comma-separated from: `title`, `body`, `status`, `deps`, `notes`, `log`. |
| `cleanup.close_done_items_age` | Default age threshold for `wn cleanup close-done-items` (e.g. `"30d"`). Accepts `d`, `h`, `m`, `s`. |
| `views.<name>` | Named filter+sort+group combo for `wn list @name`. Value is a flags string (e.g. `"--tag agent --sort priority --group status"`). Supports `--tag`, `--sort`, `--group`, `--done`, `--undone`, `--all`, `--json`, `--limit`, `--offset`. |

All `worktree.*` settings are shared by `wn worktree`, `wn do`, and `wn launch`. Runners and views are merged by key between user and project settings (project overrides same-named entries, unique keys from each are preserved). CLI flags override settings.

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

**Flags:** `--next` claims the next undone item (respects `next.tag` from settings; override with `--tag`). `--claim <duration>` overrides `worktree.claim`. `--branch-prefix`, `--worktree-base` override the corresponding settings. `--branch <slug>` overrides the auto-generated slug (the full branch name is built via `worktree.branch_template`).

**After the work is done:** run `wn release [id]` to mark the item review-ready (or `wn done` if you want to skip review). The worktree stays until you remove it — `git worktree remove <path>`.

**Branch notes:** The worktree path is derived from the branch name, which is stored as a `wn:branch` note on the item. On a subsequent run the same branch and worktree are reused. To use a specific branch, set the `wn:branch` note before running: `wn note add wn:branch -m "my-branch-name"` (or omit `-m` to auto-detect from the current git branch).

**Switching between worktrees:** When you `cd` into a worktree (e.g. after `wn launch` opens a tmux window), run `wn pick .` to re-select the associated work item as current. This looks up the current git branch and finds the item whose `wn:branch` note matches.

## Agent runners (`wn do`, `wn launch`)

Runners are named command profiles defined in `settings.runners`. Each runner has a `cmd` template (and optionally a `prompt` template and `leave_worktree` flag). You can define as many as you like and switch between them by name.

### `wn do` — sync runner

For unattended, automated agent runs. Requires `agent.default` to be set in settings (or pass a runner name explicitly).

**`wn do [runner] [id]`** runs the full flow for one item then exits: claim → worktree → run agent → commit any uncommitted changes → release. Omit id to use the current task. Omit runner to use `agent.default`; pass a runner name (e.g. `wn do claude`) to override.

**`wn do --next`** claims the next undone item from the queue, runs the full flow, then exits. Fails immediately if the queue is empty.

**`wn do --loop`** loops continuously, picking the next item each time. When the queue is empty it waits and polls. Interrupted by Ctrl-C. Use `-n N` to stop after N items.

**Flow per item:**
1. Atomically claim the next undone item (filtered by `next.tag` if set).
2. Create a git worktree and branch (name generated from `worktree.branch_template`, default `wn-<id>-<slug>`; or reuse the branch from the item's `wn:branch` note).
3. Record the branch name as a `wn:branch` note on the item.
4. Run the runner's `cmd` in the worktree with `WN_ROOT` set to the main repo, so the subagent's `wn mcp` uses the same queue.
5. Stage and commit any uncommitted changes with a message from `agent.commit_template` (default: `wn <id>: <first line of description>`).
6. Release the claim: if the item is now blocked (e.g. the agent created prompt dependencies via `wn prompt`), only the claim is cleared—the item stays undone until deps resolve. Otherwise the item is marked review-ready.
7. Optionally remove the worktree (per runner's `leave_worktree`) or leave it for a PR.
8. Wait `agent.delay`, then loop.

**Configuration example** (in `~/.config/wn/settings.json`):
```json
{
  "next": { "tag": "agent" },
  "worktree": { "claim": "2h", "branch_prefix": "keith/" },
  "runners": {
    "cursor": { "cmd": "cursor agent --print --trust --approve-mcps \"{{.Prompt}}\"" },
    "claude": { "cmd": "claude {{.ResumeFlag}} --print --dangerously-skip-permissions \"{{.Prompt}}\"" }
  },
  "agent": { "default": "cursor", "delay": "60s", "poll": "60s" }
}
```

Then: `wn do --loop` (uses `cursor`), `wn do claude --next` (uses `claude` for one item), `wn do --loop -n 5` (at most 5 items).

**Subagent contract:** The agent runs in the worktree with `WN_ROOT` pointing at the main repo. It should implement the work, optionally add follow-up items via `wn` MCP, and call `wn_release` (or `wn release`) when done. The runner will commit any remaining uncommitted changes automatically.

### `wn launch` — async dispatch

For interactive workflows: open a tmux window, launch an IDE, or any command that should run in the background while you continue working.

**`wn launch [runner] [id]`** sets up the worktree and fires the runner's `cmd` without waiting. The item stays claimed; the agent (or you) releases it later via `wn release` or `wn_release`. Omit runner to use `agent.default_launch`.

**`wn launch --next`** dispatches the next undone item from the queue. Fails immediately if the queue is empty.

**`wn launch --loop`** continuously dispatches items from the queue, polling when empty.

**`wn launch --loop -n N`** dispatches N items then exits.

The worktree is always preserved for async runners (regardless of `leave_worktree`).

**Example:** with `tmux-claude` configured as `default_launch`:
```bash
wn launch               # dispatches current item to a new tmux window
wn launch cursor        # uses cursor runner instead
wn launch --next        # dispatches next queue item
wn launch --loop        # dispatches items continuously (one tmux window per item)
wn launch --loop -n 3   # dispatches next 3 items then exits
```

In `wn tui`, press `>` to launch the selected item using `agent.default_launch`.

All git commands and agent invocations are logged with timestamps to stderr.

### Agent/human prompt workflow

Agents sometimes need input before they can continue—a clarifying question, a design decision, or a credential they cannot self-provide. The prompt workflow lets a background agent pause and surface that question to you without blocking other queue items.

**How it works:**

1. **Agent asks a question** — inside `wn do` / `wn launch`, the agent calls `wn prompt` (or via MCP if you wire it up) to create a new *prompt* item and add it as a dependency of its own work item:
   ```
   wn prompt abc123 -m "Should the retry logic use exponential backoff or fixed delay?"
   ```
   This creates a prompt item (e.g. `def456`), sets it to *prompt* status, and adds it as a dep of `abc123`. When the agent finishes its current turn, `wn do` detects that `abc123` is now blocked and clears the claim without marking review-ready—so it stays in the undone queue, blocked.

2. **You see it in `wn list`** — `abc123` shows as *blocked*; `def456` shows as *prompt*. Use `wn show def456` to read the question.

3. **You respond** — run `wn respond def456 -m "Use exponential backoff, cap at 60s"`. This marks `def456` done (unblocking `abc123`) and stores the answer as a `response` note.

4. **Agent resumes** — `abc123` is no longer blocked. On the next `wn do` run it is picked up normally. Configure your runner with `{{.ResumeFlag}}` to automatically resume the prior Claude Code session:
   ```json
   "cmd": "claude {{.ResumeFlag}} --print --dangerously-skip-permissions \"{{.Prompt}}\""
   ```
   If the agent stored its session ID in a `claude-session` note, `{{.ResumeFlag}}` expands to `--resume <id>`; otherwise it is empty and the agent starts fresh.

**Prompt state** is excluded from `wn next` and agent claim (agents don't accidentally pick up their own questions). A prompt item can be transitioned to any other status freely—use `wn status undone def456` if you want to re-open it as a regular task instead.

### Tags and suspend

- **Tags:** Add tags when creating items (`wn add -t priority:high -m "..."`) or after (`wn tag add priority:high`). Filter with `wn list --tag priority:high`, `wn next --tag agent`, or MCP `wn_list` / `wn_next`. Set `next.tag` in settings to permanently scope which items `wn next` and `wn do` consider.
- **Suspend:** For items you might revisit but don't want in the active queue, use `wn status suspend [id] -m "reason"`. Suspended items are excluded from `wn next` and agent claim but stay visible in `wn list`.
- **Dependencies:** When adding follow-up items via MCP, use `wn_add` with `depends_on` (e.g. current task id) to preserve queue order without a separate `wn_depend` call.

## Sort order and grouping

List order and fzf pick order are controlled by:

- **`wn list --sort '...'`** — Comma-separated sort keys; each key may be suffixed with `:asc` or `:desc`. Keys: `created`, `updated`, `priority` (backlog order), `alpha` (description), `tags`. Example: `wn list --sort 'updated:desc,priority,tags'`.
- **`sort` in settings** — Applies to `wn list` when `--sort` is not given, and to fzf/numbered lists for `wn pick`, `wn tag add -i`, and `wn depend -i`.

When no sort preference is set, `wn list` uses dependency order (topological) for undone items.

**Grouping** (`--group <key>`) splits the list into labeled sections. Items are sorted by the group key first, then rendered with a `--- section ---` header between groups:

- `--group tags` — groups by tag set. Items with the same tags appear together; items with no tags are collected under `(no tags)`. Example header: `--- #agent ---`, `--- #agent #backend ---`, `--- (no tags) ---`.
- `--group status` — groups by computed status (e.g. `--- undone ---`, `--- blocked ---`, `--- done ---`).

`--group` is incompatible with `--json`. Example: `wn list --all --group status`.

## Optional: fzf for interactive commands

If `fzf` is in your `PATH`:
- **`wn pick`** uses it for fuzzy selection of the current task.
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
