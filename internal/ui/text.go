package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func width(text string) int { return ansi.StringWidth(text) }

func truncate(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	return ansi.Truncate(text, limit, "…")
}

// fit makes text exactly `limit` cells wide: cut with an ellipsis or padded.
func fit(text string, limit int) string {
	text = truncate(text, limit)
	return text + strings.Repeat(" ", max(limit-width(text), 0))
}

func fitRight(text string, limit int) string {
	text = truncate(text, limit)
	return strings.Repeat(" ", max(limit-width(text), 0)) + text
}

// spread puts left and right at the two ends of a line of the given width.
func spread(left, right string, limit int) string {
	gap := limit - width(left) - width(right)
	if gap < 1 {
		return fit(left, max(limit-width(right)-1, 0)) + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

// fill paints a whole line of the given width with a style's background.
func fill(style lipgloss.Style, text string, limit int) string {
	return style.Render(fit(text, limit))
}

// block returns exactly `height` lines, each exactly `limit` cells wide.
func block(lines []string, limit, height int) []string {
	out := make([]string, height)
	for index := range out {
		if index < len(lines) {
			out[index] = fit(lines[index], limit)
		} else {
			out[index] = strings.Repeat(" ", limit)
		}
	}
	return out
}

// beside zips two blocks of equal height into lines, with a separator between.
func beside(left []string, separator string, right []string) []string {
	out := make([]string, len(left))
	for index := range left {
		out[index] = left[index] + separator + right[index]
	}
	return out
}

func wrap(text string, limit int) []string {
	if limit < 1 {
		return []string{text}
	}
	return strings.Split(ansi.Wrap(text, limit, ""), "\n")
}

// elapsed is gh-dash's compact age: the largest unit only.
func elapsed(moment time.Time) string {
	if moment.IsZero() {
		return ""
	}
	seconds := int(time.Since(moment).Seconds())
	switch {
	case seconds < 30:
		return "now"
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	case seconds < 3600:
		return fmt.Sprintf("%dm", seconds/60)
	case seconds < 86400:
		return fmt.Sprintf("%dh", seconds/3600)
	case seconds < 7*86400:
		return fmt.Sprintf("%dd", seconds/86400)
	case seconds < 28*86400:
		return fmt.Sprintf("%dw", seconds/(7*86400))
	case seconds < 365*86400:
		return fmt.Sprintf("%dmo", seconds/(28*86400))
	}
	return fmt.Sprintf("%dy", seconds/(365*86400))
}

func ago(moment time.Time) string {
	age := elapsed(moment)
	if age == "now" || age == "" {
		return age
	}
	return age + " ago"
}

func duration(start, end time.Time) string {
	if start.IsZero() {
		return ""
	}
	if end.IsZero() {
		end = time.Now()
	}
	seconds := int(end.Sub(start).Seconds())
	if seconds < 60 {
		return fmt.Sprintf("%ds", max(seconds, 0))
	}
	return fmt.Sprintf("%dm %02ds", seconds/60, seconds%60)
}

func shortNumber(number int) string {
	if number < 1000 {
		return fmt.Sprint(number)
	}
	return fmt.Sprintf("%.1fk", float64(number)/1000)
}

func plural(count int, singular string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %ss", count, singular)
}

func clamp(value, low, high int) int {
	return max(low, min(value, high))
}

// paintBackground puts a background behind a line that is already styled.
// Every style reset inside the line would end the background, so it is
// started again after each one.
func paintBackground(line string, background color.Color) string {
	start := ansi.Style{}.BackgroundColor(background).String()
	return start + strings.ReplaceAll(line, ansi.ResetStyle, ansi.ResetStyle+start) + ansi.ResetStyle
}
