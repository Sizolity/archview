# Changelog

Notable changes, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) as described in
[RELEASING.md](RELEASING.md). A tag without a section here fails the release
workflow.

## Unreleased

## [0.4.1] - 2026-10-30

### Added

- One interaction model for every card: a single click selects (a package card
  too, highlighting its import edges and opening its panel), a double click
  drills into the package or opens the source. Clicking the empty canvas or
  pressing Esc clears the selection.
- Function and value cards can be selected like type cards: click opens a panel
  (`#/func/<id>`, `#/value/<id>`) with the package, position, doc, CRAP and
  calls; double-click opens the source.
- The style legend sits in the canvas corner: card colours, the entry/change
  markers, and the three line classes.
- References to types and to package-level consts and vars are edges now
  (`uses`/`value`), resolved through `go/types`: a function that returns a type
  or reads a constant is connected to it. The model gains 140 edges here
  (25 → 165), and a package like `internal/crap` stops looking empty.
- Call edges connect the cards of one package (function to function, method to
  its owner type); a cross-package call is drawn at module level only when no
  import exists in that direction.
- A cycle is first-class: strongly connected components are condensed, a scene
  containing one shows `环(强连通): A ⇄ B` in a banner, and its edges are red
  dashed. Layout-induced cross-row hops are a separate grey class (`span`).
- Package cards show the package doc comment and the worst CRAP inside it.

### Changed

- The graph is laid out from its edges: Tarjan condensation, longest-path
  layers, deterministic barycentre ordering, left-packed rows that wrap at
  1300px and a fixed 120px gap. One flow direction per view; the horizontal
  layout, its selector and the per-row grids are gone. `internal/model` with
  every member shown went from 5,800×15,278 to 1,266×5,059.
- The page is one renderer: `buildCard`, `drawNode` and `CARD_STYLE` plus pure
  panel functions in `ui.js`; `ui.html` lost 300 lines, and one delegated
  click/double-click handler and one card index replace the per-card closures.
- `精简` and `聚焦` are gone; `调用边` draws declaration-level calls.
- Cards can no longer be dragged: the position was never reapplied after a
  render, and the drag could swallow the click that followed it.

### Fixed

- A single click could be lost: the scene was redrawn on every mouse move, so a
  one-pixel wobble replaced the card under the pointer before the click fired.
- A call edge between two cards in one row was routed bottom-to-top through its
  target, hiding its arrowhead at a corner.

## [0.4.0] - 2026-10-29

### Added

- The package view draws package-level functions and, in a second band, the
  constants and variables: an embed package or a command no longer opens on an
  empty canvas. Values show their initialiser (`const = "archview.json"`).
- Double-clicking a type, function or value card opens its source page at the
  declaration, with the back link returning to the view you left. A static
  export falls back to `file://`, an imported IR has no source to open.
- The module card counts constants and variables, and packages that contain
  changes are marked in the module view.

### Fixed

- Value cards in a package without functions got `y = NaN`: the band base was
  computed inside the functions-only branch, so the browser drew an empty card.
- The toolbar squeezed instead of scrolling: the breadcrumb wrapped one
  character per line and the buttons stacked. It is a fixed 56px row again and
  scrolls sideways when it does not fit.

### Changed

- `main` is now `run` plus a three-line entry point: the command returns errors
  (exit status 2 still means a bad config) and its assembly is split into
  `parseFlags`, `makeCollector`, `loadInput`, `writeOutputs` and `serve`. CRAP
  for `main` fell from 1190 to 12, and nothing in the module exceeds 30 now.
- The EDN importer is split into `readPackage`, `readClass`, `readFields`,
  `readOps` and `readEdges`, so a malformed part is counted where it is found.
## [0.3.0] - 2026-10-28

### Added

- `package main` is modelled as the program entry (`Package.Main`): the card
  says "程序入口", carries a badge, and always shows its unexported members —
  an entry has no exported API, so the default filter used to draw it empty.
- Packages that contain changes are marked in the module view, so the graph you
  land on after starting points at the work.
- The model's warnings are shown in a strip under the toolbar: a failed
  go-crap, git diff or type check used to look exactly like a healthy run.
- EDN export writes `:private true` on package-visible members, matching the
  upstream IR, and the importer reads it back.

### Fixed

- `-diff`: the parser tracked headers by line prefix, so a changed source line
  starting with `+++`/`@@` could be read as a header; quoted paths kept their
  `b/` prefix. A diff that touched no declaration no longer leaves an empty
  summary on the model.
- `/source` followed symlinks out of the module.
- A failed `-diff` is logged instead of only recorded in the model.
## [0.2.0] - 2026-10-28

### Added

- `-diff REF` marks the declarations the working tree changed since a git ref:
  `changed` on functions, methods and types, a `diff` summary on the module, an
  orange edge and dot on the cards, a badge in the inspector, and
  "改动 N 文件 / M 函数" in the status line.

### Changed

- The model JSON gains `changed` (declarations) and `diff` (module), and
  declarations now carry their end line.

## [0.1.5] - 2026-10-28

### Added

- `-crap-profile` reuses an existing coverage profile, so go-crap no longer has
  to run the module's tests.
- The CRAP report is cached in the user's cache directory, keyed by the tree
  fingerprint, the go-crap binary and the flags; `-cache-dir` and
  `ARCHVIEW_CACHE_DIR` relocate it, `-no-crap-cache` turns it off.

> Numbered as a patch when it shipped; under the current rules this is a minor
> release (new flags).

## [0.1.4] - 2026-10-28

### Fixed

- CRAP scores disappeared after any rescan: the report was collected once at
  startup and keyed by `file:line`, so a shifted line orphaned it. It is now
  collected with the model it describes.
- `/model.json` built the model under a 15-second request deadline; a scan that
  includes go-crap's coverage run was killed and returned no scores. Builds use
  the process context with a two-minute budget.

## [0.1.3] - 2026-10-28

### Changed

- Ran the local toolchain until it was quiet: `go fix` modernisations,
  `goimports` grouping, and the comments `staticcheck -checks=all` flagged.

## [0.1.2] - 2026-10-28

### Added

- A package comment on every internal package, so pkg.go.dev explains what each
  one is for.

## [0.1.1] - 2026-10-28

### Added

- MIT licence, mention in the README, and the file inside the release archives.

## [0.1.0] - 2026-10-28

### Added

- First release: package/type/relation model, browser view with layered and
  grid layouts, call graph, CRAP scores from go-crap, `-editor` jump modes,
  single-file export, uml-viewer IR export and import, `-watch` with SSE reload,
  `go.work` support, and the config file.
