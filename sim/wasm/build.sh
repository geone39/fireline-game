#!/bin/sh
# Builds sim/wasm/dist/sim.wasm (+ wasm_exec.js from GOROOT). Run from anywhere.
set -eu
cd "$(dirname "$0")"
mkdir -p dist
GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o dist/sim.wasm .
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" dist/wasm_exec.js
ls -l dist/sim.wasm
