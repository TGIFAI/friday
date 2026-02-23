#!/bin/bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MACOS_DIR="${REPO_ROOT}/macos"
RESOURCES_DIR="${MACOS_DIR}/Resources"
VERSION="${VERSION:-dev}"
GIT_SHA="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo "unknown")"
LDFLAGS="-s -w -X github.com/tgifai/friday.VERSION=${VERSION} -X github.com/tgifai/friday.MAGIC=${GIT_SHA}"

echo "==> Building Go binary (Universal)"
mkdir -p "$RESOURCES_DIR"

# 1. Build arm64
echo "  -> darwin/arm64"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
  -trimpath -ldflags="${LDFLAGS}" \
  -o "${RESOURCES_DIR}/friday-core-arm64" \
  "${REPO_ROOT}/cmd/friday"

# 2. Build amd64
echo "  -> darwin/amd64"
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
  -trimpath -ldflags="${LDFLAGS}" \
  -o "${RESOURCES_DIR}/friday-core-amd64" \
  "${REPO_ROOT}/cmd/friday"

# 3. Universal binary
echo "  -> lipo (universal)"
lipo -create \
  "${RESOURCES_DIR}/friday-core-arm64" \
  "${RESOURCES_DIR}/friday-core-amd64" \
  -output "${RESOURCES_DIR}/friday-core"
chmod +x "${RESOURCES_DIR}/friday-core"
rm -f "${RESOURCES_DIR}/friday-core-arm64" "${RESOURCES_DIR}/friday-core-amd64"

# 4. Bundle skills if available
if [ -d "${REPO_ROOT}/skills" ]; then
  echo "==> Bundling skills"
  rm -rf "${RESOURCES_DIR}/skills"
  cp -r "${REPO_ROOT}/skills" "${RESOURCES_DIR}/skills"
fi

# 5. Generate Xcode project
echo "==> Generating Xcode project (xcodegen)"
if ! command -v xcodegen &>/dev/null; then
  echo "error: xcodegen not found. Install with: brew install xcodegen"
  exit 1
fi
cd "$MACOS_DIR"
xcodegen generate --quiet

# 6. Build app
echo "==> Building Friday.app"
MKT_VERSION="${VERSION#v}"
xcodebuild \
  -project Friday.xcodeproj \
  -scheme Friday \
  -configuration Release \
  -derivedDataPath build/DerivedData \
  CODE_SIGN_IDENTITY="-" \
  DEVELOPMENT_TEAM="" \
  MARKETING_VERSION="${MKT_VERSION}" \
  2>&1 | tail -5

# 7. Copy result
APP_PATH="build/DerivedData/Build/Products/Release/Friday.app"
if [ -d "$APP_PATH" ]; then
  echo "==> Success: ${MACOS_DIR}/${APP_PATH}"
  echo "    Run: open '${MACOS_DIR}/${APP_PATH}'"
else
  echo "==> Build output not found at expected path"
  find build/DerivedData -name "Friday.app" -type d 2>/dev/null || true
fi
