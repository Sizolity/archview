package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Sizolity/archview/internal/model"
	"strings"
)

// dataSentinel marks where a single-file export puts the model. It has to stay
// in ui.html, otherwise the export cannot find its slot and fails loudly.
const dataSentinel = "<!-- archview:data -->"

// HTML builds the self-contained page: the model and ui.js are inlined,
// so the file opens from file:// with no server, no module scan and no network.
func HTML(ui, js []byte, m *model.Module) ([]byte, error) {
	if bytes.Contains(js, []byte("</script")) {
		return nil, fmt.Errorf("ui.js 不能包含 </script,否则会提前结束脚本块")
	}
	if !bytes.Contains(ui, []byte(dataSentinel)) {
		return nil, fmt.Errorf("ui.html 缺少 %s 占位", dataSentinel)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return nil, fmt.Errorf("编码模型: %w", err)
	}
	// A literal "</script>" inside the JSON would close the script block early.
	// In a JSON string "\/" is the same character, so escaping it is lossless.
	data := strings.ReplaceAll(buf.String(), "</", "<\\/")

	page := strings.Replace(string(ui), dataSentinel,
		"<script>window.__ARCHVIEW_DATA__ = "+data+";</script>", 1)
	page = strings.Replace(page, `<script src="/ui.js"></script>`,
		"<script>\n"+string(js)+"\n</script>", 1)
	return []byte(page), nil
}

// StaticEditor picks the link mode for a static page: there is no /source
// endpoint behind file://, so "source" degrades to file:// and "none" stays.
func StaticEditor(mode string) string {
	if mode == "source" {
		return "file"
	}
	return mode
}
