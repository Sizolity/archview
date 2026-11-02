// Package severity is the typed-constant fixture from the bug report: the
// constant Error names its own type and the type's method reads Error back.
// Neither relation is an architectural dependency, so the two cards must not
// be condensed into a cycle.
package severity

// Severity is a level.
type Severity int

const (
	Error Severity = iota
	Warn
	Info
)

// String returns the name of s.
func (s Severity) String() string {
	if s == Error {
		return "error"
	}
	return ""
}
