[![ci](https://github.com/Sizolity/archview/actions/workflows/ci.yml/badge.svg)](https://github.com/Sizolity/archview/actions/workflows/ci.yml)

# archview

A read-only, browser-based view of a Go module's architecture: packages,
types, fields, methods, constants, variables and the references between them.
It is a development tool, standard library only, and works against any Go
module.

## Run

Install it into your Go toolchain, then run it from the module you want to
inspect:

```sh
go install github.com/Sizolity/archview/cmd/archview@latest

cd /path/to/your/project
archview                     # http://127.0.0.1:8788
```

Or build from a checkout:

```sh
go build -o archview ./cmd/archview
```

Or run it from source:

```sh
go run ./cmd/archview -addr 127.0.0.1:8788
```

Scan another directory, or a `go.work` workspace:

```sh
archview -dir ../other-module
archview -dir /path/to/workspace     # go.work is detected
```

`-addr` defaults to `127.0.0.1:8788`, so the viewer is loopback-only unless you
explicitly change it.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-addr` | `127.0.0.1:8788` | listen address |
| `-editor` | `source` | where "查看源码" jumps: `source`, `vscode`, `goland`, `file`, `none` |
| `-out` | | write a self-contained page to this file and exit |
| `-edn` | | write the uml-viewer topology IR to this file and exit |
| `-import-edn` | | render an IR instead of scanning code |
| `-crap-cmd` | `go-crap` | command that produces the CRAP report |
| `-crap-profile` | | reuse a coverage profile instead of running the tests |
| `-diff` | | mark what the working tree changed since this git ref |
| `-no-crap-cache` | `false` | do not reuse the cached CRAP report |
| `-cache-dir` | user cache dir | where the CRAP report is cached |
| `-no-crap` | `false` | skip CRAP metrics entirely |
| `-config` | `archview.json` | config file to read |
| `-dir` | `.` | directory to scan; a `go.work` in it means every module is scanned |
| `-version` | | print the build's version and exit |
| `-watch` | `false` | poll for source changes and push a reload to the page over SSE |

`-dir` points the scan somewhere else. If that directory holds a `go.work`, every
module it lists is scanned and merged into one model: package paths stay unique
(they are import paths), source links are re-rooted under the workspace, calls
crossing modules are kept, and CRAP scores still line up because go-crap reports
workspace-relative paths too.

### Configuration

`archview.json` in the working directory is picked up automatically; `-config`
names another file, and a missing explicit file is an error. Flags win over the
file. An invalid file is refused whole — every problem is reported at once,
exit status 2 — so a half-applied config never renders.

| Key | Type | Meaning |
| --- | --- | --- |
| `exclude` | `[]string` | module-relative package prefixes to leave out, matching whole segments: `internal/gen` hides `internal/gen` and `internal/gen/x`, not `internal/genx` |
| `editor` | `string` | default jump mode, same values as `-editor` |
| `layers` | `[][]string` | architectural layers, inner first; a package in a lower layer depending on one in a higher layer is drawn as a red edge |
| `metrics.enabled` | `bool` | default for the CRAP step; an explicit `-no-crap` still wins |
| `metrics.thresholds` | `[warn, bad]` | colour bands, default `[5, 30]` |

```json
{
  "exclude": ["internal/gen"],
  "editor": "vscode",
  "layers": [["cmd"], ["web"], ["wiki", "query"], ["platform"]],
  "metrics": {"enabled": true, "thresholds": [5, 30]}
}
```

Unknown keys are rejected: a typo is a mistake, not a forward-compatible
extension.

CRAP scores come from [go-crap](https://github.com/padiazg/go-crap), an external
command, not a Go dependency: it runs `go-crap scan -f json .` once per scan, so
the first page load waits for a `go test` run **and the module's test suite
executes**. Use `-no-crap` if that is not what you want. Without go-crap on
`PATH` the viewer still starts and simply shows no scores (`warnings` records
why).

CRAP needs coverage, so go-crap gets it by running the tests. Two ways out:

```sh
archview -crap-profile coverage.out   # reuse a profile, run no tests
archview -no-crap                     # no scores at all
```

The report is also cached in the user's cache directory
(`%LocalAppData%\archview` on Windows, `~/Library/Caches/archview` on macOS,
`$XDG_CACHE_HOME/archview` on Linux) and keyed by the source fingerprint, the
go-crap binary and the flags — a restart on an unchanged tree reuses it
(measured: 20 s cold, 54 ms warm). It is deliberately **not** written into the
module: archview reads projects and does not write into them, so there is no
`.gitignore` entry to add. Relocate it with `-cache-dir` or
`ARCHVIEW_CACHE_DIR`, and turn it off with `-no-crap-cache`.

### Diff-aware view

```sh
archview -diff HEAD~1        # what changed since the previous commit
archview -diff origin/main   # what the branch adds
```

The working tree (staged and unstaged) is compared with the ref, the changed
hunks are mapped onto the declarations they fall in, and the model carries a
`changed` flag per function, method and type plus a `diff` summary. The page
draws those cards with an orange edge and a dot, badges the inspector, and puts
"改动 N 文件 / M 函数" in the status line.

Read together with the scores this answers the review question: the cards that
changed are the ones carrying the new CRAP numbers. If git is missing or the
directory is not a repository, the loader adds a warning and everything else
still works.
### Single-file export

```sh
archview -out view.html
```

`view.html` has the model and the UI inlined, so it opens from `file://` and
works with no server and no network. Nothing behind it can serve `/source`, so
`-editor source` degrades to `file://` links there, and `-editor none` hides the
link entirely. `-editor vscode` / `goland` keep working because the editor
protocol opens the file directly.

```sh
archview -out view.html -editor vscode
```

### EDN export

```sh
archview -edn view.edn
```

`view.edn` is the topology IR [uml-viewer](https://github.com/unclebob/uml-viewer)
reads: `:packages` of `:classes` with `:fields` and `:ops`, plus `:edges`.
Relations map to the IR's edge kinds — `uses` → `:association`, `implements` →
`:implements`, `embeds` → `:aggregation` — and class ids are stable slugs
(`wiki.Page` → `:wiki-page`). When CRAP was collected, each class also carries
`:crap` (the mean of its methods) and `:coverage` as a 0–1 ratio.

The other direction reads an IR back:

```sh
archview -import-edn view.edn          # serve it
archview -import-edn view.edn -out page.html
```

Packages become `Package`s, `:interface` classes become interfaces, `:fields`
and `:ops` become members, and `:edges` map back to `uses`/`implements`/
`embeds`. Unknown kinds and dangling endpoints are counted and reported, not
fatal. An imported model has no source directory, so `/source` returns 404 and
`-watch` has nothing to poll.

## What it shows

- **Module diagram**: one box per package, arrows for imports. Click a box to
  drill in. With `layers` configured, an import that points from an inner layer
  to an outer one is drawn red, and the package inspector lists it as a
  violation.
- **Package view**: one card per type; fields and methods listed inside the
  card, `+` exported and `-` unexported.
- **Inspector**: package docs, type docs, fields, methods, outgoing references
  and "used by", plus a link to the exact source line.
- **Source view**: the file as a framed code block with Go highlighting and a
  line-number gutter; `#L35` anchors jump to the declaration. `-editor` sends
  the link to VS Code, GoLand, `file://` or nowhere instead.
- **Search** across type, method, function, constant and variable names, plus
  docs and signatures; hits are highlighted, `↑`/`↓`/`Enter` picks one, and
  the canvas fades everything that does not match. An **include unexported**
  toggle, pan/zoom and a fit-to-window button ride along.
- **Focus, fold, declutter**: focus a type to keep only one hop around it
  (Esc leaves), fold a card down to its title with the ▾ handle, or tick
  "精简" to hide member rows and the edges you did not select.
- **Two type layouts**: the default puts users above the types they depend on,
  one band per dependency depth; the dropdown switches to grouping by kind
  (struct, interface, named type).
- **Call graph**: tick "调用边" to draw calls aggregated to the type level
  (solid when static, dashed when they go through an interface). The type
  inspector always shows one hop of calls in and out, with a 动态 badge on
  interface calls.

## HTTP surface

| Path | Purpose |
| --- | --- |
| `/` | the explorer UI |
| `/ui.js` | the pure UI helpers (routing, layout, scene building) |
| `/model.json` | the scanned model |
| `/source?path=<relative>.go` | read-only source view |
| `/healthz` | liveness probe |
| `/events` | server-sent reload events (only with `-watch`) |

## Model

`/model.json` is the whole snapshot. Two fields go beyond the declarations:

- `edges` — a sorted list of `{kind, from, to}`, where both ends are type IDs
  (`<import path>#<TypeName>`) and `kind` is one of:

  | kind | meaning | how it is found |
  | --- | --- | --- |
  | `uses` | a field or signature mentions the type | AST name matching |
  | `implements` | the type satisfies the interface | `go/types`, so promoted methods and pointer receivers count |
  | `embeds` | the struct embeds the type | `go/types`, struct field with no name |

- `warnings` — type checking degrades per package, never fatally. A package
  that cannot be checked keeps its syntactic `uses` relations, and the reason
  lands here instead of failing the scan. `/model.json` still returns 200.

- `metrics` — the module summary from go-crap (`average`, `combined`,
  `exceeded`, `total_funcs`). Functions and methods carry their own `metrics`:
  `cyclomatic`, `coverage` (a 0–100 percentage, go-crap's unit) and `crap`.
  Collected per scan and cached between runs; `-no-crap` omits the field entirely.

- `calls` — the module's call graph, `{from, to, dynamic}`, where both ends are
  function IDs (`<import path>#<Func>` or `<import path>#<Type>.<Method>`) and
  `dynamic` marks a call through an interface. Calls into the standard library
  or a dependency are not in the model: there is no node to point at.

The UI colours a type by the worst CRAP score among its methods: ≤5 green,
≤30 amber, above 30 red — go-crap's default threshold is 30.

Edges come from `go list -export` plus `go/types` (`internal/model/types.go`); the browser
draws `uses` today and the `implements`/`embeds` edges are waiting for the
layered layout in the roadmap.

## Limits

- Package and type structure, plus a module-internal call graph. The graph is
  aggregated to the type level; function-level hops live in the inspector.
- Calls through function values, fields and closures have no static target and
  are not in the call graph.
- `uses` is syntactic: a type is matched by name in the declaring package or
  by its package qualifier.
- `implements` accepts `T` or `*T`, and generic instantiations are not
  modelled yet, so generic types get no edges.
- `/model.json` is cached: the server fingerprints the module (path, size and
  mtime of every `.go` file plus `go.mod`/`go.sum`/`go.work`) on each request and
  rescans only when something moved. A cache hit costs a directory walk, not a
  `go list`. With `-watch` the page also reloads itself through `/events`
  (server-sent events); without it, press 重载 after editing code.
- CRAP metrics refresh when the tree changes, at the cost of one `go test` run
  per change; the disk cache only helps across restarts. `-no-crap` skips it,
  `-crap-profile` replaces the test run with an existing coverage file.
- A `-out` page is a snapshot: it grows with the inlined model and does not
  update when the code changes.
- It reads the module with `go list`, `go/parser` and `go/types`; it never
  writes to the project it inspects — the only file it writes is the `-out`
  page you name.

## Layout

| Path | Contents |
| --- | --- |
| `cmd/archview` | flags, wiring, exit codes |
| `internal/model` | scan + type check + relations + call graph + workspaces |
| `internal/crap` | go-crap collection and score mapping |
| `internal/config` | `archview.json` |
| `internal/cache` | fingerprint cache and the `-watch` broadcaster |
| `internal/export` | single-file page, uml-viewer IR in and out |
| `internal/web` | HTTP surface and the source page |
| `internal/assets` | `ui.html`, `ui.js`, `source.css` (embedded) |
| `internal/model/testdata` | fixture modules used by the tests |

## Development

```sh
gofmt -l .
go vet ./...
go test ./...
node --test
```

`internal/assets/ui.html` holds the page and the DOM code;
`internal/assets/ui.js` holds everything that can run without a document —
routing, layout, scene building, formatting — so the layout can be tested
(`ui.test.js`) instead of eyeballed. `node --test` needs no npm install; the
tests import `ui.js` directly.

CI runs those four commands plus `go build ./...` on `ubuntu-latest` and
`windows-latest` (`.github/workflows/ci.yml`).

## Versioning

Versions follow the rules in [RELEASING.md](RELEASING.md): a fix is a patch, a
new flag or field is a minor, and a broken CLI/JSON/EDN contract is a major.
Changes are recorded in [CHANGELOG.md](CHANGELOG.md), and the release workflow
refuses a tag that has no entry there.
## Version

A plain `go build` reports `dev`. Releases inject the version at link time:

```sh
go build -ldflags "-X main.version=1.2.3" -o archview ./cmd/archview
./archview -version        # archview 1.2.3
```

`.goreleaser.yaml` builds static binaries for linux/darwin/windows on
amd64/arm64 with that same flag; `go install github.com/Sizolity/archview/cmd/archview@latest`
works once the module is published.

## Design

How a scan becomes a page, the model schema and the trade-offs:
[docs/design.md](docs/design.md).

## Roadmap

Unimplemented features with acceptance criteria live in [ROADMAP.md](ROADMAP.md).

## License

MIT — see [LICENSE](LICENSE).
