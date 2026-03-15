# Project overview

`wn` ("What's Next") is a CLI work item tracker for local use by humans and coding agents. Written in Go (cobra CLI, bubbletea TUI, MCP server). Work items are stored as JSON files under `.wn/` in the project directory.

Always use `make test` or `make all` to run tests rather than `go test` directly — the Makefile sets required environment variables (e.g. `WN_PICKER=numbered` to avoid blocking on fzf) and enforces lint.

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
