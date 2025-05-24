#!/bin/bash

echo "Attempting to cross-compile for Windows (x86_64)..."

# Ensure Go modules are tidy (dependencies are downloaded)
go mod tidy

# Set environment variables for Windows cross-compilation
export GOOS=windows           # Target operating system
export GOARCH=amd64           # Target architecture (64-bit)
export CGO_ENABLED=1          # Crucial: Enable Cgo for linking C libraries like GLFW
export CC=x86_64-w64-mingw32-gcc # C compiler for cross-compilation
export CXX=x86_64-w64-mingw32-g++ # C++ compiler for cross-compilation (might be needed for some Cgo dependencies)

# Define the output executable name
OUTPUT_NAME="SpinningModels.exe"

# Perform the build
go build -o "$OUTPUT_NAME" .

# Check if the build was successful
if [ $? -eq 0 ]; then
    echo "--------------------------------------------------------"
    echo "Successfully compiled for Windows!"
    echo "Executable: $(pwd)/$OUTPUT_NAME"
    echo "Copy this .exe file to your Windows machine to run it."
    echo "Note: go-gl/glfw typically statically links GLFW for Windows, so no separate DLLs are usually needed."
    echo "--------------------------------------------------------"
else
    echo "--------------------------------------------------------"
    echo "Cross-compilation FAILED!"
    echo "Please check the error messages above for details."
    echo "Common issues: "
    echo "  - mingw-w64 not installed or path incorrect."
    echo "  - CGO_ENABLED not set to 1."
    echo "  - Go module dependencies are not met (run 'go mod tidy')."
    echo "--------------------------------------------------------"
fi

# Unset environment variables to avoid interfering with subsequent Go builds
unset GOOS
unset GOARCH
unset CGO_ENABLED
unset CC
unset CXX
