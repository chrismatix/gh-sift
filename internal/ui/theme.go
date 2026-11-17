package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"

	"github.com/chrismatix/gh-sift/internal/config"
)

// Theme holds every colour the UI uses. The list colours are gh-dash's
// defaults; the diff colours are hunk's GitHub palette.
type Theme struct {
	Dark bool

	PrimaryText   color.Color
	SecondaryText color.Color
	FaintText     color.Color
	InvertedText  color.Color
	SuccessText   color.Color
	WarningText   color.Color
	ErrorText     color.Color

	PrimaryBorder   color.Color
	SecondaryBorder color.Color
	FaintBorder     color.Color

	SelectedBackground color.Color
	FlashBackground    color.Color
	SearchMatch        color.Color

	Open   color.Color
	Closed color.Color
	Merged color.Color
	// The one accent that is not a state: stacks.
	Stack color.Color
	Focus color.Color

	Panel           color.Color
	LineNumber      color.Color
	AddedSign       color.Color
	RemovedSign     color.Color
	AddedLine       color.Color
	AddedWord       color.Color
	RemovedLine     color.Color
	RemovedWord     color.Color
	CursorLine      color.Color
	Note            color.Color
	SyntaxStyleName string
}

func NewTheme(dark bool, colors *config.ColorTheme) Theme {
	pick := func(light, darkValue string) color.Color {
		if dark {
			return lipgloss.Color(darkValue)
		}
		return lipgloss.Color(light)
	}
	theme := Theme{
		Dark:               dark,
		PrimaryText:        pick("0", "15"),
		SecondaryText:      pick("244", "251"),
		FaintText:          pick("246", "245"),
		InvertedText:       pick("15", "236"),
		SuccessText:        pick("#1A7F37", "#3DF294"),
		WarningText:        pick("#9A6700", "11"),
		ErrorText:          pick("1", "9"),
		PrimaryBorder:      pick("8", "8"),
		SecondaryBorder:    pick("8", "7"),
		FaintBorder:        pick("254", "236"),
		SelectedBackground: pick("254", "236"),
		FlashBackground:    pick("#FFF1B8", "#5C5426"),
		SearchMatch:        pick("#FFD33D", "#9E6A03"),
		Open:               lipgloss.Color("#42A0FA"),
		Closed:             pick("#C38080", "#C38080"),
		Merged:             lipgloss.Color("#A371F7"),
		Stack:              pick("#8250DF", "#BC8CFF"),
		Focus:              lipgloss.Color("#42A0FA"),
		Panel:              pick("#F6F6F6", "#1E2329"),
		LineNumber:         pick("#8C959F", "#6E7681"),
		AddedSign:          pick("#116329", "#3FB950"),
		RemovedSign:        pick("#CF222E", "#F85149"),
		AddedLine:          pick("#E2ECE5", "#12251D"),
		AddedWord:          pick("#B6DDC0", "#1C4A2B"),
		RemovedLine:        pick("#F9E4E6", "#3C1E21"),
		RemovedWord:        pick("#F1B9BE", "#6B2A2E"),
		CursorLine:         pick("#EAEEF2", "#262C36"),
		Note:               pick("#9A6700", "#BB8009"),
		SyntaxStyleName:    map[bool]string{true: "github-dark", false: "github"}[dark],
	}
	if colors == nil {
		return theme
	}
	// As in gh-dash, a configured colour applies to light and dark alike.
	override := func(target *color.Color, value string) {
		if value != "" {
			*target = lipgloss.Color(value)
		}
	}
	override(&theme.PrimaryText, colors.Text.Primary)
	override(&theme.SecondaryText, colors.Text.Secondary)
	override(&theme.FaintText, colors.Text.Faint)
	override(&theme.InvertedText, colors.Text.Inverted)
	override(&theme.SuccessText, colors.Text.Success)
	override(&theme.WarningText, colors.Text.Warning)
	override(&theme.ErrorText, colors.Text.Error)
	override(&theme.PrimaryBorder, colors.Border.Primary)
	override(&theme.SecondaryBorder, colors.Border.Secondary)
	override(&theme.FaintBorder, colors.Border.Faint)
	override(&theme.SelectedBackground, colors.Background.Selected)
	return theme
}

func (theme Theme) text(foreground color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(foreground)
}

func (theme Theme) primary() lipgloss.Style   { return theme.text(theme.PrimaryText) }
func (theme Theme) secondary() lipgloss.Style { return theme.text(theme.SecondaryText) }
func (theme Theme) faint() lipgloss.Style     { return theme.text(theme.FaintText) }
func (theme Theme) success() lipgloss.Style   { return theme.text(theme.SuccessText) }
func (theme Theme) warning() lipgloss.Style   { return theme.text(theme.WarningText) }
func (theme Theme) failure() lipgloss.Style   { return theme.text(theme.ErrorText) }
func (theme Theme) bold() lipgloss.Style      { return theme.primary().Bold(true) }
func (theme Theme) stack() lipgloss.Style     { return theme.text(theme.Stack) }

// Nerd Font glyphs, the same ones gh-dash draws.
const (
	glyphOpenPR      = ""
	glyphDraftPR     = ""
	glyphClosedPR    = ""
	glyphMergedPR    = ""
	glyphMergeQueue  = ""
	glyphOpenIssue   = ""
	glyphClosedIssue = ""
	glyphComments    = ""
	glyphReview      = "\U000f0be2"
	glyphApproved    = "\U000f012c"
	glyphChanges     = ""
	glyphCommented   = ""
	glyphWaiting     = ""
	glyphSuccess     = ""
	glyphFailure     = "\U000f0159"
	glyphNone        = ""
	glyphCI          = ""
	glyphLines       = ""
	glyphUpdated     = "\U000f19bb"
	glyphCreated     = "\U000f1862"
	glyphRepo        = ""
	glyphStack       = "\U000f0328"
	glyphReactions   = ""
)

// stateStyle colours a PR by its state, the same way everywhere.
func (theme Theme) stateGlyph(state string, isDraft, inMergeQueue bool) string {
	switch {
	case state == "MERGED":
		return theme.text(theme.Merged).Render(glyphMergedPR)
	case state == "CLOSED":
		return theme.text(theme.Closed).Render(glyphClosedPR)
	case inMergeQueue:
		return theme.warning().Render(glyphMergeQueue)
	case isDraft:
		return theme.faint().Render(glyphDraftPR)
	}
	return theme.text(theme.Open).Render(glyphOpenPR)
}

func (theme Theme) stateLabel(state string, isDraft, inMergeQueue bool) string {
	label, background := "Open", theme.Open
	switch {
	case state == "MERGED":
		label, background = "Merged", theme.Merged
	case state == "CLOSED":
		label, background = "Closed", theme.Closed
	case inMergeQueue:
		label, background = "Queued", theme.WarningText
	case isDraft:
		label, background = "Draft", theme.FaintText
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#0D1117")).Background(background).Bold(true).Padding(0, 1).Render(label)
}

func (theme Theme) checksGlyph(state string) string {
	switch state {
	case "SUCCESS":
		return theme.success().Render(glyphSuccess)
	case "PENDING", "EXPECTED":
		return theme.warning().Render(glyphWaiting)
	case "FAILURE", "ERROR":
		return theme.failure().Render(glyphFailure)
	}
	return theme.faint().Render(glyphNone)
}

// checkMark is the plain-Unicode check state used inside the PR view.
func (theme Theme) checkMark(state string) string {
	switch state {
	case "SUCCESS":
		return theme.success().Render("✓")
	case "PENDING", "EXPECTED":
		return theme.warning().Render("●")
	case "FAILURE", "ERROR":
		return theme.failure().Render("✗")
	}
	return theme.faint().Render("·")
}

func (theme Theme) reviewGlyph(decision string, reviewCount int) string {
	switch {
	case decision == "APPROVED":
		return theme.success().Render(glyphApproved)
	case decision == "CHANGES_REQUESTED":
		return theme.failure().Render(glyphChanges)
	case reviewCount > 0:
		return theme.faint().Render(glyphCommented)
	}
	return theme.warning().Render(glyphWaiting)
}
