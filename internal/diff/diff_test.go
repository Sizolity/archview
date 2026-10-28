package diff

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Sizolity/archview/internal/model"
)

// A modified hunk, a new file and a deletion: the parser keeps the new-side
// ranges and ignores the deleted file.
func TestParse(t *testing.T) {
	const patch = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -10,0 +11,3 @@ func A() {
+	x := 1
+	y := 2
+	_ = x + y
@@ -30,2 +33,0 @@ func Gone() {
-	old
-	old
diff --git a/new.go b/new.go
--- /dev/null
+++ b/new.go
@@ -0,0 +1,42 @@
+package b
diff --git a/gone.go b/gone.go
--- a/gone.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package c
`
	got := Parse(patch)
	if _, ok := got["gone.go"]; ok {
		t.Error("a deleted file must not contribute ranges")
	}
	a := got["a.go"]
	want := []Range{{11, 13}, {33, 33}}
	if len(a) != len(want) {
		t.Fatalf("a.go ranges = %v, want %v", a, want)
	}
	for i := range want {
		if a[i] != want[i] {
			t.Fatalf("a.go ranges = %v, want %v", a, want)
		}
	}
	if n := got["new.go"]; len(n) != 1 || n[0] != (Range{1, 42}) {
		t.Fatalf("new.go ranges = %v", n)
	}
}

// A range inside a function body marks the function; one in a comment between
// declarations marks only the declaration it falls in.
func TestApply(t *testing.T) {
	m := &model.Module{Packages: []*model.Package{{
		Path: "example.com/m/a",
		Funcs: []*model.FuncInfo{
			{ID: "a#A", Name: "A", Pos: "a/a.go:5", End: 9},
			{ID: "a#B", Name: "B", Pos: "a/a.go:12", End: 14},
		},
		Types: []*model.TypeInfo{{
			ID: "a#T", Name: "T", Pos: "a/a.go:20", End: 24,
			Methods: []model.MethodInfo{{ID: "a#T.M", Name: "M", Pos: "a/a.go:27", End: 30}},
		}},
	}}}

	c := &Changes{Ref: "HEAD", Files: map[string][]Range{
		"a/a.go": {{7, 7}, {28, 29}},
	}}
	if !Apply(m, c) {
		t.Fatal("Apply found nothing")
	}
	if !m.Packages[0].Funcs[0].Changed {
		t.Error("A should be marked: the change is inside its body")
	}
	if m.Packages[0].Funcs[1].Changed {
		t.Error("B should not be marked")
	}
	if m.Packages[0].Types[0].Changed {
		t.Error("T's own span is untouched by the diff")
	}
	if !m.Packages[0].Types[0].Methods[0].Changed {
		t.Error("M should be marked")
	}
	if got := m.Diff; got == nil || got.Functions != 2 || got.Types != 0 || got.Files != 1 {
		t.Fatalf("summary = %+v", got)
	}
}

func TestApplyWithoutChanges(t *testing.T) {
	m := &model.Module{}
	if Apply(m, &Changes{}) {
		t.Fatal("empty diff must not report changes")
	}
	if Apply(m, nil) {
		t.Fatal("nil diff must not report changes")
	}
	if m.Diff != nil {
		t.Fatalf("summary = %+v, want nil", m.Diff)
	}
}

// Run shells out to git; it is skipped where git or a repository is missing.
func TestRunAgainstRealRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package a\n\nfunc A() {\n}\n")
	git("add", ".")
	git("commit", "-q", "-m", "one")
	write("a.go", "package a\n\nfunc A() {\n\t_ = 1\n}\n")

	c, err := Run(context.Background(), dir, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Files) != 1 || len(c.Files["a.go"]) == 0 {
		t.Fatalf("changes = %+v", c.Files)
	}
	if r := c.Files["a.go"][0]; r.Start != 4 || r.End != 4 {
		t.Fatalf("range = %+v, want the inserted line 4", r)
	}
}

// A source line that begins with "+++" or "@@" is content, not a header, and a
// quoted path keeps its slash after unquoting.
func TestParseIsNotFooledByContent(t *testing.T) {
	const patch = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -5,0 +6,2 @@
+++ b/not-a-header
+@@ -1,1 +1,1 @@
diff --git "b/dir with space/x.go" "b/dir with space/x.go"
--- "b/dir with space/x.go"
+++ "b/dir with space/x.go"
@@ -2,0 +3,1 @@
+added
`
	got := Parse(patch)
	if _, ok := got["not-a-header"]; ok {
		t.Error("a content line starting with +++ became a file header")
	}
	files := got["a.go"]
	if len(files) != 1 || files[0] != (Range{6, 7}) {
		t.Fatalf("a.go ranges = %v, want [{6 7}]", files)
	}
	quoted := got["dir with space/x.go"]
	if len(quoted) != 1 || quoted[0] != (Range{3, 3}) {
		t.Fatalf("quoted path ranges = %v (keys %v)", quoted, keys(got))
	}
}

func keys(m map[string][]Range) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
