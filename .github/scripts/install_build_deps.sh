#!/bin/sh

set -e

DIR=$(cd -P -- "$(dirname -- "$(command -v -- "$0")")" && pwd -P)

cd "${DIR}/../.."

ARCH=$(uname -m)
if ! command -v yq >/dev/null 2>&1; then
	case $ARCH in
	"x86_64")
		URL="https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64"
		;;
	"aarch64")
		URL="https://github.com/mikefarah/yq/releases/latest/download/yq_linux_arm64"
		;;
	*)
		echo "Unknown architecture: $ARCH"
		exit 1
		;;
	esac
	BIN=/usr/local/bin/yq
	sudo curl -sSL "$URL" -o "$BIN"
	sudo chmod +x "$BIN"
fi
if ! command -v jq >/dev/null 2>&1; then
	sudo apt-get update
	sudo apt-get install jq -y
fi

YQ_VERSION=$(yq --version)
echo "Installed yq version: $YQ_VERSION"
