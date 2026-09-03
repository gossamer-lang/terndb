#!/bin/bash
# Runs the demo transcript on all three tiers and fails if any two differ.
set -euo pipefail
cd "$(dirname "$0")/.."
gos build >/dev/null
gos build --release >/dev/null
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
./scripts/demo.sh gos run .              > "$out/vm.txt"
./scripts/demo.sh ./target/debug/terndb    > "$out/cranelift.txt"
./scripts/demo.sh ./target/release/terndb  > "$out/llvm.txt"
diff "$out/vm.txt" "$out/cranelift.txt"
diff "$out/vm.txt" "$out/llvm.txt"
echo "tier parity: VM, Cranelift and LLVM agree"
