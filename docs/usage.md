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
