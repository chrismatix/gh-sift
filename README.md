# gh-sift

<p>
  <img src="https://github.com/chrismatix/gh-sift/actions/workflows/test.yml/badge.svg" alt="Test status">
  <a href="LICENSE"><img src="https://img.shields.io/github/license/chrismatix/gh-sift.svg" alt="MIT license"></a>
</p>

Sift **is** one terminal app for the whole GitHub pull request review.

Sift **reads** your [gh-dash](https://github.com/dlvhdr/gh-dash) config, so your sections and keys are already there.

Sift **remembers** what you have read, and treats stacked PRs as first class.

What it feels like:

<img src="docs/demo/demo.gif" alt="gh-sift demo" width="800">

## Highlights

- 📋 gh-dash's lists, from the config you already have
- 🔎 A full PR view: conversation, review threads, merge status
- 🥞 Stacked PRs: one key up or down, stack-aware merge
- 🚦 GitHub Actions that open on the failure, with the log at its error
- ↔️ Split or stacked diff with syntax highlighting
- ✅ Read tracking, synced with GitHub's Viewed checkbox
- 💬 Comment, suggest, reply, resolve and approve in the diff
- 📦 Single binary

## Installation

No release binaries yet, so build from a checkout (Go 1.26+):

```shell
git clone https://github.com/chrismatix/gh-sift && cd gh-sift
go build -o gh-sift .
gh extension install .
```

Then run `gh sift`, or `gh sift 123` to open a PR directly.

## Inspirations

- [gh-dash](https://github.com/dlvhdr/gh-dash): the list and its config. Sift opens the PR and its diff in the same process.
- [hunk](https://github.com/modem-dev/hunk): the diff. In sift its notes are GitHub review threads, and hunks can be marked read.
- [gh-enhance](https://github.com/dlvhdr/gh-enhance): the Checks tab. In sift it lives inside the PR view.

## Documentation

- [Usage and keys](docs/usage.md)
- [What works and what is next](docs/status.md)
- [Design](docs/design.md)

## Versioning

Sift is pre-release: anything may change until there is a tagged version.
