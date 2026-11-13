# A tour of gh-sift: the list, a stacked PR, its failing check, and a review
# in the diff. Record it with scripts/demo.sh, which stages the binary, this
# tape and its config in a directory mounted at /vhs.

Output demo.gif

Set Shell "bash"
Set FontFamily "JetBrainsMono Nerd Font Mono"
Set FontSize 15
Set Width 1600
Set Height 900
Set Padding 12
Set Framerate 12
Set TypingSpeed 60ms
Set Theme { "name": "GitHub Dark", "background": "#0d1117", "foreground": "#e6edf3", "cursor": "#e6edf3", "selection": "#264f78", "black": "#484f58", "red": "#ff7b72", "green": "#3fb950", "yellow": "#d29922", "blue": "#58a6ff", "magenta": "#bc8cff", "cyan": "#39c5cf", "white": "#b1bac4", "brightBlack": "#6e7681", "brightRed": "#ffa198", "brightGreen": "#56d364", "brightYellow": "#e3b341", "brightBlue": "#79c0ff", "brightMagenta": "#d2a8ff", "brightCyan": "#56d4dd", "brightWhite": "#ffffff" }

# Setup, off camera: sift on the PATH as `gh sift`, and one run to fill the
# list cache so the recorded start is what a second start really looks like.
Hide
Type "export COLORTERM=truecolor GH_DASH_CONFIG=/vhs/config.yml XDG_STATE_HOME=/vhs/state XDG_CACHE_HOME=/vhs/cache"
Enter
Type 'gh() { shift; /vhs/gh-sift "$@"; }'
Enter
Type "gh sift"
Enter
Sleep 8s
Type "q"
Type "clear"
Enter
Show

Type "gh sift"
Sleep 600ms
Enter
Sleep 3s

# Down the list onto a stacked PR: its rows share a rail, the preview draws the stack.
Type "j"
Sleep 1.2s
Type "j"
Sleep 3.5s

# Open it, then move up the stack.
Enter
Sleep 3.5s
Type "K"
Sleep 3.5s

# Checks: opens on the failing test, with the log at its error.
Type "2"
Sleep 5s
Type "l"
Sleep 600ms
Type "l"
Sleep 800ms
Type "k"
Sleep 2.5s

# Back down the stack and into the diff. Hunks read earlier are folded.
Type "J"
Sleep 3s
Type "d"
Sleep 4s
Type ".."
Sleep 1.5s
Type "]"
Sleep 1s
Type "jjjjjj"
Sleep 1.2s

# Comment on the line, then check the hunk off as read.
Type "c"
Sleep 700ms
Type "Should Negate guard against the smallest int?"
Sleep 1.2s
Ctrl+S
Sleep 6s
Type "x"
Sleep 4.5s

Hide
Type "q"
Type "q"
