#!/usr/bin/env python3
"""Read-only source manifest for acceptance targets; no Git mutation commands."""
import argparse
import hashlib
import json
import os
import pathlib
import subprocess


def manifest(root):
    root = pathlib.Path(root).resolve()
    paths = subprocess.check_output(
        ["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"], cwd=root
    ).split(b"\0")
    result = {}
    for raw in sorted(set(paths)):
        if not raw:
            continue
        name = os.fsdecode(raw)
        path = root / name
        if path.is_symlink():
            result[name] = {"symlink": os.readlink(path)}
        elif path.is_file():
            with path.open("rb") as stream:
                hasher = hashlib.sha256()
                for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                    hasher.update(chunk)
                digest = hasher.hexdigest()
            result[name] = {
                "sha256": digest,
                "mode": path.stat().st_mode & 0o777,
            }
        else:
            result[name] = {"missing": True}
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root")
    parser.add_argument("--compare", type=pathlib.Path)
    parser.add_argument("--output", type=pathlib.Path)
    args = parser.parse_args()
    current = manifest(args.root)
    if args.compare:
        previous = json.loads(args.compare.read_text())
        changed = sorted(k for k in current.keys() | previous.keys() if current.get(k) != previous.get(k))
        print(json.dumps({"unchanged": not changed, "paths": len(current), "changed": changed}, indent=2))
        return int(bool(changed))
    data = json.dumps(current, sort_keys=True, indent=2)
    if args.output:
        target = args.output.resolve()
        if target.is_relative_to(pathlib.Path(args.root).resolve()):
            parser.error("manifest output must be outside the source target")
        target.write_text(data + "\n")
        print(f"Recorded {len(current)} source paths in {target}")
    else:
        print(data)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
