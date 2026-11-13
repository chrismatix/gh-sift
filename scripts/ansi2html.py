#!/usr/bin/env python3
"""ansi2html.py <in.ansi> <out.html>: render a tmux capture as an HTML page.

Set LIGHT=1 for a light terminal."""
import html, os, re, subprocess, sys

BASE = ["#484f58", "#ff7b72", "#3fb950", "#d29922", "#58a6ff", "#bc8cff", "#39c5cf", "#b1bac4",
        "#6e7681", "#ffa198", "#56d364", "#e3b341", "#79c0ff", "#d2a8ff", "#56d4dd", "#ffffff"]

LIGHT = bool(os.environ.get("LIGHT"))
if LIGHT:
    BASE = ["#24292f", "#cf222e", "#116329", "#4d2d00", "#0969da", "#8250df", "#1b7c83", "#6e7781",
            "#57606a", "#a40e26", "#1a7f37", "#633c01", "#218bff", "#a475f9", "#3192aa", "#8c959f"]

def xterm(index):
    if index < 16:
        return BASE[index]
    if index < 232:
        index -= 16
        levels = [0, 95, 135, 175, 215, 255]
        return "#%02x%02x%02x" % (levels[index // 36], levels[index // 6 % 6], levels[index % 6])
    grey = 8 + (index - 232) * 10
    return "#%02x%02x%02x" % (grey, grey, grey)

def convert(text):
    out, state = [], {"fg": None, "bg": None, "bold": False, "faint": False, "underline": False, "reverse": False}
    position = 0
    for match in re.finditer(r"\x1b\[([0-9;]*)m", text):
        out.append(span(state, text[position:match.start()]))
        position = match.end()
        codes = [int(code or 0) for code in match.group(1).split(";")] if match.group(1) else [0]
        index = 0
        while index < len(codes):
            code = codes[index]
            if code == 0: state.update(fg=None, bg=None, bold=False, faint=False, underline=False, reverse=False)
            elif code == 1: state["bold"] = True
            elif code == 2: state["faint"] = True
            elif code == 4: state["underline"] = True
            elif code == 7: state["reverse"] = True
            elif code == 22: state.update(bold=False, faint=False)
            elif code == 24: state["underline"] = False
            elif code == 27: state["reverse"] = False
            elif 30 <= code <= 37: state["fg"] = BASE[code - 30]
            elif 90 <= code <= 97: state["fg"] = BASE[code - 82]
            elif code == 39: state["fg"] = None
            elif 40 <= code <= 47: state["bg"] = BASE[code - 40]
            elif 100 <= code <= 107: state["bg"] = BASE[code - 92]
            elif code == 49: state["bg"] = None
            elif code in (38, 48):
                key = "fg" if code == 38 else "bg"
                if codes[index + 1] == 5:
                    state[key] = xterm(codes[index + 2]); index += 2
                else:
                    state[key] = "#%02x%02x%02x" % tuple(codes[index + 2:index + 5]); index += 4
            index += 1
    out.append(span(state, text[position:]))
    return "".join(out)

def span(state, text):
    if not text:
        return ""
    fg, bg = state["fg"], state["bg"]
    if state["reverse"]:
        fg, bg = bg or "#0d1117", fg or "#e6edf3"
    style = []
    if fg: style.append("color:" + fg)
    if bg: style.append("background:" + bg)
    if state["bold"]: style.append("font-weight:bold")
    if state["faint"]: style.append("opacity:.55")
    if state["underline"]: style.append("text-decoration:underline")
    return '<span style="%s">%s</span>' % (";".join(style), html.escape(text))

def font(style):
    """Path of the Nerd Font the screenshots use, wherever fontconfig has it."""
    listing = subprocess.run(["fc-list", ":family=JetBrainsMono Nerd Font Mono:style=" + style, "file"], capture_output=True, text=True).stdout
    return listing.split(":")[0].strip()

page = """<!doctype html><meta charset=utf-8><style>
@font-face { font-family: T; src: url(file://%s); }
@font-face { font-family: T; font-weight: bold; src: url(file://%s); }
body { margin: 12px; background: BG; color: FG; }
pre { font: 14px/18px T, monospace; margin: 0; }
</style><pre>%s</pre>""" % (font("Regular"), font("Bold"), convert(open(sys.argv[1], encoding="utf-8").read()))
page = page.replace("BG", "#ffffff" if LIGHT else "#0d1117").replace("FG", "#1f2328" if LIGHT else "#e6edf3")
open(sys.argv[2], "w", encoding="utf-8").write(page)
