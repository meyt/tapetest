#!/usr/bin/env bash
#
# run-and-docs.sh — deterministic tapetest "run tests + regenerate OpenAPI docs" pipeline.
#
# tapetest records request/response exchanges inside TestMain (EnableRecording) and
# regenerates docs there too (GenerateDocs). This script makes that whole flow reproducible
# from the command line so it can be wired into CI, Makefiles, or `make docs`.
#
# Usage:
#   ./skills/tapetest/run-and-docs.sh [PKG]   # PKG defaults to "./..."
#
# Environment overrides:
#   TAPETEST_RECORDING_DIR  default ".tapetest"
#   TAPETEST_OUTPUT_DIR     default "docs"
#   TAPETEST_OPEN           set to "1" to open the generated UI in a browser (macOS/Linux)
#
# Exit codes: non-zero if `go test` fails OR if the expected artifacts are missing.
set -euo pipefail

PKG="${1:-./}"
RECORDING_DIR="${TAPETEST_RECORDING_DIR:-.tapetest}"
OUTPUT_DIR="${TAPETEST_OUTPUT_DIR:-docs}"

log() { printf '\033[1;34m[tapetest]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[tapetest] error:\033[0m %s\n' "$*" >&2; exit 1; }

log "cleaning stale recordings: ${RECORDING_DIR}"
rm -rf "${RECORDING_DIR}"
rm -rf "${OUTPUT_DIR}"

log "running tests (recording enabled in TestMain): go test ${PKG}"
if ! go test -v "${PKG}"; then
  die "go test failed; docs were not generated"
fi

log "verifying generated artifacts"
[ -f "${OUTPUT_DIR}/openapi.json" ] || die "missing ${OUTPUT_DIR}/openapi.json — is GenerateDocs wired into TestMain?"
[ -f "${OUTPUT_DIR}/index.html" ]   || die "missing ${OUTPUT_DIR}/index.html"

if ! grep -q '"openapi"' "${OUTPUT_DIR}/openapi.json" >/dev/null 2>&1; then
  die "${OUTPUT_DIR}/openapi.json does not look like an OpenAPI document"
fi

RECORDINGS="${RECORDING_DIR}/recordings.json"
if [ -f "${RECORDINGS}" ]; then
  COUNT=$(grep -c '"test"' "${RECORDINGS}" || true)
  log "recorded ${COUNT} exchanges from ${RECORDINGS}"
else
  log "note: ${RECORDINGS} not found (recording may not have been enabled)"
fi

log "docs generated successfully:"
log "  spec : ${OUTPUT_DIR}/openapi.json"
log "  UI   : ${OUTPUT_DIR}/index.html"

if [ "${TAPETEST_OPEN:-0}" = "1" ]; then
  if command -v xdg-open >/dev/null 2>&1; then xdg-open "${OUTPUT_DIR}/index.html"
  elif command -v open >/dev/null 2>&1; then open "${OUTPUT_DIR}/index.html"
  else die "no 'open'/'xdg-open' command found to launch the browser"; fi
fi
