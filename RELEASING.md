# Releasing

Tags are what users install, so a tag means "this is a release". The rules
below exist so the number says what changed and nothing is published by
accident.

## Version rules

| Change | Bump | Examples |
| --- | --- | --- |
| bug fix, internal refactor, docs, performance | **PATCH** `0.2.x` | CRAP scores lost after a rescan; `goimports` grouping |
| new flag, new model/JSON field, new UI capability, behaviour users must notice | **MINOR** `0.x.0` | `-diff`, the CRAP cache |
| removing or renaming a flag, a breaking JSON/EDN shape or output format | **MAJOR** `x.0.0` | renaming `-edn` |

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

## A note on the early history

`v0.1.1`–`v0.1.3` were cut per commit (licence, package docs, tool cleanups),
and `v0.1.5` shipped a feature but was numbered as a patch. Both are what this
document exists to prevent. The rules apply from `v0.2.0` onward; published tags
are never moved.
