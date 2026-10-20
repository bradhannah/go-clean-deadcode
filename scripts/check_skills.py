#!/usr/bin/env python3
"""Offline portable-skill contract checks; not a substitute for agent evaluations."""
import ast
import json
import pathlib
import re
import subprocess


def main():
    root = pathlib.Path(__file__).resolve().parents[1]
    for name in ["go-clean-deadcode", "go-clean-deadcode-workflow"]:
        directory = root / "skills" / name
        skill = directory / "SKILL.md"
        text = skill.read_text()
        assert text.startswith("---\n"), skill
        frontmatter = text.split("---", 2)[1]
        assert f"name: {name}\n" in frontmatter, skill
        assert "description: Use when" in frontmatter, skill
        assert len(text.splitlines()) < 500, skill
        for doc in directory.rglob("*.md"):
            content = doc.read_text()
            assert "/Users/" not in content, doc
            for block in re.findall(r"```json\n(.*?)```", content, re.DOTALL):
                json.loads(block)
            for link in re.findall(r"\]\(([^)]+)\)", content):
                if "://" in link or link.startswith("#"):
                    continue
                target = (doc.parent / link.split("#")[0]).resolve()
                assert target.is_relative_to(directory.resolve()) and target.is_file(), (doc, link)
        for script in directory.rglob("*.py"):
            ast.parse(script.read_text(), filename=str(script))
        for script in directory.rglob("*.sh"):
            subprocess.run(["sh", "-n", str(script)], check=True)
    evals = json.loads((root / "skills/evals/evals.json").read_text())
    assert len(evals["evals"]) >= 2
    for case in evals["evals"]:
        assert case["prompt"] and case["expected_output"] and case["assertions"]
    for doc in [root / "README.md", root / "AGENTS.md", *sorted((root / "docs").glob("*.md")), root / "skills/README.md"]:
        for link in re.findall(r"\]\(([^)]+)\)", doc.read_text()):
            if "://" not in link and not link.startswith("#"):
                assert (doc.parent / link.split("#")[0]).is_file(), (doc, link)
    print("Portable skill metadata, bundled links, scripts and evaluation scenarios passed.")


if __name__ == "__main__":
    main()
