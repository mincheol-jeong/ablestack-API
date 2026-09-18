#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKAGE_NAME="ablestack-api"
VERSION_FILE="${VERSION_FILE:-${ROOT_DIR}/VERSION}"
CHANGELOG_FILE="${CHANGELOG_FILE:-${ROOT_DIR}/CHANGELOG.md}"
if [[ -z "${VERSION:-}" ]]; then
  if [[ ! -f "$VERSION_FILE" ]]; then
    echo "missing VERSION file: ${VERSION_FILE}" >&2
    exit 2
  fi
  IFS= read -r VERSION < "$VERSION_FILE" || true
  VERSION="${VERSION//$'\r'/}"
  VERSION="${VERSION//[[:space:]]/}"
fi
RELEASE="${RELEASE:-1}"
API_PORT="${API_PORT:-18090}"
DIST_DIR="${DIST_DIR:-${ROOT_DIR}/dist/rpm}"
RPMBUILD_DIR="${RPMBUILD_DIR:-${DIST_DIR}/rpmbuild}"
SPEC_FILE="${ROOT_DIR}/packaging/rpm/${PACKAGE_NAME}.spec"

if [[ -z "$VERSION" ]]; then
  echo "VERSION must not be empty" >&2
  exit 2
fi

if [[ "$VERSION" == *-* ]]; then
  echo "VERSION must be an RPM-compatible version without '-': ${VERSION}" >&2
  exit 2
fi

if [[ ! "$API_PORT" =~ ^[0-9]+$ ]] || (( 10#$API_PORT < 1 || 10#$API_PORT > 65535 )); then
  echo "API_PORT must be an integer between 1 and 65535: ${API_PORT}" >&2
  exit 2
fi
API_PORT="$((10#$API_PORT))"

if [[ ! -f "$CHANGELOG_FILE" ]]; then
  echo "missing CHANGELOG file: ${CHANGELOG_FILE}" >&2
  exit 2
fi

if ! grep -Fq "## [${VERSION}]" "$CHANGELOG_FILE"; then
  echo "CHANGELOG.md must contain a release section for VERSION ${VERSION}" >&2
  exit 2
fi

for command in rpmbuild go tar; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "missing required command: $command" >&2
    exit 127
  fi
done

if [[ -z "$RPMBUILD_DIR" || "$RPMBUILD_DIR" == "/" ]]; then
  echo "refusing unsafe RPMBUILD_DIR: ${RPMBUILD_DIR}" >&2
  exit 2
fi

rm -rf "$RPMBUILD_DIR"
mkdir -p "$RPMBUILD_DIR"/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

SOURCE_ROOT="${TMP_DIR}/${PACKAGE_NAME}-${VERSION}"
mkdir -p "$SOURCE_ROOT"

tar -C "$ROOT_DIR" \
  --exclude='.git' \
  --exclude='.DS_Store' \
  --exclude='dist' \
  --exclude='*.rpm' \
  --exclude='./internal/handler/cube/create_ccvm_cloudinit.go' \
  --exclude='./internal/handler/cube/create_scvm_cloudinit.go' \
  --exclude='./internal/handler/cube/create_ccvm_xml.go' \
  --exclude='./internal/handler/cube/create_scvm_xml.go' \
  --exclude='./internal/model/mold/status.go' \
  --exclude='./internal/service/controller/Neighbor.go' \
  --exclude='./internal/service/controller/config.go' \
  -cf - . | tar -C "$SOURCE_ROOT" -xf -

tar -C "$TMP_DIR" -czf "$RPMBUILD_DIR/SOURCES/${PACKAGE_NAME}-${VERSION}.tar.gz" "${PACKAGE_NAME}-${VERSION}"
cp "$SPEC_FILE" "$RPMBUILD_DIR/SPECS/"

RPMBUILD_ARGS=(
  --define "_topdir ${RPMBUILD_DIR}"
  --define "rpm_version ${VERSION}"
  --define "rpm_release ${RELEASE}"
  --define "api_port ${API_PORT}"
)

if [[ "${SKIP_TESTS:-0}" == "1" ]]; then
  RPMBUILD_ARGS+=(--without tests)
fi

rpmbuild "${RPMBUILD_ARGS[@]}" -ba "$RPMBUILD_DIR/SPECS/${PACKAGE_NAME}.spec"

echo "RPM output:"
find "$RPMBUILD_DIR/RPMS" "$RPMBUILD_DIR/SRPMS" -type f -print
