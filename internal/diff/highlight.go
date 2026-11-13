package diff

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// Token is a run of text with the colour syntax highlighting gave it.
type Token struct {
	Text string
	// "#rrggbb", or empty for the default text colour.
	Color string
	Bold  bool
}

// Highlight tokenises the lines as one piece of source, so constructs that
// span lines (block comments, multi-line strings) colour correctly within
// what the diff shows. It returns one token list per input line.
func Highlight(path string, lines []string, styleName string) [][]Token {
	lexer := lexers.Match(path)
	if lexer == nil {
		return plain(lines)
	}
	iterator, err := chroma.Coalesce(lexer).Tokenise(nil, strings.Join(lines, "\n"))
	if err != nil {
		return plain(lines)
	}
	style := styles.Get(styleName)

	highlighted := make([][]Token, len(lines))
	line := 0
	for token := iterator(); token != chroma.EOF; token = iterator() {
		entry := style.Get(token.Type)
		color := ""
		if entry.Colour.IsSet() {
			color = entry.Colour.String()
		}
		for index, part := range strings.Split(token.Value, "\n") {
			if index > 0 {
				line++
			}
			if part != "" && line < len(lines) {
				highlighted[line] = append(highlighted[line], Token{Text: part, Color: color, Bold: entry.Bold == chroma.Yes})
			}
		}
	}
	return highlighted
}

func plain(lines []string) [][]Token {
	tokens := make([][]Token, len(lines))
	for index, line := range lines {
		tokens[index] = []Token{{Text: line}}
	}
	return tokens
}
