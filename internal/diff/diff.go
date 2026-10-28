// Package diff maps a git diff onto the model's declarations, so a view can
// talk about the code that changed instead of the whole module.
package diff

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/Sizolity/archview/internal/model"
)

// Range is an inclusive line span in the new version of a file.
type Range struct{ Start, End int }

// Changes is one git diff, reduced to the lines that appeared or moved.
type Changes struct {
	Ref   string
	Files map[string][]Range
}

var hunkRe = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// Parse reads `git diff --unified=0` output. Paths are taken from the +++
// header, so a deleted file (+++ /dev/null) contributes nothing.
//
// The parser tracks hunk state instead of pattern-matching every line: a source
// line that happens to start with "+++" or "@@" is content, not a header, and
// git quotes paths that contain spaces or non-ASCII characters.
func Parse(text string) map[string][]Range {
	out := map[string][]Range{}
	file := ""
	inHunk := false
	pendingHeader := false

	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			inHunk, pendingHeader = false, false
		case !inHunk && strings.HasPrefix(line, "--- "):
			pendingHeader = true
		case !inHunk && pendingHeader && strings.HasPrefix(line, "+++ "):
			pendingHeader = false
			name := strings.TrimPrefix(line, "+++ ")
			if name == "/dev/null" {
				file = ""
				continue
			}
			file = strings.TrimPrefix(unquote(name), "b/")
			if _, ok := out[file]; !ok {
				out[file] = nil
			}
		case strings.HasPrefix(line, "@@"):
			// A hunk header can carry a section heading, so match the numbers.
			m := hunkRe.FindStringSubmatch(line)
			if m == nil || file == "" {
				continue
			}
			inHunk = true
			start, _ := strconv.Atoi(m[1])
			count := 1
			if m[2] != "" {
				count, _ = strconv.Atoi(m[2])
			}
			if count == 0 {
				// A pure deletion: the lines are gone, mark where they were.
				count = 1
			}
			out[file] = append(out[file], Range{Start: start, End: start + count - 1})
		}
	}
	return out
}

// Run asks git what changed under root since ref. The comparison includes
// staged and unstaged changes, and paths are made relative to root so they line
// up with the model's positions even inside a monorepo.
func Run(ctx context.Context, root, ref string) (*Changes, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "diff", "--relative",
		"--unified=0", "--no-color", "--no-ext-diff", ref, "--", ".")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("git diff %s: %s", ref, model.OneLine(detail))
	}
	return &Changes{Ref: ref, Files: Parse(string(out))}, nil
}

// Apply marks every declaration the diff touches and records the summary on the
// module. It reports whether anything matched.
func Apply(m *model.Module, c *Changes) bool {
	if c == nil || len(c.Files) == 0 {
		return false
	}
	var funcs, types int
	seen := map[string]bool{}

	for _, p := range m.Packages {
		for _, t := range p.Types {
			if hits(c.Files, t.Pos, t.End) {
				t.Changed = true
				types++
				seen[fileOf(t.Pos)] = true
			}
			for i := range t.Methods {
				if hits(c.Files, t.Methods[i].Pos, t.Methods[i].End) {
					t.Methods[i].Changed = true
					funcs++
					seen[fileOf(t.Methods[i].Pos)] = true
				}
			}
		}
		for _, f := range p.Funcs {
			if hits(c.Files, f.Pos, f.End) {
				f.Changed = true
				funcs++
				seen[fileOf(f.Pos)] = true
			}
		}
	}
	if funcs+types == 0 {
		// The diff touched no declaration: no summary, so the page does not show
		// "0 files / 0 functions" as if the run had broken.
		return false
	}
	m.Diff = &model.DiffSummary{Ref: c.Ref, Files: len(seen), Functions: funcs, Types: types}
	return true
}

// hits reports whether a declaration's span intersects any changed range in its
// file.
func hits(files map[string][]Range, pos string, end int) bool {
	ranges, ok := files[fileOf(pos)]
	if !ok {
		return false
	}
	if end == 0 {
		end, _ = lineOf(pos)
	}
	start, _ := lineOf(pos)
	for _, r := range ranges {
		if r.Start <= end && r.End >= start {
			return true
		}
	}
	return false
}

// fileOf splits "dir/file.go:12" into "dir/file.go".
func fileOf(pos string) string {
	i := strings.LastIndex(pos, ":")
	if i < 0 {
		return pos
	}
	return pos[:i]
}

func lineOf(pos string) (int, bool) {
	i := strings.LastIndex(pos, ":")
	if i < 0 {
		return 0, false
	}
	n, err := strconv.Atoi(pos[i+1:])
	return n, err == nil
}

// unquote undoes git's C-style quoting for paths with unusual characters.
func unquote(s string) string {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	if u, err := strconv.Unquote(s); err == nil {
		return u
	}
	return s
}
