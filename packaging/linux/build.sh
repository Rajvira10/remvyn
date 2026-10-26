#!/usr/bin/env bash
# Builds dist/Remvyn-<version>-linux-x86_64.tar.gz inside Docker.
# Optional: REMVYN_GITHUB_CLIENT_ID / REMVYN_GITHUB_APP_SLUG to build in a different sync app.
set -euo pipefail
cd "$(dirname "$0")/../.."
VERSION=$(sed -n 's/^const Version = "\(.*\)"/\1/p' app.go)
docker build --platform linux/amd64 -t remvyn-linux-build packaging/linux
mkdir -p dist
LDFLAGS=""
[ -n "${REMVYN_GITHUB_CLIENT_ID:-}" ] && LDFLAGS+=" -X main.githubClientID=$REMVYN_GITHUB_CLIENT_ID"
[ -n "${REMVYN_GITHUB_APP_SLUG:-}" ] && LDFLAGS+=" -X main.githubAppSlug=$REMVYN_GITHUB_APP_SLUG"
docker run --rm --platform linux/amd64 \
  -v "$PWD":/repo:ro -v "$PWD/dist":/out -v remvyn-gomod:/root/go/pkg/mod -v remvyn-npm:/root/.npm \
  -e LDFLAGS="$LDFLAGS" -e VERSION="$VERSION" remvyn-linux-build bash -euo pipefail -c '
    mkdir /src && cd /repo
    tar --exclude=./frontend/node_modules --exclude=./mobile --exclude=./dist --exclude=./build/bin --exclude=./.git -cf - . | tar -xf - -C /src
    cd /src
    wails build -tags webkit2_41 -platform linux/amd64 -clean -ldflags "$LDFLAGS"
    pkg=/tmp/Remvyn-$VERSION-linux-x86_64 && mkdir -p $pkg
    install -m755 build/bin/Remvyn $pkg/remvyn
    install -m644 build/appicon.png $pkg/remvyn.png
    install -m644 packaging/linux/remvyn.desktop packaging/linux/README.txt $pkg/
    install -m755 packaging/linux/install.sh $pkg/
    tar -C /tmp -czf /out/Remvyn-$VERSION-linux-x86_64.tar.gz Remvyn-$VERSION-linux-x86_64
    echo "built /out/Remvyn-$VERSION-linux-x86_64.tar.gz"
  '
