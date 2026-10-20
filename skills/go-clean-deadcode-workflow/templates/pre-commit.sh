#!/bin/sh
# Integration example: working-source audit, NOT staged-tree verification.
# Incorporate into the existing hook chain only after explicit installation approval.
set -eu

root=$(git rev-parse --show-toplevel)
tool=${GO_CLEAN_DEADCODE_BIN:-go-clean-deadcode}
config=${GO_CLEAN_DEADCODE_CONFIG:-"$root/go-clean-deadcode.json"}

set -- analyze --root "$root" --config "$config" --format text --timeout 5m
if [ -n "${GO_CLEAN_DEADCODE_CACHE:-}" ]; then
    set -- "$@" --cache-dir "$GO_CLEAN_DEADCODE_CACHE"
fi
exec "$tool" "$@"
