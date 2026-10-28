// Package export writes the model out and reads a model in.
//
// Two outputs share the data: a self-contained HTML page (the model as JSON and
// the UI script inlined, so it opens from a file:// path with no server) and
// the uml-viewer topology IR in EDN. The same EDN can be read back into a
// model, which is how -import-edn renders a diagram without any source code.
package export
