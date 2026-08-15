#!/usr/bin/env -S nix shell nixpkgs#bash nixpkgs#apkeep nixpkgs#unzip nixpkgs#jadx --command bash
# shellcheck shell=bash
set -euo pipefail

# Download the VivaGym Group APK (package com.myvitale.vivagym.group) from the
# APKPure mirror into apk/ (no Google Play credentials required), then extract
# the base APK and decompile it with jadx.
#
# Run with: scripts/download-apk.sh
#
# Dependencies (bash, apkeep, unzip, jadx) are provided via the nix shell
# shebang, so the script is self-contained as long as `nix` is on the PATH.
#
# Outputs:
#   apk/com.myvitale.vivagym.group.xapk   split bundle from APKPure
#   apk/com.myvitale.vivagym.group.apk    extracted base APK
#   apk/jadx-out/sources/                 decompiled Java sources
#
# Notes:
# - jadx is a large nix derivation (pulls a Python toolchain); the first run
#   builds it, subsequent runs are cached.

PKG_ID="com.myvitale.vivagym.group"
SOURCE="apk-pure"
OUT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/apk"

mkdir -p "${OUT_DIR}"

echo "Downloading ${PKG_ID} from ${SOURCE}..."
apkeep -a "${PKG_ID}" -d "${SOURCE}" "${OUT_DIR}"

XAPK="${OUT_DIR}/${PKG_ID}.xapk"
APK="${OUT_DIR}/${PKG_ID}.apk"
JADX_OUT="${OUT_DIR}/jadx-out"

echo "Extracting base APK from ${XAPK}..."
unzip -o -q "${XAPK}" "${PKG_ID}.apk" -d "${OUT_DIR}"

echo "Decompiling ${APK} into ${JADX_OUT}..."
jadx --no-res -d "${JADX_OUT}" "${APK}"

echo "Done. Sources in ${JADX_OUT}/sources"
