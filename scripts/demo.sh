#!/usr/bin/env bash
# Records docs/screenshots/demo.gif from docs/demo/demo.tape with VHS.
#
# VHS runs from its Docker image, which brings a browser that works. The tour
# drives a real repository, chrismatix/sift-playground, and writes to it (a
# pending review comment, a Viewed mark), so PR #3 is first put back the way
# the tape expects to find it.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
repository=chrismatix/sift-playground
stage=$(mktemp -d)

pull_request=$(gh api graphql -f query='query($owner: String!, $name: String!) { repository(owner: $owner, name: $name) {
  pullRequest(number: 3) { id reviews(states: PENDING, first: 1) { nodes { id } } } } }' \
	-f owner="${repository%/*}" -f name="${repository#*/}" --jq '.data.repository.pullRequest')
pull_request_id=$(jq -r .id <<<"$pull_request")
pending_review=$(jq -r '.reviews.nodes[0].id // empty' <<<"$pull_request")
if [ -n "$pending_review" ]; then
	gh api graphql -f query='mutation($id: ID!) { deletePullRequestReview(input: { pullRequestReviewId: $id }) { clientMutationId } }' -f id="$pending_review" >/dev/null
fi
for path in calc.go calc_test.go; do
	gh api graphql -f query='mutation($id: ID!, $path: String!) { unmarkFileAsViewed(input: { pullRequestId: $id, path: $path }) { clientMutationId } }' \
		-f id="$pull_request_id" -f path="$path" >/dev/null
done

(cd "$root" && CGO_ENABLED=0 go build -o "$stage/gh-sift" .)
cp "$root/docs/demo/demo.tape" "$root/docs/demo/config.yml" "$stage"
# Two hunks of calc.go already read, so the tour shows a second visit.
mkdir -p "$stage/state/gh-sift" "$stage/cache"
cp "$root/docs/demo/read.json" "$stage/state/gh-sift/read.json"

# The list's glyphs need a Nerd Font, which the image does not ship.
fonts=$(dirname "$(fc-list ':family=JetBrainsMono Nerd Font Mono:style=Regular' file | head -1 | cut -d: -f1)")
GH_TOKEN=$(gh auth token) docker run --rm -e GH_TOKEN \
	-v "$stage":/vhs -v "$fonts":/usr/share/fonts/truetype/nerd:ro \
	ghcr.io/charmbracelet/vhs demo.tape

cp "$stage/demo.gif" "$root/docs/screenshots/demo.gif"
echo "$root/docs/screenshots/demo.gif"
