package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path"
	"strings"
)

// Metrics is the per-function CRAP result reported by go-crap. Coverage is a
// percentage (0–100), not a fraction: go-crap's number, kept verbatim.
type Metrics struct {
	Cyclomatic int     `json:"cyclomatic"`
	Coverage   float64 `json:"coverage"`
	CRAP       float64 `json:"crap"`
}

// MetricsSummary mirrors go-crap's own summary block. Thresholds comes from
// the config file, not from go-crap.
type MetricsSummary struct {
	Average    float64 `json:"average"`
	Combined   float64 `json:"combined"`
	Exceeded   int     `json:"exceeded"`
	TotalFuncs int     `json:"total_funcs"`
	Thresholds []int   `json:"thresholds,omitempty"`
}

// crapReport is the subset of `go-crap scan -f json` the viewer reads. Unknown
// fields are ignored on purpose: the tool owns its schema.
type crapReport struct {
	Summary struct {
		Average    float64 `json:"average"`
		Combined   float64 `json:"combined"`
		Exceeded   int     `json:"exceeded"`
		TotalFuncs int     `json:"total_funcs"`
	} `json:"summary"`
	Entries []struct {
		File       string  `json:"file"`
		Function   string  `json:"function"`
		Package    string  `json:"package"`
		Line       int     `json:"line"`
		Cyclomatic int     `json:"cyclomatic"`
		Coverage   float64 `json:"coverage"`
		CRAP       float64 `json:"crap"`
	} `json:"entries"`
}

// metricsBundle is one go-crap run, kept for the life of the process. The
// model is rebuilt per request; the metrics snapshot is not, so a scan costs
// one `go test` at startup instead of one per page load.
type metricsBundle struct {
	ByKey      map[string]Metrics
	Summary    MetricsSummary
	Thresholds []int
	Warnings   []string
}

// collectMetrics runs go-crap in dir ("" = the process working directory).
// Failure is not fatal: it becomes a warning and the viewer runs without
// scores.
func collectMetrics(ctx context.Context, dir, command string) *metricsBundle {
	b := &metricsBundle{ByKey: map[string]Metrics{}}
	out, err := runCrap(ctx, dir, command)
	if err != nil {
		b.Warnings = append(b.Warnings, "CRAP 指标不可用: "+oneLine(err.Error()))
		return b
	}
	byKey, summary, err := parseCrapReport(out)
	if err != nil {
		b.Warnings = append(b.Warnings, "CRAP 报告无法解析: "+oneLine(err.Error()))
		return b
	}
	b.ByKey = byKey
	b.Summary = summary
	return b
}

// runCrap asks go-crap for a JSON report on stdout. Nothing is written into
// the module being inspected.
//
// The pattern must be "." — go-crap v0.5.1 fails on "./..." while discovering
// modules.
func runCrap(ctx context.Context, dir, command string) ([]byte, error) {
	if command == "" {
		command = "go-crap"
	}
	cmd := exec.CommandContext(ctx, command, "scan", "-f", "json", "--no-progress", ".")
	if dir != "" {
		cmd.Dir = dir
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s scan: %w: %s", command, err, oneLine(stderr.String()))
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, fmt.Errorf("%s scan: 空输出", command)
	}
	return out, nil
}

// parseCrapReport indexes entries by "file:line", the same key the model uses
// for FuncInfo.Pos and MethodInfo.Pos.
func parseCrapReport(data []byte) (map[string]Metrics, MetricsSummary, error) {
	var rep crapReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, MetricsSummary{}, fmt.Errorf("解析 JSON: %w", err)
	}
	byKey := make(map[string]Metrics, len(rep.Entries))
	for _, e := range rep.Entries {
		byKey[metricKey(e.File, e.Line)] = Metrics{
			Cyclomatic: e.Cyclomatic,
			Coverage:   e.Coverage,
			CRAP:       e.CRAP,
		}
	}
	return byKey, MetricsSummary{
		Average:    rep.Summary.Average,
		Combined:   rep.Summary.Combined,
		Exceeded:   rep.Summary.Exceeded,
		TotalFuncs: rep.Summary.TotalFuncs,
	}, nil
}

// metricKey normalises the file part so a Windows path, a slash path and a
// "./" prefix all match the model's relative position.
func metricKey(file string, line int) string {
	return fmt.Sprintf("%s:%d", normalizeFile(file), line)
}

// normalizeFile puts a reported path into the model's slash form. The replace
// is explicit rather than filepath.ToSlash: go-crap writes backslashes on
// Windows, and that data must normalise the same way when the scan runs on
// Linux (CI does exactly that).
func normalizeFile(f string) string {
	f = strings.ReplaceAll(f, "\\", "/")
	f = strings.TrimPrefix(f, "./")
	return path.Clean(f)
}

// apply attaches metrics to the model in place and reports how many
// declarations matched. Declarations go-crap did not report keep no metrics.
func (b *metricsBundle) apply(m *Module) int {
	if b == nil {
		return 0
	}
	m.Warnings = append(m.Warnings, b.Warnings...)
	if len(b.ByKey) == 0 {
		return 0
	}

	matched := 0
	lookup := func(pos string) *Metrics {
		norm := normalizePos(pos)
		if norm == "" {
			return nil
		}
		mt, ok := b.ByKey[norm]
		if !ok {
			return nil
		}
		matched++
		cp := mt
		return &cp
	}

	for _, p := range m.Packages {
		for _, t := range p.Types {
			for i := range t.Methods {
				t.Methods[i].Metrics = lookup(t.Methods[i].Pos)
			}
		}
		for _, f := range p.Funcs {
			f.Metrics = lookup(f.Pos)
		}
	}
	if matched > 0 {
		summary := b.Summary
		summary.Thresholds = b.Thresholds
		m.Metrics = &summary
	}
	return matched
}

// normalizePos rewrites a model position ("dir/file.go:12") into the key form.
func normalizePos(pos string) string {
	i := strings.LastIndex(pos, ":")
	if i < 0 {
		return ""
	}
	file, line := pos[:i], pos[i+1:]
	line = strings.TrimSpace(line)
	if file == "" || line == "" {
		return ""
	}
	return normalizeFile(file) + ":" + line
}
