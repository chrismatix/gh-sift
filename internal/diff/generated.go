package diff

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GeneratedByAttributes evaluates a repository's root .gitattributes the way
// GitHub does to collapse files: linguist-generated set or true means generated,
// unset or false means not, and undeclared paths are left out of the map.
func GeneratedByAttributes(attributes string, paths []string) (map[string]bool, error) {
	declared := map[string]bool{}
	if !strings.Contains(attributes, "linguist-generated") || len(paths) == 0 {
		return declared, nil
	}
	// git does the pattern matching, in a scratch repository holding only that file.
	scratch, err := os.MkdirTemp("", "gh-sift-attributes-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scratch)
	if err := exec.Command("git", "init", "--quiet", scratch).Run(); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(scratch, ".gitattributes"), []byte(attributes), 0o644); err != nil {
		return nil, err
	}
	command := exec.Command("git", "check-attr", "-z", "--stdin", "linguist-generated")
	command.Dir = scratch
	command.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	output, err := command.Output()
	if err != nil {
		return nil, err
	}
	fields := bytes.Split(output, []byte{0})
	for index := 0; index+2 < len(fields); index += 3 {
		path, value := string(fields[index]), string(fields[index+2])
		if value != "unspecified" {
			declared[path] = value != "unset" && value != "false"
		}
	}
	return declared, nil
}
