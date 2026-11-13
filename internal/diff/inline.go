package diff

import "unicode"

// Span is a byte range [Start, End) of a line that changed.
type Span struct {
	Start int
	End   int
}

// InlineSpans finds what changed between a deleted line and the added line
// that replaced it. It returns nothing when the lines share too little for
// word-level marks to mean anything.
func InlineSpans(oldText, newText string) (oldSpans, newSpans []Span) {
	oldTokens, newTokens := tokenize(oldText), tokenize(newText)
	// The LCS table is quadratic; very long lines are not worth it.
	if len(oldTokens)*len(newTokens) > 250_000 {
		return nil, nil
	}

	lengths := make([][]int, len(oldTokens)+1)
	for row := range lengths {
		lengths[row] = make([]int, len(newTokens)+1)
	}
	for oldIndex := len(oldTokens) - 1; oldIndex >= 0; oldIndex-- {
		for newIndex := len(newTokens) - 1; newIndex >= 0; newIndex-- {
			if oldTokens[oldIndex].text == newTokens[newIndex].text {
				lengths[oldIndex][newIndex] = lengths[oldIndex+1][newIndex+1] + 1
			} else {
				lengths[oldIndex][newIndex] = max(lengths[oldIndex+1][newIndex], lengths[oldIndex][newIndex+1])
			}
		}
	}

	commonBytes := 0
	oldIndex, newIndex := 0, 0
	for oldIndex < len(oldTokens) && newIndex < len(newTokens) {
		switch {
		case oldTokens[oldIndex].text == newTokens[newIndex].text:
			if !oldTokens[oldIndex].space {
				commonBytes += len(oldTokens[oldIndex].text)
			}
			oldIndex++
			newIndex++
		case lengths[oldIndex+1][newIndex] >= lengths[oldIndex][newIndex+1]:
			oldSpans = appendSpan(oldSpans, oldTokens[oldIndex])
			oldIndex++
		default:
			newSpans = appendSpan(newSpans, newTokens[newIndex])
			newIndex++
		}
	}
	for ; oldIndex < len(oldTokens); oldIndex++ {
		oldSpans = appendSpan(oldSpans, oldTokens[oldIndex])
	}
	for ; newIndex < len(newTokens); newIndex++ {
		newSpans = appendSpan(newSpans, newTokens[newIndex])
	}

	// A rewritten line would light up almost entirely, which is just noise.
	if commonBytes*3 < min(nonSpaceBytes(oldTokens), nonSpaceBytes(newTokens)) {
		return nil, nil
	}
	return oldSpans, newSpans
}

type token struct {
	text  string
	start int
	space bool
}

func tokenize(text string) []token {
	var tokens []token
	start := 0
	class := func(character rune) int {
		switch {
		case unicode.IsSpace(character):
			return 0
		case unicode.IsLetter(character) || unicode.IsDigit(character) || character == '_':
			return 1
		}
		return 2
	}
	previous := -1
	for index, character := range text {
		current := class(character)
		// Punctuation never groups, so `((` differs from `(` by one token.
		if index > 0 && (current != previous || current == 2) {
			tokens = append(tokens, token{text: text[start:index], start: start, space: previous == 0})
			start = index
		}
		previous = current
	}
	if start < len(text) {
		tokens = append(tokens, token{text: text[start:], start: start, space: previous == 0})
	}
	return tokens
}

func appendSpan(spans []Span, changed token) []Span {
	end := changed.start + len(changed.text)
	if count := len(spans); count > 0 && spans[count-1].End == changed.start {
		spans[count-1].End = end
		return spans
	}
	return append(spans, Span{Start: changed.start, End: end})
}

func nonSpaceBytes(tokens []token) int {
	total := 0
	for _, each := range tokens {
		if !each.space {
			total += len(each.text)
		}
	}
	return total
}
