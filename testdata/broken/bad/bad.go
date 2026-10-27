// Package bad imports a package that does not exist, so type checking fails.
package bad

import "example.com/broken/missing"

// V depends on the missing package.
var V = missing.Value

// T must still reach the model: only the relations degrade, not the parse.
type T struct{}
