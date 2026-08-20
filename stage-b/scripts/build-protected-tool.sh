#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
  echo "usage: build-protected-tool.sh SOURCE_ROOT COMMAND OUTPUT" >&2
  exit 2
fi

source_root=$1
command_name=$2
output=$3
output_dir=$(dirname "$output")
output_name=$(basename "$output")

[ -f "$source_root/go.mod" ]
[ ! -w "$source_root" ]
mkdir -p "$output_dir"
chmod 0777 "$output_dir"

docker run --rm \
  --platform linux/amd64 \
  --network none \
  --read-only \
  --user 65532:65532 \
  --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --pids-limit 128 \
  --memory 512m \
  --cpus 1 \
  --tmpfs /work:rw,nosuid,nodev,size=268435456 \
  --mount "type=bind,src=${source_root},dst=/src,readonly" \
  --mount "type=bind,src=${output_dir},dst=/out" \
  --workdir /src/stage-b \
  golang:1.26.5-bookworm@sha256:1ecb7edf62a0408027bd5729dfd6b1b8766e578e8df93995b225dfd0944eb651 \
  /usr/bin/env -i \
    PATH=/usr/local/go/bin:/usr/bin:/bin \
    HOME=/work/home \
    GOCACHE=/work/gocache \
    GOMODCACHE=/work/gomodcache \
    GOTOOLCHAIN=local \
    GOPROXY=off \
    GOSUMDB=off \
    GOFLAGS=-mod=readonly \
    CGO_ENABLED=0 \
    go build -trimpath -buildvcs=false '-ldflags=-s -w' -o "/out/${output_name}" "./cmd/${command_name}"

chmod 0555 "$output"
chmod 0555 "$output_dir"
sha256sum "$output"
