# Usage

```shell
gh sift                      # the list, from your gh-dash config
gh sift 123                  # PR 123 of the repository you are in
gh sift owner/repo#123       # any PR
gh sift <PR URL>             # as copied from the browser; /files and /checks open that tab
gh sift HEAD                 # the PR of the branch you are on
gh sift my-branch            # the PR of another branch: the open one, else the latest
gh sift --config other.yml   # a config layered over the global one
```

Opened on a PR, sift shows the same PR view as from the list, without loading
the list. `q` quits from there.

Sift reads `~/.config/gh-dash/config.yml` (and `$GH_DASH_CONFIG`, and
`.gh-dash.yml` in the repository) and never writes to it. Without one it uses
gh-dash's default sections.

- `gh` must be authenticated.
- Merging a stack needs the [gh stack](https://github.com/github/gh-stack) extension.
- The list's glyphs need a [Nerd Font](https://www.nerdfonts.com), as in gh-dash.
- `SIFT_READ_ONLY=1 gh sift` refuses every write.

### Merge queue sections

A PR section with `mergeQueue` in place of `filters` lists a repository's merge
queue in order, with each entry's position, state and estimated time to merge.
It is a sift addition that gh-dash does not know.

```yaml
prSections:
  - title: Merge queue
    mergeQueue:
      repo: owner/name
      branch: main   # optional; the default branch when left out
```

### Notifications

`s` cycles PRs → issues → notifications, as in gh-dash, and
`defaults.view: notifications` starts there. Sections come from
`notificationsSections` (gh-dash's eight defaults otherwise), each filtered by:

- `repo:owner/name`: that repository's notifications only (smart filtering
  adds the current one, as for PRs).
- `is:unread`, `is:read`, `is:all`: which threads to list. Without one, read
  threads are listed too unless `includeReadNotifications: false`.
- `reason:author`, `reason:mention`, `reason:review-requested` and the rest of
  GitHub's reasons; `reason:participating` means author, comment, mention,
  review requested, assign and state change.

| Key | Does |
|---|---|
| `enter` | a PR opens in sift's PR view, anything else in the browser; marks it read |
| `o` | open in the browser and mark read |
| `m` / `M` | mark read / mark every notification read (asks first) |
| `D` / `alt+d` | mark done / mark the whole section done (asks first) |
| `u` | unsubscribe from the thread |
| `t` | toggle smart filtering |

Done threads are remembered in `~/.local/state/gh-sift/done.json`, because
GitHub keeps listing them; one with new activity comes back. Custom commands
under `keybindings.notifications` get `{{.RepoName}}` and `{{.PrNumber}}` or
`{{.IssueNumber}}`, and PR and issue commands also run on those subjects.

## Keys

`?` shows the keys of the screen you are on. The list keeps gh-dash's defaults
and honours `keybindings:` from the config, the diff keeps hunk's keys and the
Checks tab gh-enhance's. These are sift's own:

| Key | Where | Does |
|---|---|---|
| `enter` / `d` | list | open the PR / its diff |
| `K` / `J` | everywhere | PR above / below in the stack |
| `m` | everywhere | merge, stack aware |
| `tab`, `1`-`4` | PR view | switch tab |
| `a` | PR view, diff | submit a review |
| `x` | diff | check off: hunk read, thread resolved |
| `v` | diff | file viewed on GitHub |
| `c` / `S` / `V` | diff | comment / suggest / select lines |
| `esc` or `q` | PR view, diff | back one level |

## Development

```shell
go test ./...
scripts/tui.sh start 170 44 owner/repo#123   # run it in a detached tmux, read-only
scripts/tui.sh shot diff                     # the screen as a PNG
scripts/demo.sh                              # re-record the demo with VHS
```
