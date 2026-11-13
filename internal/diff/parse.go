// Package diff parses unified diffs and prepares them for display.
package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
)

type LineKind byte

const (
	Context LineKind = ' '
	Added   LineKind = '+'
	Deleted LineKind = '-'
)

type Line struct {
	Kind LineKind
	// 0 on the side the line does not exist on.
	OldNumber int
	NewNumber int
	Text      string
}

type Hunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	// The function or heading git prints after the second @@.
	Section string
	Lines   []Line
	// Filled by Parse: the hash is asked for on every drawn row.
	hash string
}

type Status string

const (
	Modified    Status = "M"
	AddedFile   Status = "A"
	DeletedFile Status = "D"
	Renamed     Status = "R"
)

type File struct {
	OldPath   string
	NewPath   string
	Status    Status
	Binary    bool
	Hunks     []Hunk
	Additions int
	Deletions int
}

// Path is the name GitHub knows the file by: the new one unless it was deleted.
func (file File) Path() string {
	if file.Status == DeletedFile {
		return file.OldPath
	}
	return file.NewPath
}

// Hash identifies a hunk by its changed lines only, so a hunk that merely
// moves after a rebase or a push elsewhere in the file keeps its identity.
func (hunk Hunk) Hash() string {
	if hunk.hash != "" {
		return hunk.hash
	}
	digest := sha256.New()
	for _, line := range hunk.Lines {
		if line.Kind != Context {
			digest.Write([]byte{byte(line.Kind)})
			digest.Write([]byte(line.Text))
			digest.Write([]byte{'\n'})
		}
	}
	return hex.EncodeToString(digest.Sum(nil))[:16]
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@ ?(.*)$`)

func Parse(unified string) []File {
	var files []File
	var file *File
	var hunk *Hunk
	oldNumber, newNumber := 0, 0

	for _, raw := range strings.Split(unified, "\n") {
		raw = strings.TrimSuffix(raw, "\r")
		if strings.HasPrefix(raw, "diff --git ") {
			files = append(files, File{Status: Modified})
			file = &files[len(files)-1]
			file.OldPath, file.NewPath = pathsFromGitHeader(raw)
			hunk = nil
			continue
		}
		if file == nil {
			continue
		}
		if hunk == nil {
			switch {
			case strings.HasPrefix(raw, "new file mode"):
				file.Status = AddedFile
			case strings.HasPrefix(raw, "deleted file mode"):
				file.Status = DeletedFile
			case strings.HasPrefix(raw, "rename from "):
				file.Status = Renamed
				file.OldPath = unquote(strings.TrimPrefix(raw, "rename from "))
			case strings.HasPrefix(raw, "rename to "):
				file.NewPath = unquote(strings.TrimPrefix(raw, "rename to "))
			case strings.HasPrefix(raw, "Binary files ") || strings.HasPrefix(raw, "GIT binary patch"):
				file.Binary = true
			case strings.HasPrefix(raw, "--- a/"):
				file.OldPath = unquote(strings.TrimPrefix(raw, "--- a/"))
			case strings.HasPrefix(raw, "+++ b/"):
				file.NewPath = unquote(strings.TrimPrefix(raw, "+++ b/"))
			}
		}
		if match := hunkHeader.FindStringSubmatch(raw); match != nil {
			file.Hunks = append(file.Hunks, Hunk{
				OldStart: atoi(match[1]), OldLines: countOrOne(match[2]),
				NewStart: atoi(match[3]), NewLines: countOrOne(match[4]),
				Section: match[5],
			})
			hunk = &file.Hunks[len(file.Hunks)-1]
			oldNumber, newNumber = hunk.OldStart, hunk.NewStart
			continue
		}
		if hunk == nil || raw == "" && oldNumber >= hunk.OldStart+hunk.OldLines && newNumber >= hunk.NewStart+hunk.NewLines {
			continue
		}
		switch {
		case strings.HasPrefix(raw, "+"):
			hunk.Lines = append(hunk.Lines, Line{Kind: Added, NewNumber: newNumber, Text: raw[1:]})
			newNumber++
			file.Additions++
		case strings.HasPrefix(raw, "-"):
			hunk.Lines = append(hunk.Lines, Line{Kind: Deleted, OldNumber: oldNumber, Text: raw[1:]})
			oldNumber++
			file.Deletions++
		case strings.HasPrefix(raw, `\`):
			// "\ No newline at end of file" describes the line above; nothing to show.
		default:
			// Some tools strip the single space of an empty context line.
			hunk.Lines = append(hunk.Lines, Line{Kind: Context, OldNumber: oldNumber, NewNumber: newNumber, Text: strings.TrimPrefix(raw, " ")})
			oldNumber++
			newNumber++
		}
	}
	for fileIndex := range files {
		for hunkIndex := range files[fileIndex].Hunks {
			hunk := &files[fileIndex].Hunks[hunkIndex]
			hunk.hash = hunk.Hash()
		}
	}
	return files
}

func pathsFromGitHeader(header string) (string, string) {
	rest := strings.TrimPrefix(header, "diff --git ")
	if strings.HasPrefix(rest, `"`) {
		// Quoted paths: `"a/x y" "b/x y"`.
		parts := strings.SplitN(rest, `" "`, 2)
		if len(parts) == 2 {
			return strings.TrimPrefix(unquote(parts[0]+`"`), "a/"), strings.TrimPrefix(unquote(`"`+parts[1]), "b/")
		}
	}
	// Unquoted paths have no way to mark the boundary, so assume the usual
	// case where both sides name the same file.
	half := (len(rest) - 1) / 2
	if len(rest)%2 == 1 && rest[half] == ' ' && strings.TrimPrefix(rest[:half], "a/") == strings.TrimPrefix(rest[half+1:], "b/") {
		return strings.TrimPrefix(rest[:half], "a/"), strings.TrimPrefix(rest[half+1:], "b/")
	}
	if index := strings.Index(rest, " b/"); index >= 0 {
		return strings.TrimPrefix(rest[:index], "a/"), rest[index+3:]
	}
	return rest, rest
}

func unquote(path string) string {
	if strings.HasPrefix(path, `"`) {
		if unquoted, err := strconv.Unquote(path); err == nil {
			return unquoted
		}
	}
	return path
}

func atoi(text string) int {
	number, _ := strconv.Atoi(text)
	return number
}

func countOrOne(text string) int {
	if text == "" {
		return 1
	}
	return atoi(text)
}

// Pair is one row of the side-by-side layout; a nil side is blank.
type Pair struct {
	Left  *Line
	Right *Line
}

// Pairs lines a hunk up for the split layout: each run of deletions sits
// beside the run of additions that replaced it.
func (hunk Hunk) Pairs() []Pair {
	var pairs []Pair
	lines := hunk.Lines
	for index := 0; index < len(lines); {
		if lines[index].Kind == Context {
			pairs = append(pairs, Pair{Left: &lines[index], Right: &lines[index]})
			index++
			continue
		}
		deletedStart := index
		for index < len(lines) && lines[index].Kind == Deleted {
			index++
		}
		addedStart := index
		for index < len(lines) && lines[index].Kind == Added {
			index++
		}
		deleted, added := lines[deletedStart:addedStart], lines[addedStart:index]
		for row := 0; row < max(len(deleted), len(added)); row++ {
			pair := Pair{}
			if row < len(deleted) {
				pair.Left = &deleted[row]
			}
			if row < len(added) {
				pair.Right = &added[row]
			}
			pairs = append(pairs, pair)
		}
	}
	return pairs
}
