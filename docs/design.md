# gh-sift design

One process, three screens: the gh-dash list, a PR view, and a diff view. This
document records what was learned from the tools it replaces and the decisions
made for the first version.

## Review of gh-dash

gh-dash (Go, Bubble Tea v2, Lip Gloss v2) is a list with a preview pane.

- **Root model** (`internal/tui/ui.go`) owns the section lists per view (PRs,
  issues, notifications, repo), the tabs, the footer, a task map with a spinner,
  and one preview pane per view. Shared state (sizes, config, theme, styles)
  travels in a `ProgramContext` pointer.
- **Sections** each own a table, a search bar and a prompt. A section's filter
  becomes the search `is:pr archived:false <filters> sort:updated`, fetched with
  cursor pagination (`first: limit`), refetched every `refetchIntervalMinutes`.
  "Smart filtering" prepends `repo:<current repo>` when launched inside a
  checkout.
- **Key routing** is a priority chain: focused text inputs, then user-defined
  keybindings (which therefore shadow builtins), then universal keys, then
  view keys. Custom commands are Go templates run through `$SHELL -c` with the
  TUI suspended.
- **Actions** shell out to `gh pr …` in the background and patch the row
  optimistically.
- **Preview** enriches the selected row with a second, heavier GraphQL query.
  There is no cache beyond the rows in memory.
- **Look**: two-line rows (repo, number, author on the first, bold title on the
  second) with a faint rule between them, cells padded by one column, a tab row
  with a thick rule under it, a one-line footer on the selection background.
  Colour is used for state only.

## Review of hunk

hunk has the right reading experience: one continuous scroll through all files,
a file tree on the left, a coloured rail per row that dims outside the current
hunk, split and stacked layouts, word-level emphasis, notes drawn as cards
under their line. Sift copies that structure, keymap and palette (GitHub
dark/light).

## Decisions

**Stack: Go, Bubble Tea v2, Lip Gloss v2.** Same as gh-dash, so its config
schema, styles and idioms port directly, and a gh extension in Go is a single
binary with fast startup. Chroma does syntax highlighting, glamour renders
markdown, go-gh supplies authenticated GraphQL and REST clients without
spawning `gh` per request.

**Config: read gh-dash's file, never write it.** `internal/config` mirrors
gh-dash's schema and layering (defaults, global `config.yml`, then `--config`
/ `$GH_DASH_CONFIG` / `<repo>/.gh-dash.yml`, with `include`, sections replaced
per layer and keybindings unioned by key). Keys with a `builtin:` rebind the
builtin, keys with a `command:` run as custom commands and shadow builtins,
exactly as in gh-dash. Not honoured: `pager.diff` (the diff is built in),
notification and branch sections (not built yet).

**One screen stack instead of panes.** `list → PR view → diff`. `enter` goes
in, `esc` or `q` goes back, and the list keeps its position. The PR view has
four tabs: Overview, Checks, Threads, Files. Files *is* the diff view; it drops
the PR header to one line to give the diff the room.

**Stacks come from GitHub, not from branch names.** `PullRequest.stackEntry {
position stack { id size entries { pullRequest } } }` returns the whole stack in
the list query and the detail query, so nothing walks base refs. The stack is
always drawn top first with the base at the bottom. `K` and `J` move up and
down the stack from all three screens; `[` and `]` could not be used because
gh-dash binds them in the list and hunk binds them in the diff.

**Merging goes through `gh`.** `gh stack merge <pr> --yes --<method>` for a
stacked PR (up to the selected PR, or the whole stack), `gh pr merge` otherwise.
These are the only writes that shell out; the merge logic is GitHub's and is
not worth reimplementing.

**Read state is two layers.**

- *File viewed* is GitHub's checkbox (`viewerViewedState`,
  `markFileAsViewed`, `unmarkFileAsViewed`). It is the source of truth and is
  shared with the web UI.
- *Hunk read* is local, in `~/.local/state/gh-sift/read.json`, keyed by
  repository, PR, file path and a hash of the hunk's added and removed lines.
  Context and line numbers are left out of the hash, so a hunk that only moves
  after a push stays read and a hunk whose content changed becomes unread.

They meet at the file: marking the last unread hunk of a file marks the file
viewed on GitHub, and marking a file viewed marks its hunks read. When GitHub
dismisses a viewed mark because the file changed, the hunk hashes still tell
which parts are new.

**Review comments use GitHub's pending review.** A comment written in the diff
is added to the viewer's pending review (`addPullRequestReviewThread`), so it
shows up as pending on github.com too and nothing is lost if sift exits.
Submitting (approve, request changes, comment) publishes them together.
With comments pending, `a` then `enter` publishes them with no summary.
Replies and resolving go straight through.

**The Checks tab follows gh-enhance.** dlvhdr's gh-enhance lays GitHub Actions
out as four panes (runs, jobs, steps, log), opens on the failure and lets a
step position the log. Sift's Checks tab reproduces that layout and its keys
inside the PR view, in sift's own colours. Checks are grouped into runs by
their workflow run; a commit status or third-party check is a run of its own
and shows the summary it reports where a log would be. The log is fetched raw
from the jobs API, which does not name steps, so a step is found in it by
matching the step's start time against the line timestamps, as gh-enhance
does.

**Live checks are steps, not logs.** GitHub's API serves a job's log only once
the job has finished (the endpoint 404s before that), so a running check shows
its steps with durations, polled every five seconds, and the log opens when the
job ends.

**The list is cached on disk.** GitHub's search takes two to four seconds
(gh-dash's query is slower still: `mergeStateStatus` alone adds about four
seconds for twenty PRs, so the list query leaves it out). The last result of
each section is kept in `~/.cache/gh-sift/sections.json`, shown at once on
start and replaced when the search returns. PR details and diffs are cached in
memory for the session, and stack neighbours are fetched ahead.

**A read-only switch for testing.** `SIFT_READ_ONLY=1` makes the client
refuse every write. It exists so the app can be exercised against
repositories that must not be touched.

## Layout of the code

```
main.go                 flags, config, program start
internal/config         gh-dash compatible config
internal/gh             GitHub client: search, PR detail, stacks, checks, writes
internal/diff           unified diff parser, split pairing, word diff, highlighting
internal/readstate      local hunk read state
internal/ui             the screens: list, PR view, diff view, overlays
```

`internal/ui` is one package with one root model. The screens are plain structs
owned by the root, not independent Bubble Tea models: they need each other's
data (the list row seeds the PR view, the PR view's threads appear in the
diff), and a message bus between components was most of gh-dash's complexity.

## Keys

The list keeps gh-dash's keys. The diff keeps hunk's. Additions:

| Key | Where | Does |
|---|---|---|
| `enter` | list | open the PR view |
| `d` | list, PR view | open the diff |
| `K` / `J` | everywhere | PR above / below in the stack |
| `m` | list, PR view, diff | merge, stack aware |
| `x` | diff | check off what is under the cursor: hunk read, thread resolved |
| `v` | diff | toggle the file viewed on GitHub |
| `H` | diff | show read hunks dimmed instead of folded |
| `c` / `S` | diff | comment / suggest a change on the line or selection |
| `V` | diff | start a line selection |
| `r` | diff, threads | reply to the thread |
| `a` | PR view, diff | submit a review |
