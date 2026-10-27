// Package sub is a second fixture package, so relations cross package boundaries.
package sub

// J is satisfied by relmod.S, which lives in the parent package.
type J interface{ M() }
