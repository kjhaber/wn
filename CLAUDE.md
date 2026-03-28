# Project overview

`wn` ("What's Next") is a CLI work item tracker for local use by humans and coding agents. Written in Go (cobra CLI, bubbletea TUI, MCP server). Work items are stored as JSON files under `.wn/` in the project directory.

Use `wn verify` to run tests rather than `make test` or `go test` to run tests.  `wn verify` is a general-purpose validation command.  In this project it executes `make all`; the Makefile sets required environment variables (e.g. `WN_PICKER=numbered` to avoid blocking on fzf) and enforces lint.

# Build artifacts

Always use `make build` rather than `go build` directly — it outputs the binary to `./build/wn`. Never output binaries or other build artifacts to the project root or elsewhere. `make clean` removes `./build/`; note that `.wn/` is intentionally kept (it tracks work items for this project's own development).

# Code change completion

Before reporting that a code change is complete, you **must** run:

```bash
make all
```

and it must succeed. Do not tell the user the change is done until `make all` has passed.

`make all` runs format check, lint, coverage, and build. If it fails, fix the issues (or inform the user what failed and what they need to do), then run `make all` again until it succeeds.

# README file updates

Whenever commands are added, updated, or removed, the README must be updated accordingly.

# Conventions for adding new CLI commands and MCP tools

## Adding a new CLI command

1. **Implementation file**: Put the business logic in `internal/wn/<feature>.go`. Put the CLI wiring (cobra command struct, flags, `run*` function) in a new file `cmd/wn/cmd_<feature>.go`. The `main.go` file contains only `main()`, `rootCmd`, and the top-level `init()` that calls `rootCmd.AddCommand(...)`.

2. **Declare the command** in `cmd/wn/main.go`:
   ```go
   var myCmd = &cobra.Command{
       Use:          "mycommand <arg>",
       Short:        "One-line description",
       Long:         `Optional longer description.`,
       Args:         cobra.ExactArgs(1),   // or NoArgs, MaximumNArgs, etc.
       RunE:         runMyCommand,
       SilenceUsage: true,
   }
   ```

3. **Declare flags** (if any) in a separate `init()` block or the existing one near the command:
   ```go
   var myFlag string

   func init() {
       myCmd.Flags().StringVarP(&myFlag, "flag", "f", "", "Description of flag")
   }
   ```

4. **Register** the command by adding it to the `rootCmd.AddCommand(...)` call in `cmd/wn/main.go`'s top-level `init()`.

5. **Implement** the run function — it should call into `internal/wn`:
   ```go
   func runMyCommand(cmd *cobra.Command, args []string) error {
       root, err := wn.FindRootForCLI()
       if err != nil {
           return err
       }
       store, err := wn.NewFileStore(root)
       if err != nil {
           return err
       }
       return wn.MyFeature(store, root, args[0])
   }
   ```

6. **Tests**: Add `cmd/wn/main_test.go` integration tests that run the binary via `exec.Command` (see existing tests for the pattern). Add unit tests in `internal/wn/<feature>_test.go`.

## Adding a new MCP tool

1. **Input struct** in `internal/wn/mcp.go` — use `jsonschema` tags for documentation:
   ```go
   type wnMyToolIn struct {
       ID   string `json:"id,omitempty" jsonschema:"Work item id; omit for current task"`
       Root string `json:"root,omitempty" jsonschema:"Optional project root path (directory containing .wn); if omitted, uses process cwd"`
   }
   ```
   Always include `Root string` for workspace flexibility.

2. **Handler function**:
   ```go
   func handleWnMyTool(ctx context.Context, req *mcp.CallToolRequest, in wnMyToolIn) (*mcp.CallToolResult, any, error) {
       store, root, err := getStoreWithRoot(ctx, in.Root)
       if err != nil {
           return nil, nil, err
       }
       meta, err := ReadMeta(root)
       if err != nil {
           return nil, nil, err
       }
       id, err := ResolveItemID(meta.CurrentID, in.ID)  // resolves omitted id to current task
       if err != nil {
           return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "no id provided and no current task"}}, IsError: true}, nil, nil
       }
       // ... do work ...
       return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "result text"}}}, nil, nil
   }
   ```
   Return `IsError: true` for user-facing errors (bad input, item not found). Return a bare `error` only for unexpected internal errors.

3. **Register** in `NewMCPServer()` in `internal/wn/mcp.go`:
   ```go
   mcp.AddTool(server, &mcp.Tool{
       Name:        "wn_my_tool",
       Description: "What this tool does and when to use it.",
   }, handleWnMyTool)
   ```

4. **Tests**: Add cases in `internal/wn/mcp_test.go` using `setupMCPSession(t)` (creates a temp store + in-memory MCP client/server). Call the tool via `clientSession.CallTool(ctx, ...)` and assert on the returned JSON.

## Core abstractions map

| Symbol | File | Purpose |
|--------|------|---------|
| `Store` interface | `internal/wn/store.go` | Persistence: `List`, `Get`, `Put`, `UpdateItem`, `Delete`, `Root` |
| `NewFileStore(root)` | `internal/wn/fsstore.go` | Returns a `Store` backed by `.wn/` JSON files |
| `SetStatus(store, id, status, opts)` | `internal/wn/status.go` | Central function for all item state transitions (undone, claimed, review, done, closed, suspend) |
| `ItemListStatus(it, now, blocked)` | `internal/wn/progress.go` | Derives display status string from item fields; used for list/JSON output |
| `TopoOrder(items)` | `internal/wn/sort.go` | Dependency-ordered sort; returns `(ordered []*Item, acyclic bool)` |
| `FindRootForCLI()` | `internal/wn/root.go` | Walks up from cwd to find `.wn/`; use in CLI `RunE` functions |
| `ResolveItemID(currentID, requestID)` | `internal/wn/resolve.go` | Resolves empty request ID to current task; returns error if both are empty |
