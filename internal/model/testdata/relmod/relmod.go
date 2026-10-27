// Package relmod is a fixture for interface satisfaction and struct embedding.
package relmod

// I is the interface S and P satisfy.
type I interface{ M() }

// S satisfies I with a value receiver.
type S struct{}

// M implements I.M.
func (S) M() {}

// P satisfies I only through *P.
type P struct{}

// M implements I.M on *P.
func (*P) M() {}

// E embeds S.
type E struct{ S }
