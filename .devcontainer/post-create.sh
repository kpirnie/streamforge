#!/bin/sh
set -eu

cd "$(dirname "$0")/.."

# root in the container doesn't own the mounted repo
git config --system --add safe.directory "$PWD"

mkdir -p /working/input /working/output

go mod download
go build ./...
