#!/bin/bash

# Install wasm-pack if not already installed
if ! command -v wasm-pack &> /dev/null; then
    echo "Installing wasm-pack..."
    curl https://rustwasm.github.io/wasm-pack/installer/init.sh -sSf | sh
fi

# Build the WASM package
echo "Building WASM package..."
wasm-pack build --target web --release

# Create static directory if it doesn't exist
mkdir -p ../static

# Copy the WASM files to the static directory
echo "Copying WASM files to static directory..."
cp pkg/flippy_wasm_bg.wasm ../static/
cp pkg/flippy_wasm.js ../static/

echo "Build complete! WASM files are in the static directory."
