#!/usr/bin/env bash
set -euo pipefail

#############################################
# SiteSnap Debian Package Builder
#
# Usage:
#   ./build-deb.sh 1.0.0
#############################################

APP="sitesnap"
VERSION="${1:-}"

if [[ -z "$VERSION" ]]; then
    echo "Usage: ./build-deb.sh <version>"
    exit 1
fi

ARCH="amd64"
MAINTAINER="Shivendra Rajan"
EMAIL="your@email.com"

PKGDIR="${APP}_${VERSION}_${ARCH}"

echo "==> Cleaning old build..."
rm -rf "$PKGDIR"

echo "==> Building binary..."
CGO_ENABLED=0 go build -ldflags="-s -w" -o "$APP"

echo "==> Creating package structure..."

mkdir -p "$PKGDIR/DEBIAN"
mkdir -p "$PKGDIR/usr/local/bin"
mkdir -p "$PKGDIR/usr/share/doc/$APP"

cp "$APP" "$PKGDIR/usr/local/bin/"
cp README.md "$PKGDIR/usr/share/doc/$APP/"

cat > "$PKGDIR/DEBIAN/control" <<EOF
Package: $APP
Version: $VERSION
Section: utils
Priority: optional
Architecture: $ARCH
Maintainer: $MAINTAINER <$EMAIL>
Description: Concurrent website snapshot and deployment verification tool.
 SiteSnap crawls websites, creates snapshots, validates deployments,
 detects regressions, and compares changes across releases.
EOF

chmod 755 "$PKGDIR/usr/local/bin/$APP"

echo "==> Building package..."

dpkg-deb --build "$PKGDIR"

echo
echo "Package created:"
echo
echo "  ${PKGDIR}.deb"
echo