#!/bin/sh
set -eu

emit() {
  classification="$1"
  detail="$2"
  printf '{"schema_version":"1.0","classification":"%s","detail":"%s"}\n' "$classification" "$detail"
}

fail() {
  emit "$1" "$2"
  exit 1
}

fail_with_diagnostic() {
  classification="$1"
  detail="$2"
  diagnostic_file="$3"
  diagnostic="$(tail -c 4096 "$diagnostic_file" | base64 | tr -d '\n')"
  printf '{"schema_version":"1.0","classification":"%s","detail":"%s","diagnostic_encoding":"base64","diagnostic_last_bytes_limit":4096,"diagnostic":"%s"}\n' \
    "$classification" "$detail" "$diagnostic"
  exit 1
}

[ "$(id -u)" != "0" ] || fail isolation_failure running_as_root
[ ! -S /var/run/docker.sock ] || fail isolation_failure docker_socket_present
[ ! -e /run/host-services/docker.proxy.sock ] || fail isolation_failure host_socket_present

case "${GITHUB_TOKEN-}${GH_TOKEN-}${ACTIONS_RUNTIME_TOKEN-}${ACTIONS_ID_TOKEN_REQUEST_TOKEN-}" in
  "") ;;
  *) fail isolation_failure token_environment_present ;;
esac

mkdir -p /work/home /work/gocache /work/gomodcache /work/gotmp /work/project
export HOME=/work/home
export GOTOOLCHAIN=local
export CGO_ENABLED=0
export GOFLAGS=-mod=readonly
export GOPROXY=off
export GOSUMDB=off
export GOCACHE=/work/gocache
export GOMODCACHE=/work/gomodcache
export GOTMPDIR=/work/gotmp

[ -w /src ] && fail isolation_failure source_mount_is_writable

for required_tool in go tail base64 tr; do
  command -v "$required_tool" >/dev/null 2>&1 || \
    fail toolchain_unavailable "${required_tool}_not_found"
done
go version >/dev/null 2>&1 || fail toolchain_unavailable go_version_failed

cat > /work/project/go.mod <<'EOF'
module example.invalid/synthetic/probe

go 1.26
EOF

cat > /work/project/probe.go <<'EOF'
package probe

func Add(a, b int) int { return a + b }
EOF

cat > /work/project/probe_test.go <<'EOF'
package probe

import "testing"

func TestAdd(t *testing.T) {
	if Add(20, 22) != 42 {
		t.Fatal("synthetic arithmetic invariant failed")
	}
}
EOF

cd /work/project
if ! go list -deps ./... >/work/dependency-output.txt 2>&1; then
  fail_with_diagnostic dependency_disallowed offline_dependency_resolution_failed /work/dependency-output.txt
fi

if ! go test -c -o /work/project/probe.test . >/work/compile-output.txt 2>&1; then
  fail_with_diagnostic compile_rejected synthetic_compile_failed /work/compile-output.txt
fi

if ! /work/project/probe.test -test.v >/work/test-output.txt 2>&1; then
  fail_with_diagnostic test_failed synthetic_go_test_failed /work/test-output.txt
fi

cat /work/test-output.txt
emit passed synthetic_go_test_passed
