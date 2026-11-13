#!/usr/bin/env bash
# Drive gh-sift in a detached tmux session, to look at it without a terminal.
#
#   scripts/tui.sh start <cols> <rows> [gh-sift args...]   read-only unless SIFT_READ_ONLY= is passed
#   scripts/tui.sh keys <key>...                            tmux key names: j Enter C-s Escape ...
#   scripts/tui.sh text                                     the screen as plain text
#   scripts/tui.sh shot <name>                              the screen as /tmp/sift-shots/<name>.png
#   scripts/tui.sh stop
set -euo pipefail
session=sift
root=$(cd "$(dirname "$0")/.." && pwd)
shots=/tmp/sift-shots

case "${1:-}" in
start)
	cols=$2 rows=$3
	shift 3
	tmux kill-session -t $session 2>/dev/null || true
	# TERM is set so colour detection does not cap at 256 colours inside tmux.
	tmux new-session -d -s $session -x "$cols" -y "$rows" -c /tmp \
		"TERM=xterm-256color COLORTERM=truecolor SIFT_READ_ONLY=${SIFT_READ_ONLY-1} $root/gh-sift $*; sleep 600"
	;;
keys)
	shift
	for key in "$@"; do
		tmux send-keys -t $session "$key"
		sleep 0.15
	done
	;;
text) tmux capture-pane -t $session -p ;;
shot)
	mkdir -p $shots/profile
	tmux capture-pane -t $session -e -N -p >"$shots/$2.ansi"
	python3 "$root/scripts/ansi2html.py" "$shots/$2.ansi" "$shots/$2.html"
	cols=$(tmux display -t $session -p '#{pane_width}') rows=$(tmux display -t $session -p '#{pane_height}')
	timeout 60 firefox --headless --profile $shots/profile --screenshot "$shots/$2.png" \
		--window-size=$(((cols * 84 + 9) / 10 + 24)),$((rows * 18 + 24)) "file://$shots/$2.html" >/dev/null 2>&1
	echo "$shots/$2.png"
	;;
stop) tmux kill-session -t $session ;;
*)
	sed -n '2,8p' "$0"
	exit 1
	;;
esac
