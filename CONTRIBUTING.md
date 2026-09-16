# Contributing

## Commit convention

This repo follows [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<optional scope>)!: <subject>
```

Common types: `feat`, `fix`, `perf`, `refactor`, `docs`, `chore`. A `!` after
the type/scope, or a `BREAKING CHANGE:` footer, marks a breaking change.

**PRs are squash-merged, so the PR title becomes the permanent commit
message.** Write PR titles as Conventional Commits — that's what the release
tooling parses to decide the next version (`fix` -> patch, `feat` -> minor,
breaking change -> major). Repo Settings -> General -> "Default commit
message for squash merges" should be set to "Pull request title" so GitHub
doesn't append individual commit subjects onto the squashed message.

If a release is cut and no commits since the last one follow this
convention, the release tooling falls back to a patch bump rather than
failing — but following the convention gives accurate version bumps and
changelogs.

## Releasing

See `.mise/tasks/{version-next,release-dry-run,release}`. All modules in
this repo (`trails` root plus the `pack/driver/*` and `jobs/backend/cloudtask`
submodules) are released together under one lockstep version.

- `mise run version-next` — preview the next version and the commits behind it.
- `mise run release-dry-run` — preview the full release (tags + notes), no side effects.
- `mise run release` — cut the release: rewrites each submodule's
  `github.com/asimmons91/trails*` `go.mod` require lines to the released
  version (and commits that change), tags all modules, pushes, and creates
  a GitHub Release.

Submodules pin `github.com/asimmons91/trails` (and cross-submodule deps,
e.g. `cloudtask` -> `pack/driver/sqlite`) via a local `replace` directive
for in-repo development, but `replace` directives are ignored when a module
is consumed as a dependency from outside this repo. `mise run release`
rewrites the corresponding `require` lines to the real released version
before tagging, so external consumers resolve a real `trails` version
instead of the local placeholder.

The same tasks run identically locally or via the `Release` GitHub Actions
workflow (`workflow_dispatch`, with a `dry_run` input).
