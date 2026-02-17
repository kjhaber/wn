# wn: "What's Next"

Yet another todo-list style CLI application for tracking work items on a project.

I've noticed that when working on agent-based software development projects I compose a prompt, execute it in Cursor or Claude Code, watch the agent run the prompt to completion, and then compose the next prompt based on the outcome of the agent process.  Not very efficient.

From there I started writing prompts into a NEXT-STEPS.md file as I thought of them, pasting them into Cursor prompt when agent work completes (and as I think of it).  An improvement, but still pretty basic and manual, and not really a good system for agents to operate on directly.  Why have me in the loop to copy-and-paste?  Why not make it easier to track done/open?  So at some point I want `wn` to be easy for coding agents to track work items as well.


## Commands

`wn init`
  - wn initialized at "~/projects/myproject/.wn"
  ... I like the idea of hunting up directories to find current wn tracker.
  ... I also like the idea of not polluting project directory with the todo list but that'll be harder to manage :P
  ... if source files are kept outside of the project, it might keep agents from accidentally finding contents via grep or other searches in the project directory.  probably not worth worrying about that though.

`wn add -m "introduce feature XXXX"`
  - added entry af123
  ... opens $EDITOR if no desc/message given

`wn add -m "introduce feature YYYY"`
  - added entry bc234
  .. include arg to set tags, allow multiple tag args

`wn rm af123`
  - removed entry af123

`wn edit af123`
  ... opens $EDITOR

`wn tag af123 'whatever'`
`wn untag af123 'whatever'`
  .. or `wn tag af123 '+whatever'` and `wn tag af123 '-whatever'`?
  .. tag names allow alphanumeric + dash + underscore, maxlen 32
  .. might be interesting for tags to have more detailed description/notes, allowing more context in the task description.


`wn list`
`wn list --undone`
`wn list --done`
`wn list --all`
  - af123: introduce feature XXXX
  - bc234: introduce feature YYYY
  .. include arg for filter by tag
  .. --undone is default 
  .. dependency order, might add options for sort order (create time, edit time), may end up with some syntax for arbitrary filters and sorting by attributes

`wn depend af123 --on bc234`
  - entry af123 depends on bc234
  - circular dependency detected, could not mark entry af123 dependent on bc234

`wn rmdepend af123 --on bc234`
  - entry af123 no longer depends on bc234

`wn done af123 -m "Completed in git commit ca1f722a`
  - entry af123 marked complete
  - dependency bc234 not complete, use --force to mark complete anyway
  .. when agents or humans mark a task complete it'd be good to ensure context is provided about the completion, e.g. agents provide the git commit(s) corresponding to task

`wn undone af123`
  - entry af123 marked undone

`wn log af123`
  .. show history of create, dependency changes, tag changes, marked done/undone, etc for a work item

`wn next`
  - af123: introduce feature XXXX
  - picks a new "current" task
    .. still debating whether a "current" task is a good idea - could be a dead end if multiple agents are working on tasks from this queue.
    .. I like idea of treating task list as a queue where workers mark a task as "taken/in-progress" by a worker, this can expire
  .. copies task content to clipboard? (maybe not as default, but with -c arg or setting enabled?  omit to start but experiment with this later.)


`wn` [no args]
  - current task: [af123] introduce feature XXXX
  .. copies task content to clipboard?

`wn pick`
  - pick a "current" task directly
  - interactive CLI prompt to pick a "current" task directly - ideally uses fzf fuzzy finder
  .. copies task content to clipboard?

`wn settings`
  .. opens ~/.config/wn/settings.json in $EDITOR
  .. should honor XDG directory settings

`wn help`
`wn help <subcommand>`
.. what you'd expect

`wn version`
`wn --version`
.. what you'd expect

`wn export`
- output all work item state as a single file

`wn import <export filename>`
- accept the content of exported file as work items in project
.. my thought is this should replace existing state entirely...


`wn tui`
.. launch some kind of terminal text-based UI.  my rough idea here is to make it easier to create/modify work items in bulk, browse work items, etc.  Probably low priority.


## Implementation notes
Not sure what programming language/environment to choose for `wn`.
- In general I prefer strong/statically-typed languages over dynamically typed languages as projects grow large - I've found they're good for catching mistakes early, esp when software gets too large to keep in head at once or test everything manually.
- I've done a CLI with node/typescript before and thought that was ok.  I liked yargs for command line arg parsing and providing shell tab completion, though I expect most modern languages to have a reasonable answer for that.
- Having a Golang single binary rather than depending on node would be really nice too.  Good performance as projects get thousands of items added.
- I want the tool to be easy to install
- Language should be popular/mainstream enough for agents like Cursor/Claude Code to be able to work with them efficiently.

The tool must support shell tab completion in zsh/bash/etc.

The tool must scale well to managing thousands of work items in a project.  Maybe it uses sqlite to help index tasks?  Would be nice if wn source-of-truth was fully trackable in git and played nicely with git diffs, merges, etc.



