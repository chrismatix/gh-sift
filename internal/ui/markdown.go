package ui

import (
	"regexp"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
)

var (
	htmlComment   = regexp.MustCompile(`(?s)<!--.*?-->`)
	imageTag      = regexp.MustCompile(`(?i)<img\b(?:[^>]*?\salt="([^"]*)")?[^>]*>`)
	emptyImage    = regexp.MustCompile(`!\[\]\([^)\s]*\)`)
	detailsBlock  = regexp.MustCompile(`(?is)<details>.*?</details>`)
	htmlTag       = regexp.MustCompile(`(?i)</?[a-z][a-z0-9]*\b[^>]*>`)
	blankLines    = regexp.MustCompile(`\n{3,}`)
	danglingBold  = regexp.MustCompile(`(^|\n|\s)\*\*[ \t]+`)
	severityBadge = regexp.MustCompile(`(?i)^\W*(Critical|Major|Minor|Trivial|Nitpick)\b[^\n]*·`)
)

// cleanBody drops what GitHub hides or folds when it renders a comment: HTML
// comments, collapsed <details> (bot prompts, mostly) and tags around text.
func cleanBody(text string) string {
	text = strings.ReplaceAll(text, "\r", "")
	text = htmlComment.ReplaceAllString(text, "")
	text = detailsBlock.ReplaceAllString(text, "")
	text = imageTag.ReplaceAllStringFunc(text, func(tag string) string {
		if alt := imageTag.FindStringSubmatch(tag)[1]; alt != "" {
			return "[image: " + alt + "]"
		}
		return ""
	})
	text = emptyImage.ReplaceAllString(text, "")
	text = htmlTag.ReplaceAllString(text, "")
	// Removing an icon leaves "** Minor**", which is no longer bold.
	text = danglingBold.ReplaceAllString(text, "$1**")
	return strings.TrimSpace(blankLines.ReplaceAllString(text, "\n\n"))
}

// severity reads the badge CodeRabbit opens its findings with.
func severity(body string) string {
	for _, line := range strings.Split(cleanBody(body), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if match := severityBadge.FindStringSubmatch(line); match != nil {
			return strings.ToLower(match[1])
		}
		return ""
	}
	return ""
}

var (
	listMarker = regexp.MustCompile(`^\s*(#{1,6}|>|[-*+]|\d+[.)])\s+`)
	linkSyntax = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
)

// summary is one line saying what a comment is about, for list rows.
func summary(body string) string {
	heading := ""
	for _, raw := range strings.Split(cleanBody(body), "\n") {
		if strings.HasPrefix(strings.TrimSpace(raw), "```") || severityBadge.MatchString(raw) {
			continue
		}
		plain := listMarker.ReplaceAllString(raw, "")
		plain = linkSyntax.ReplaceAllString(plain, "$1")
		plain = strings.TrimSpace(strings.NewReplacer("**", "", "__", "", "`", "").Replace(plain))
		if plain == "" {
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(raw), "#") {
			return plain
		}
		if heading == "" {
			heading = plain
		}
	}
	return heading
}

// markdown renders a comment body to lines no wider than limit.
func (app *App) markdown(body string, limit int) []string {
	body = cleanBody(body)
	if body == "" {
		return nil
	}
	body = strings.ReplaceAll(body, "```suggestion", "**Suggested change**\n```")
	style := styles.LightStyleConfig
	if app.theme.Dark {
		style = styles.DarkStyleConfig
	}
	// The caller indents; glamour's own page margin would double it.
	zero := uint(0)
	style.Document = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: style.Document.Color}, Margin: &zero}
	// Quieter than glamour's defaults: headings without the literal "##",
	// inline code as coloured text instead of a red block.
	bold, heading, code := true, "#58A6FF", "#D2A8FF"
	if !app.theme.Dark {
		heading, code = "#0969DA", "#8250DF"
	}
	style.Heading = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: &heading, Bold: &bold, BlockSuffix: "\n"}}
	for _, level := range []*ansi.StyleBlock{&style.H1, &style.H2, &style.H3, &style.H4, &style.H5, &style.H6} {
		*level = ansi.StyleBlock{}
	}
	style.Code = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: &code}}
	renderer, err := glamour.NewTermRenderer(glamour.WithStyles(style), glamour.WithWordWrap(limit))
	if err != nil {
		return wrap(body, limit)
	}
	rendered, err := renderer.Render(body)
	if err != nil {
		return wrap(body, limit)
	}
	lines := strings.Split(strings.Trim(rendered, "\n"), "\n")
	for index, line := range lines {
		lines[index] = strings.TrimRight(line, " ")
	}
	return lines
}
