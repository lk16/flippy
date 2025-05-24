#!/bin/bash
set -e

# Compile to WebAssembly
GOOS=js GOARCH=wasm go build -o static/evaluate.wasm cmd/evaluate/main.go

# Copy the wasm_exec.js file from Go's installation
cp $(dirname $(which go))/../lib/wasm/wasm_exec.js static

