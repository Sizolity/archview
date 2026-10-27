// Package calls is a fixture for the call graph.
package calls

// A calls B.
func A() { B() }

// B calls C.
func B() { C() }

// C is the leaf.
func C() {}

// I is satisfied by S.
type I interface{ M() }

// S implements I.
type S struct{}

// M implements I.M.
func (S) M() {}

// Dynamic calls through the interface: the receiver is only known at run time.
func Dynamic(i I) { i.M() }

// T carries a method that calls a function.
type T struct{}

// Do calls A.
func (T) Do() { A() }
