#!/usr/bin/env python3
"""Inspect a JSON report without treating unreviewed candidates as proven obsolete."""
import argparse
import collections
import json
import pathlib


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("report", type=pathlib.Path)
    parser.add_argument("--symbol")
    args = parser.parse_args()
    data = json.loads(args.report.read_text())
    if args.symbol:
        symbols = [s for s in data["symbols"] if args.symbol in s["id"]]
        ids = {s["id"] for s in symbols}
        out = {"symbols": symbols, "findings": [f for f in data["findings"] if f["symbol"] in ids],
               "references": [r for r in data["references"] if r["symbol"] in ids]}
    else:
        by_category = collections.defaultdict(list)
        for f in data["findings"]:
            if len(by_category[f["category"]]) < 8:
                by_category[f["category"]].append(f["symbol"])
        out = {"complete": data["complete"], "summary": data["summary"], "metrics": data["metrics"],
               "records": {k: len(data[k]) for k in ["symbols", "edges", "references", "groups"]},
               "examples": by_category, "diagnostics": data["diagnostics"]}
    print(json.dumps(out, indent=2))


if __name__ == "__main__":
    main()
