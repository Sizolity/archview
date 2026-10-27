// Package b is the second workspace module.
package b

import "example.com/moda"

// B is a struct in module b.
type B struct{}

// FB calls into module a, so the call graph crosses the module boundary.
func FB() { a.FA() }
