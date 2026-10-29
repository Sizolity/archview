// Package values is a fixture for references from function bodies, signatures,
// type declarations and value initializers. The names are deliberately
// unexported: the model must keep them, the UI's export filter hides them.
package values

const k = 3

var v = k

type T struct{ n int }

func f(x T) int { return k + v + x.n }

func g() T { return T{} }

func (t T) m() int { return k }
