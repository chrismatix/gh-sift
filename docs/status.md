# Status

Sift is young: it works for daily review, and the gaps are listed here.

## What works

- gh-dash config: sections, filters (with `nowModify`), limits, layout widths
  and hidden columns, `repoPaths`, theme colours, compact table, separator,
  preview position, `include`, layered configs, builtin rebinds and custom
  commands with `{{.RepoName}}`, `{{.PrNumber}}`, `{{.HeadRefName}}`,
  `{{.BaseRefName}}`, `{{.Author}}`, `{{.RepoPath}}`.
- PR, issue and notification lists with pagination, search editing, smart
  filtering and periodic refetch.
- List actions: approve, comment, assign, unassign, label, checkout, close,
  reopen, ready, update branch, merge, open in browser, copy number and URL.
- The PR view, Checks, the diff, read tracking, review comments, and stack
  navigation and merging, as in the tour.
- Light and dark terminals.
- Generated files start folded in the diff, as on GitHub: paths marked
  `linguist-generated` in the root `.gitattributes` at the PR head, and
  lockfiles, minified and protobuf files otherwise. `z` unfolds one.

## What does not work yet

- The repo (branches) view, section mode (`ctrl+s`), `watch checks`, `approve
  workflows`, the preview's tabs (`[` `]` in the list), author role icons,
  `dateFormat`, and notification bookmarks and sorting by repository. Their
  config keys are accepted and ignored.
- Logs of running jobs. GitHub's API has none until the job ends, so a running
  check shows its steps live instead.
- In the diff: expanding context beyond what GitHub's diff carries, line
  wrapping (long lines scroll sideways with the arrow keys), the mouse, and
  opening a file in `$EDITOR`.
- `.gitattributes` files below the repository root, which GitHub also honours.
- Commenting on the deleted side of a changed line in the split layout: switch
  to the stacked layout (`2`), where each line is its own row.
- More than 100 review threads or check runs per PR, and stacks deeper than 30,
  are cut off.

## Next steps

1. Context expansion in the diff: fetch the file at the head commit and unfold
   the gap between hunks, as hunk's `z` does.
2. A "what changed since my last review" diff (compare against the commit of
   the viewer's latest review), next to the hunk-level read marks.
3. Editing and deleting pending comments, and rendering suggestions as a
   small diff.
4. Local diffs when the repository is checked out (`repoPaths`), for PRs too
   large for the API and for full-file context.
