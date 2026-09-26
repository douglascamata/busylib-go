# Agent instructions

These instructions apply to the whole repository.

## Changelog

Add a Changie fragment for every user-facing change, in the same commit:
`make change ARGS='--kind Fixed --body "..."'`. Write the body for library
users, not as a commit summary. Skip it for CI, test, and tooling changes.
CI fails pull requests that need a fragment and lack one. The
`skip-changelog` label bypasses the check. See the Releasing section of the
[README](README.md#releasing).

## Upstream reviews

For an upstream review, use the
[review-upstream skill](.agents/skills/review-upstream/SKILL.md) or run the
script from the repository root with uv, Python 3, and Git:

```bash
uv run .agents/skills/review-upstream/scripts/review-upstream.py py --from REVIEWED_SHA --to TARGET_REF --output .cache/upstream-review/py.md
```

Replace the placeholders with a previously reviewed commit and the target commit,
branch, or tag. Use `py` (busylib-py) as the primary client source and `ts`
(busylib-ts) as secondary. Sources also include `protobuf` and `firmware` for
wire behavior. The report includes resolved revisions, source links, all changed
files, and a focused diff. This is an on-demand review aid; it does not certify
compatibility or update any pins.
