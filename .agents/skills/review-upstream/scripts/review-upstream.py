#!/usr/bin/env python3
"""Compare two upstream snapshots for a manual busylib-go review."""

import argparse
import subprocess
import sys
from pathlib import Path
from urllib.parse import quote

SOURCES = {
    "py": (
        "busy-app/busylib-py",
        ["src/busylib", "tests"],
    ),
    "ts": (
        "busy-app/busylib-ts",
        ["src/BusyBar", "src/Global", "src/StateStream"],
    ),
    "protobuf": ("busy-app/busybar-protobuf", ["."]),
    "firmware": (
        "busy-app/busybar-firmware",
        [
            "applications/services/web_server",
            "applications/services/state_publisher",
            "applications/services/front_display/front_display.h",
            "applications/services/back_display/back_display.h",
            "lib/toolbox/rle_encode.c",
            "lib/toolbox/rle_encode.h",
            "lib/toolbox/color.c",
            "lib/toolbox/color.h",
            "lib/toolbox/compress.c",
            "lib/toolbox/compress.h",
            "assets/proto",
        ],
    ),
}


def git(*args):
    return subprocess.check_output(["git", *map(str, args)], text=True)


def fence(text, language=""):
    # Upstream patches can themselves contain Markdown code fences.
    marker = "```"
    while marker in text:
        marker += "`"
    return f"{marker}{language}\n{text.rstrip()}\n{marker}"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", choices=SOURCES)
    parser.add_argument(
        "--from", dest="base", required=True, help="reviewed commit, branch, or tag"
    )
    parser.add_argument(
        "--to", dest="target", required=True, help="target commit, branch, or tag"
    )
    parser.add_argument(
        "--output", type=Path, help="write Markdown here instead of stdout"
    )
    args = parser.parse_args()

    repository, paths = SOURCES[args.source]
    url = f"https://github.com/{repository}"
    root = Path(__file__).resolve().parents[4]
    cache = root / ".cache" / "upstream-review" / f"{args.source}.git"
    if not cache.exists():
        cache.parent.mkdir(parents=True, exist_ok=True)
        git("clone", "--quiet", "--bare", "--filter=blob:none", "--depth=1", url, cache)

    revisions = []
    for ref in (args.base, args.target):
        git(
            "-C",
            cache,
            "fetch",
            "--quiet",
            "--depth=1",
            "--no-tags",
            "--filter=blob:none",
            "--",
            "origin",
            ref,
        )
        revisions.append(git("-C", cache, "rev-parse", "FETCH_HEAD^{commit}").strip())
    base, target = revisions

    # Compare snapshots directly; a shallow cache need not contain a merge base.
    diff = [
        "-C",
        cache,
        "diff",
        "--no-ext-diff",
        "--no-textconv",
        "--no-renames",
        base,
        target,
    ]
    files = git(*diff, "--name-only", "-z", "--").rstrip("\0").split("\0")
    focused = git(*diff, "--name-only", "-z", "--", *paths).rstrip("\0").split("\0")
    files = [path for path in files if path]
    focused = {path for path in focused if path}
    patch = git(*diff, "--patch", "--no-color", "--", *paths)
    summary = git(*diff, "--stat", "--", *paths)

    report = [
        f"# Upstream review: {repository}",
        f"Base: [`{base}`]({url}/tree/{base})  \nTarget: [`{target}`]({url}/tree/{target})",
        ("This compares two snapshots. It does not prove Go or firmware compatibility. "
        "The script does not update reviewed revisions or library code."),
        "## Diff scope",
        fence("\n".join(paths)),
        "## All changed files",
        ("Files outside the diff scope still need triage. New modules or moved code "
        "may require a wider review."),
    ]
    entries = []
    for path in files:
        label = path.replace("\\", "\\\\").replace("[", "\\[").replace("]", "\\]")
        encoded = quote(path, safe="/")
        scope = "included" if path in focused else "outside scope"
        entries.append(
            f"- {label} ({scope}): [base]({url}/blob/{base}/{encoded}), "
            f"[target]({url}/blob/{target}/{encoded})"
        )
    report.append("\n".join(entries) or "No files changed between these revisions.")
    if files:
        report.append(
            "For added or deleted files, only one of the two file links exists."
        )
    report.extend(
        [
            "## Focused diff",
            fence(summary) if summary else "No changes in the selected paths.",
            fence(patch, "diff") if patch else "",
        ]
    )
    text = "\n\n".join(report).rstrip() + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(text)
        print(f"Wrote {args.output}", file=sys.stderr)
    else:
        sys.stdout.write(text)


if __name__ == "__main__":
    try:
        main()
    except (OSError, subprocess.CalledProcessError) as error:
        print(f"Upstream review failed: {error}", file=sys.stderr)
        sys.exit(1)
