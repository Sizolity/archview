# Releasing

Tags are what users install, so a tag means "this is a release". The rules
below exist so the number says what changed and nothing is published by
accident.

## Version rules

**Pre-1.0 (current):** new flags, new model fields and new UI behaviour ship as
**patch** releases. The version only moves to the next minor for a breaking
change or a deliberate milestone; 1.0 is for a CLI, JSON and EDN contract you
are willing to keep. A release is a moment worth installing, not a receipt for
every merged change: if nothing would change for the user, it waits for the
next one.

| Change | Bump | Examples |
| --- | --- | --- |
| bug fix, internal refactor, docs, performance, new flag or field (pre-1.0) | **PATCH** `0.4.x` | CRAP scores lost after a rescan; `goimports` grouping |
| breaking change, or a milestone worth a new minor | **MINOR** `0.x.0` | a renamed flag, a new diagram mode |
| after 1.0: removing or renaming a flag, a breaking JSON/EDN shape or output format | **MAJOR** `x.0.0` | renaming `-edn` |

Before 1.0 a breaking change may ship in a minor, but the changelog has to say
so in the first line. Reach 1.0 when the CLI, the model JSON and the EDN schema
are ones you are willing to keep.

## Release only when someone would notice

Documentation, tests, CI and internal chores do not get their own tags; they
ride along with the next release. Several fixes that land together are one
patch release, not three.

## Before tagging

1. `CHANGELOG.md` has an `## Unreleased` section holding every user-visible
   change since the last tag.
2. The gate is green on `main`: `gofmt -l .`, `go vet ./...`, `go test ./...`,
   `node --test`, and CI on both platforms.
3. Pick the number with the table above, then move the entries under a new
   `## [x.y.z] - YYYY-MM-DD` heading and leave `## Unreleased` empty.
4. Commit `chore(release): x.y.z`, then
   `git tag -a vX.Y.Z -m "archview X.Y.Z"` and `git push origin vX.Y.Z`.

## What CI enforces

`.github/workflows/release.yml` greps `CHANGELOG.md` for a `##` heading with the
tag's version before goreleaser runs, and fails the release when it is missing:
the changelog cannot drift from what was published.

## Releases are approved, not scheduled

A release is a decision the maintainer makes, not a side effect of a merge. No
tag is cut, no `chore(release)` commit is made and no workflow is dispatched
without an explicit "release this" instruction in the same conversation. When in
doubt, leave the change in `## Unreleased`.

Never lower a version: a published tag is immutable in the module proxy, so
re-tagging an old number makes the proxy and the repository disagree forever.

## A note on the early history

`v0.1.1`–`v0.1.3` were cut per commit (licence, package docs, tool cleanups),
`v0.1.5` shipped a feature but was numbered as a patch, and `v0.2.0`–`v0.4.0`
ran ahead of the changes they described. They stay as they are: the proxy has
cached them, so the numbers cannot be reused or lowered, and rewriting history
would only make the tags lie. Treat them as an exploratory line and count from
the next release under the rules above.
