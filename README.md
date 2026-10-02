# worktrunk-cleanup

## Overview

`worktrunk-cleanup` provides `wt cleanup`, a forge-independent Worktrunk
extension for removing clean worktrees whose changes are integrated into the
repository's default branch.

## How to Use This

Install the extension:

```sh
go install github.com/stuft2/worktrunk-cleanup/cmd/wt-cleanup@latest
```

Ensure your Go bin directory is on `PATH`. Worktrunk discovers the installed
`wt-cleanup` executable and exposes it as `wt cleanup`.

```sh
wt cleanup       # fetch and preview integrated worktrees
wt cleanup --yes # remove matching worktrees and local branches
```

Cleanup mirrors Worktrunk's safe removal controls:

```sh
wt cleanup --no-delete-branch # keep local branches
wt cleanup --reap             # stop non-interactive worktree processes
wt cleanup --no-hooks         # skip removal hooks
wt cleanup --format=json      # emit machine-readable results
```

The command recognizes regular, rebased, and squash merges through
Worktrunk's native integration checks. It skips primary, current, locked,
detached, dirty, and unmerged worktrees.

## How to Build Locally

```sh
go build ./cmd/wt-cleanup
```

## How to Publish

Releases are published automatically after the checks pass on `main`. Pull
request titles must follow Conventional Commits. `feat` changes create minor
releases, `fix` changes create patch releases, and breaking changes are marked
with `!` or a `BREAKING CHANGE` footer. During initial development, breaking
changes also create minor releases.

Changes such as `docs`, `test`, and `ci` do not publish a release on their own.
Each release creates a semantic version tag and generated GitHub release notes.
Install a released version with `go install` and its Git tag, for example:

```sh
go install github.com/stuft2/worktrunk-cleanup/cmd/wt-cleanup@v0.1.0
```

## Architectural Overview

The extension is a single, standard-library-only Go command. It fetches the
default branch reported by `wt list --format=json`, asks Worktrunk which clean
worktrees are integrated, and delegates each confirmed removal to `wt remove`
so repository hooks and Worktrunk safety checks still apply.

External runtime dependencies are Git and Worktrunk. No forge CLI or API is
required.

## How to Test

```sh
go test ./...
go vet ./...
```
