# Status

Sift is young: it works for daily review, and the gaps are listed here.

## What works

- gh-dash config: sections, filters (with `nowModify`), limits, layout widths
  and hidden columns, `repoPaths`, theme colours, compact table, separator,
  preview position, `include`, layered configs, builtin rebinds and custom
  commands with `{{.RepoName}}`, `{{.PrNumber}}`, `{{.HeadRefName}}`,
  `{{.BaseRefName}}`, `{{.Author}}`, `{{.RepoPath}}`.
- PR and issue lists with pagination, search editing, smart filtering and
  periodic refetch.
- List actions: approve, comment, assign, unassign, label, checkout, close,
  reopen, ready, update branch, merge, open in browser, copy number and URL.
- The PR view, Checks, the diff, read tracking, review comments, and stack
  navigation and merging, as in the tour.
- Light and dark terminals.

## What does not work yet

- Notifications and the repo (branches) view, section mode (`ctrl+s`), `watch
  checks`, `approve workflows`, the preview's tabs (`[` `]` in the list), author
  role icons and `dateFormat`. Their config keys are accepted and ignored.
- Logs of running jobs. GitHub's API has none until the job ends, so a running
  check shows its steps live instead.
- In the diff: expanding context beyond what GitHub's diff carries, line
  wrapping (long lines scroll sideways with the arrow keys), the mouse, and
  opening a file in `$EDITOR`.
- Commenting on the deleted side of a changed line in the split layout: switch
  to the stacked layout (`2`), where each line is its own row.
- More than 100 review threads or check runs per PR, and stacks deeper than 30,
  are cut off.
- Release binaries, so `gh extension install chrismatix/gh-sift` does not work
  yet.

## Next steps

1. Release workflow with `gh-extension-precompile`, so the extension installs
   without a Go toolchain.
2. Notifications view, the last gh-dash screen people live in.
3. Context expansion in the diff: fetch the file at the head commit and unfold
   the gap between hunks, as hunk's `z` does.
4. A "what changed since my last review" diff (compare against the commit of
   the viewer's latest review), next to the hunk-level read marks.
5. Editing and deleting pending comments, and rendering suggestions as a
   small diff.
6. Local diffs when the repository is checked out (`repoPaths`), for PRs too
   large for the API and for full-file context.
