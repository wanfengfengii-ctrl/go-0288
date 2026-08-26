#!/usr/bin/env bash
# Smoke test for the benzhi backend. It builds the server, starts it on a local
# port, probes the health endpoint and the public pile API, then cleans up every
# process and temporary file. It runs entirely offline and finishes
# deterministically (no go test, no network service dependencies).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKDIR="$(mktemp -d)"
PORT="${PORT:-18080}"
ADDR="127.0.0.1:${PORT}"
SERVER_PID=""

cleanup() {
  if [[ -n "${SERVER_PID}" ]] && kill -0 "${SERVER_PID}" 2>/dev/null; then
    kill "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
  fi
  rm -rf "${WORKDIR}"
}
trap cleanup EXIT

echo "building server"
(
  cd "${ROOT}"
  go build -o "${WORKDIR}/server" ./cmd/server
)

echo "starting server on ${ADDR}"
DB_PATH="${WORKDIR}/smoke.db" ADDR="${ADDR}" "${WORKDIR}/server" &
SERVER_PID=$!

# Wait for the health endpoint to come up (bounded, deterministic retries).
ready=""
for _ in $(seq 1 50); do
  if curl -sf "http://${ADDR}/healthz" > "${WORKDIR}/health.json" 2>/dev/null; then
    ready="1"
    break
  fi
  sleep 0.1
done
if [[ -z "${ready}" ]]; then
  echo "server did not become ready" >&2
  exit 1
fi

# Assert health reports ok (capture response in a variable, not grep on a pipe).
health_body="$(cat "${WORKDIR}/health.json")"
if [[ "${health_body}" != *'"status":"ok"'* || "${health_body}" != *'"db":"ok"'* ]]; then
  echo "unexpected health response: ${health_body}" >&2
  exit 1
fi
echo "health ok: ${health_body}"

# Exercise a real public API command: create a pile task.
CREATE_BODY='{
  "Pier":"S1","PileNo":"3","Summary":"S1-3 smoke pile",
  "DesignDepth":10000,"Diameter":1000,
  "Layers":[{"Name":"soil","Start":0,"End":5000},{"Name":"rock","Start":5000,"End":10000}],
  "Rebar":[{"Index":0,"Start":0,"End":10000,"Direction":1}],
  "Sonic":[{"ID":"S1","Start":0,"End":10000,"Neighbors":["S2"]},{"ID":"S2","Start":0,"End":10000,"Neighbors":["S1"]}],
  "Mud":{"SpecificGravityMin":1100000,"SpecificGravityMax":1300000,"ViscosityMin":1800000,"ViscosityMax":2500000,"SandContentMax":40000},
  "Cleaning":{"SedimentMax":300,"ApertureTolerance":100},
  "Pour":{"FirstPourVolume":2000,"ContinuousMaxGap":100,"MinEmbedment":400,"MaxEmbedment":6000},
  "Overpour":500,
  "LineAdjacency":[["S1","S2"]],
  "Coring":{"MinCoresPerAnomaly":1,"CoreDepthStep":1000},
  "AgePeriod":10,
  "MaxRetries":3
}'

CREATE_CODE="$(curl -s -o "${WORKDIR}/create.json" -w '%{http_code}' \
  -X POST -H 'Content-Type: application/json' \
  -d "${CREATE_BODY}" "http://${ADDR}/v1/piles")"
if [[ "${CREATE_CODE}" != "201" ]]; then
  echo "create pile returned ${CREATE_CODE}: $(cat "${WORKDIR}/create.json")" >&2
  exit 1
fi
create_body="$(cat "${WORKDIR}/create.json")"
echo "create pile ok: ${create_body}"

echo "smoke test passed"
