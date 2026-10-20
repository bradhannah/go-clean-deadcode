#!/usr/bin/env python3
"""Read schema-1 reports; selected witnesses are not exhaustive test ownership."""
import argparse
import json


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("report")
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--symbol")
    group.add_argument("--test")
    args = parser.parse_args()
    try:
        with open(args.report, encoding="utf-8") as stream:
            data = json.load(stream)
        if data.get("schema_version") != 1:
            raise ValueError("unsupported report schema")
        out = {k: data[k] for k in ["mode", "complete", "scopes", "summary", "diagnostics"]}
        if args.symbol:
            ids = {args.symbol}
            ids.update(f["symbol"] for f in data["findings"] if f["id"] == args.symbol)
            out["symbols"] = [s for s in data["symbols"] if s["id"] in ids]
            out["findings"] = [f for f in data["findings"] if f["symbol"] in ids]
            out["references"] = [r for r in data["references"] if r["symbol"] in ids]
            out["incoming_edges"] = [e for e in data["edges"] if e["callee"] in ids]
            out["outgoing_edges"] = [e for e in data["edges"] if e["caller"] in ids]
            out["groups"] = [g for g in data["groups"] if ids.intersection(g["members"])]
        elif args.test:
            out["witness_subjects_not_exhaustive"] = [
                {"symbol": s["id"], "witness": witness}
                for s in data["symbols"] for state in s["states"] for witness in state["witnesses"]
                if witness["root"] == args.test
            ]
        print(json.dumps(out, indent=2))
        return 0
    except (OSError, ValueError, KeyError, TypeError) as error:
        print(json.dumps({"error": str(error)}))
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
