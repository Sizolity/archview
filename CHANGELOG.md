# Changelog

Notable changes, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) as described in
[RELEASING.md](RELEASING.md). A tag without a section here fails the release
workflow.

## Unreleased

### Fixed

- `-diff`: the hunk parser matched `+++`/`@@` by line prefix, so a changed
  source line that began with either could be read as a diff header. It now
  tracks hunk state. Quoted paths (spaces, non-ASCII) kept their `b/` prefix
  because the prefix was trimmed before unquoting.
- `/source` followed symlinks out of the module; it now resolves the path and
  requires it to stay under the root.

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
