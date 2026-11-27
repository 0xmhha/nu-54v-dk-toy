#!/bin/sh
set -eu
sdk=$(xcrun --sdk iphoneos --show-sdk-path)
clang=$(xcrun --sdk iphoneos --find clang)
exec "$clang" -arch arm64 -isysroot "$sdk" -miphoneos-version-min=15.1 "$@"
