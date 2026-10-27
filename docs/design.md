# archview design

What the tool does, how a scan becomes a page, and where the sharp edges are.
[README.md](../README.md) has the user-facing description; this file records the
decisions behind it.

## Data flow

```
flags + archview.json
        │
        ▼
   loadWith(dir)                      single module: go list -m + go list ./...
        │                             workspace: go.work → one loadModule per module
        ▼
   parse (go/parser, AST)             packages, types, fields, methods, funcs, consts, vars
        │
        ▼
   typeCheck (go/types)               export data from `go list -export -deps`
        ├── relations                  implements / embeds edges
        └── calls                      ast.CallExpr + types.Info{Uses, Selections}
        │
        ▼
   metrics (go-crap, once)            cyclomatic / coverage / crap per function
        │
        ▼
   Module{packages, edges, calls, metrics, warnings}
        │
        ├── /model.json  ──▶  ui.js: indexModel → buildScene → draw + inspector
        └── -out page    ──▶  model + ui.js inlined into one HTML file
```

The browser never talks to `go`; it receives one JSON document and every view
change (drill-down, layout mode, focus, search) is client-side.

## Scan pieces

| Stage | File | Notes |
| --- | --- | --- |
| module discovery | `model.go` | `go list -m` for the module path and root; it is the only way to learn where the module starts, and it works on Windows and Linux |
| package list | `model.go` | `go list -e ./...` keeps broken packages so the rest of the scan still works |
| parsing | `model.go` | `go/parser` with comments; unexported declarations are kept and filtered in the UI |
| type check | `types.go` | `go list -e -export -deps` for export data, `importer.ForCompiler` + `types.Config.Check`; a failure is per-package warning, never fatal |
| relations | `types.go` | `uses` is syntactic; `implements`/`embeds` come from `go/types` (a type counts if `T` or `*T` satisfies the interface) |
| calls | `calls.go` | only static targets inside the module; interface calls are marked `dynamic` |
| metrics | `metrics.go` | one `go-crap scan -f json .` at startup, keyed to functions by `file:line` |
| workspace | `workspace.go` | `go work edit -json` lists modules; results are merged and positions re-rooted |
| cache | `cache.go` | fingerprint of every `.go` + `go.mod`/`go.sum`/`go.work` (path, size, mtime) decides whether to rescan |

## Model schema

`/model.json` is the whole contract; the browser has no other input.

| Field | Shape | Meaning |
| --- | --- | --- |
| `path` | string | module import path; empty in workspace mode |
| `root` | string | absolute scan root, used to build editor/file URLs |
| `editor` | string | resolved `-editor` mode |
| `layers` | `[][]string` | config layers, inner first |
| `metrics` | object | go-crap summary: `average`, `combined`, `exceeded`, `total_funcs`, `thresholds` |
| `packages` | array | `path`, `name`, `doc`, `imports`, `types`, `funcs`, `consts`, `vars` |
| `edges` | array | `{kind, from, to}` with `kind` ∈ `uses`, `implements`, `embeds` |
| `calls` | array | `{from, to, dynamic}` between function IDs |
| `warnings` | array | degraded stages, one line each |

IDs are stable strings: a type is `<import path>#<TypeName>`, a function is
`<import path>#<Func>`, a method is `<import path>#<Type>.<Method>`. Positions
are `<module-relative file>:<line>`, which is also the join key with go-crap and
with `/source`.

## Endpoints

| Path | Purpose |
| --- | --- |
| `GET /` | the page |
| `GET /ui.js` | pure UI helpers |
| `GET /model.json` | cached snapshot |
| `GET /source?path=<rel>.go` | read-only source view; rejects `..`, absolute paths, non-`.go`, and files over 2 MB |
| `GET /healthz` | liveness |

The server is loopback-only by default and never writes to the scanned module.

## Trade-offs

- **Standard library only.** `go/types` + `go list -export` replaces
  `golang.org/x/tools/go/packages`; the cost is reimplementing the import
  resolution `go list` already knows, the benefit is no dependency and one
  behaviour on every platform.
- **Syntactic `uses`.** Matching field and signature types by name misses
  aliases and generics; it is cheap, never wrong about the common case, and
  `go/types` covers the relations that matter for architecture (`implements`,
  `embeds`).
- **Metrics at startup, not per request.** go-crap runs the module's tests, so
  it is a once-per-process snapshot; refreshing the page does not re-measure.
- **Fingerprint cache.** A directory walk per request (milliseconds) replaces a
  full `go list` + type check (seconds). Errors are never cached, so a fixed
  module recovers on the next request.
- **Call graph at type level.** Nodes are types, so calls aggregate per type
  pair; function-level hops live in the inspector instead of on the canvas.
- **Static export is a snapshot.** The page inlines the model; it does not
  update, and it has no `/source` endpoint, so `source` degrades to `file://`.
- **Workspace mode is a merge.** Two modules with the same import path would
  collide; that case is reported by the go command rather than handled.
