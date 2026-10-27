package model

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
