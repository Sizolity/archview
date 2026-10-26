# archview

A read-only, browser-based view of a Go module's architecture: packages,
types, fields, methods, constants, variables and the references between them.
It is a development tool, standard library only, and works against any Go
module.

## Run

Build once and run the binary from the module you want to inspect:

```sh
go build -o archview .

cd /path/to/your/project
/path/to/archview            # http://127.0.0.1:8788
```

Or run it from source:

```sh
go run . -addr 127.0.0.1:8788
```

`-addr` defaults to `127.0.0.1:8788`, so the viewer is loopback-only unless you
explicitly change it.

## What it shows

- **Module diagram**: one box per package, arrows for imports. Click a box to
  drill in.
- **Package view**: one card per type; fields and methods listed inside the
  card, `+` exported and `-` unexported.
- **Inspector**: package docs, type docs, fields, methods, outgoing references
  and "used by", plus a link to the exact source line.
- **Source view**: the file with line numbers; `#L35` anchors jump to the
  declaration.
- **Search**, an **include unexported** toggle, pan/zoom and a fit-to-window
  button.

## HTTP surface

| Path | Purpose |
| --- | --- |
| `/` | the explorer UI |
| `/model.json` | the scanned model |
| `/source?path=<relative>.go` | read-only source view |
| `/healthz` | liveness probe |

## Limits

- Package and type structure only; there is no call graph.
- References are syntactic (AST), so interface satisfaction, aliases and
  generic instantiations are not resolved.
- No CRAP/mutation score coloring yet.
- It reads the module with `go list` and `go/parser`; it never writes to the
  project.

## Development

```sh
gofmt -l .
go vet ./...
go test ./...
```

## Roadmap

Unimplemented features with acceptance criteria live in [ROADMAP.md](ROADMAP.md).

