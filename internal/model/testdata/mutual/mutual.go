// Package mutual is a fixture for two types that reference each other. Unlike
// the typed-constant fixture, this is a real structural cycle.
package mutual

// A refers to B.
type A struct{ b *B }

// B refers to A.
type B struct{ a *A }
