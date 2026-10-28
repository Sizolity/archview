package export

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Sizolity/archview/internal/assets"
	"github.com/Sizolity/archview/internal/model"
)

func sampleModule() *model.Module {
	return &model.Module{
		Path:   "example.com/mod",
		Root:   "E:/work/mod",
		Editor: "vscode",
		Packages: []*model.Package{{
			Path: "example.com/mod",
			Name: "mod",
			Types: []*model.TypeInfo{{
				ID:       "example.com/mod#A",
				Name:     "A",
				Kind:     "struct",
				Exported: true,
				Pos:      "a.go:3",
			}},
		}},
	}
}

// extractData pulls the inlined model back out of an exported page.
func extractData(t *testing.T, page []byte) *model.Module {
	t.Helper()
	const marker = "window.__ARCHVIEW_DATA__ = "
	s := string(page)
	start := strings.Index(s, marker)
	if start < 0 {
		t.Fatal("exported page has no data block")
	}
	start += len(marker)
	end := strings.Index(s[start:], ";</script>")
	if end < 0 {
		t.Fatal("data block is not terminated")
	}
	var m model.Module
	if err := json.Unmarshal([]byte(s[start:start+end]), &m); err != nil {
		t.Fatalf("inlined model does not parse: %v", err)
	}
	return &m
}

func TestExportHTMLIsSelfContained(t *testing.T) {
	page, err := HTML(assets.UI, assets.JS, sampleModule())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), `src="/ui.js"`) {
		t.Fatal("exported page still loads ui.js over HTTP")
	}
	if !strings.Contains(string(page), "window.ArchView") {
		t.Fatal("exported page does not inline ui.js")
	}
	if !strings.HasSuffix(strings.TrimSpace(string(page)), "</html>") {
		t.Fatal("exported page is truncated")
	}

	got := extractData(t, page)
	if got.Path != "example.com/mod" || got.Root != "E:/work/mod" || got.Editor != "vscode" {
		t.Fatalf("model did not survive the export: %+v", got)
	}
	if len(got.Packages) != 1 || len(got.Packages[0].Types) != 1 {
		t.Fatalf("packages did not survive the export: %+v", got.Packages)
	}
	if string(page) == "" {
		t.Fatal("empty export")
	}
}

func TestExportEscapesScriptClose(t *testing.T) {
	m := sampleModule()
	m.Packages[0].Doc = `never write </script> in docs`
	page, err := HTML(assets.UI, assets.JS, m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), "</script> in docs") {
		t.Fatal(`doc containing </script> was inlined verbatim and closed the data block early`)
	}
	if !strings.Contains(string(page), `<\/script> in docs`) {
		t.Fatal("doc containing </script> was not escaped in the data block")
	}
	if got := extractData(t, page); got.Packages[0].Doc != m.Packages[0].Doc {
		t.Fatalf("doc round-trip = %q, want %q", got.Packages[0].Doc, m.Packages[0].Doc)
	}
}

func TestExportHTMLNeedsSentinel(t *testing.T) {
	_, err := HTML([]byte("<html></html>"), assets.JS, sampleModule())
	if err == nil || !strings.Contains(err.Error(), dataSentinel) {
		t.Fatalf("err = %v, want a missing-sentinel error", err)
	}
}

func TestExportEditorDegradesSource(t *testing.T) {
	// There is no /source endpoint behind file://.
	if got := StaticEditor("source"); got != "file" {
		t.Errorf("StaticEditor(source) = %q, want file", got)
	}
	for _, mode := range []string{"vscode", "goland", "file", "none"} {
		if got := StaticEditor(mode); got != mode {
			t.Errorf("StaticEditor(%q) = %q, want %q", mode, got, mode)
		}
	}
}
