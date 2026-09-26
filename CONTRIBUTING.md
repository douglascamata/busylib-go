# Contributing

Thanks for helping with busylib-go. This guide covers the development setup,
changelog fragments, communication in issues and pull requests, and releases.

## Development

```bash
make build            # compile every package
make test             # tests with the race detector
make smoke-test       # boots busybar-emulator and runs the smoke tests
make lint             # gofmt, go vet, golangci-lint, lint-actions
make lint-actions     # audit the GitHub Actions workflows with zizmor
make generate-proto   # regenerate statestream/pb from proto/
make update-protos    # fetch the latest bsb-protobuf schemas, then regenerate
make change           # add a changelog fragment for a user-facing change
make changelog        # preview the next version and its release notes
make check-changes    # validate the changelog fragments
make update-changelog # rebuild CHANGELOG.md from .changes/
```

The smoke tests drive the HTTP client and the WebSocket stream against
[busybar-emulator](https://github.com/maxswinkels/busybar-emulator). The
script clones the revision in `scripts/emulator-revision` into `.cache/` on
first run. Set `BUSYBAR_EMULATOR_DIR` to reuse a clean checkout of that revision.
Existing checkouts are never reset by the script.

CI installs FFmpeg and runs the tests, lint checks, and pinned emulator smoke tests.
The normal test suite includes local mDNS discovery tests, which need
multicast-capable network interfaces. Locally, tests that execute FFmpeg skip when it is not installed.
Run `go test -race -v ./media` with FFmpeg installed to check the converted
PNG pixels and PCM samples, including all six audio input formats. Python/Pillow image references
can be regenerated with `python3 media/testdata/generate.py` (requires Pillow).

Agent instructions are in [AGENTS.md](AGENTS.md).

## Changelog fragments

The changelog is built with [Changie](https://changie.dev/) from fragments in
`.changes/unreleased/`, one per user-facing change. Commit messages are not
used, so a fragment can explain the change for library users.

Add a fragment in the same commit as the change:

```bash
make change                                             # prompts for kind and text
make change ARGS='--kind Fixed --body "Describe the fix"'
```

Kinds follow [Keep a Changelog](https://keepachangelog.com/): Added, Changed,
Deprecated, Removed, Fixed, and Security. Fragments are plain YAML. Edit, merge,
or delete them freely until the release.

### When to skip the fragment

Changes with no user impact, such as CI, tooling, test, or documentation
updates, need no fragment. If the Changelog check still fails on such a pull
request, for example an internal refactor of library code, add the
`skip-changelog` label to the PR. The check re-runs when labels change.

Do not use the label to skip a fragment for a change that library users would
notice.

## AI use and communication

Using AI tools to write code, tests, or docs is fine and does not need to be
disclosed. You are responsible for what you submit, as with any other
contribution.

All communication in issues and pull requests must be from human to human.
Write descriptions, comments, and replies yourself; do not post AI-generated
text or let an agent talk on your behalf. The only exception is PR review
bots, such as Codex or Copilot, whose automated reviews are welcome.

## Releasing

The kinds of the unreleased fragments choose the next version: Fixed and
Security bump the patch version, and every other kind bumps the minor version.
While the module is on v0, breaking changes bump the minor version too. Run
`make changelog` to see the next version and its notes.

To release, check the notes with `make changelog` and polish the fragments.
Then run this on a clean, up-to-date `main`:

```bash
make prepare-release                  # automatic version
make prepare-release VERSION=v0.3.0   # explicit version
```

The target batches the fragments into `.changes/<version>.md`, rebuilds
`CHANGELOG.md`, and commits both as `Release <version>`. It never tags or
pushes. It prints the commands for that, to run after reviewing the commit:

```bash
git tag -a v0.3.0 -m v0.3.0
git push --atomic origin main v0.3.0
```

The Go module proxy picks up the new tag on its own. To change the notes,
edit `.changes/<version>.md` and run `make update-changelog`, then amend the
release commit, or commit the fix later for a published release.
`CHANGELOG.md` is generated, so edits made directly to it are lost. To drop an
unpushed release commit, run `git reset --hard HEAD~1`.
