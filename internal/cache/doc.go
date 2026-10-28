// Package cache keeps the scanned model between requests and notices when the
// sources change.
//
// A fingerprint covers every .go file plus go.mod/go.sum/go.work (path, size
// and modification time); a Load that sees the same fingerprint hands back the
// model it already built. The same fingerprint drives -watch: when it moves,
// every subscriber gets one reload event, which the page turns into an SSE
// message.
package cache
