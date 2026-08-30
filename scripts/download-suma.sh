#!/usr/bin/env bash
# shellcheck shell=bash
set -euo pipefail

# If we're not yet inside a nix shell that provides the tools, re-exec
# ourselves through one. (The `#!/usr/bin/env -S nix shell ...` shebang does
# not work when the kernel execs the script directly on macOS, so we wrap it.)
if ! command -v apkeep >/dev/null 2>&1 || ! command -v jadx >/dev/null 2>&1; then
    exec nix shell nixpkgs#bash nixpkgs#apkeep nixpkgs#unzip nixpkgs#jadx \
        --command bash "$0" "$@"
fi

# Download the RecargaSUMA APK (package com.transermobile.recargasuma) from the
# APKPure mirror into apk/recargasuma/ (no Google Play credentials required),
# extract the base APK, and decompile it with jadx.
#
# Run with: scripts/download-suma.sh
#
# Dependencies (bash, apkeep, unzip, jadx) are provided on demand by
# re-execing this script through a `nix shell`, so it is self-contained as
# long as `nix` is on the PATH.
#
# Outputs:
#   apk/recargasuma/com.transermobile.recargasuma.xapk   split bundle from APKPure
#   apk/recargasuma/com.transermobile.recargasuma.apk    extracted base APK
#   apk/recargasuma/apk-extract/                         raw APK contents (manifest,
#                                                        native libs, assets)
#   apk/recargasuma/jadx-out/sources/                    decompiled Java sources
#
# Notes:
# - jadx is a large nix derivation (pulls a Python toolchain); the first run
#   builds it, subsequent runs are cached.

PKG_ID="com.transermobile.recargasuma"
SOURCE="apk-pure"
OUT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/apk/recargasuma"

mkdir -p "${OUT_DIR}"

echo "Downloading ${PKG_ID} from ${SOURCE}..."
apkeep -a "${PKG_ID}" -d "${SOURCE}" "${OUT_DIR}"

XAPK="${OUT_DIR}/${PKG_ID}.xapk"
APK="${OUT_DIR}/${PKG_ID}.apk"
EXTRACT="${OUT_DIR}/apk-extract"
JADX_OUT="${OUT_DIR}/jadx-out"

if [ -f "${XAPK}" ]; then
    echo "Extracting base APK from ${XAPK}..."
    unzip -o -q "${XAPK}" "${PKG_ID}.apk" -d "${OUT_DIR}"
fi

echo "Extracting raw APK contents into ${EXTRACT}..."
unzip -o -q "${APK}" -d "${EXTRACT}"

echo "Decompiling ${APK} into ${JADX_OUT}..."
jadx --no-res -d "${JADX_OUT}" "${APK}"

echo "Done. Sources in ${JADX_OUT}/sources"