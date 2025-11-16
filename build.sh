#!/bin/bash

PROGRAM_NAME="nfs_mounter_agent"
VERSION="0.1.0"
DIST_DIR=".dist"
LOG_DIR="$DIST_DIR/logs"

mkdir -p "$DIST_DIR" "$LOG_DIR"

declare -a os_array=("linux") # "darwin" "windows")
declare -a arch_array=("amd64") # "arm64")


for OS in "${os_array[@]}"; do
    for ARCH in "${arch_array[@]}"; do
        FULL_NAME="${PROGRAM_NAME}_${VERSION}.${OS}-${ARCH}"
        mkdir -p "${DIST_DIR}/${FULL_NAME}"
        DIST_PATH="${DIST_DIR}/${FULL_NAME}/${PROGRAM_NAME}"
        echo "build $DIST_PATH"
        if GOOS=$OS GOARCH=$ARCH go build -o "$DIST_PATH" -ldflags="-X 'main.ProgramVersion=${VERSION}'" >> "${LOG_DIR}/${PROGRAM_NAME}.build.log"; then
            sha256sum "$DIST_PATH"  | awk '{print $1}' > "${DIST_DIR}/${FULL_NAME}.sum.txt"
        fi
        tar -czvf "${DIST_DIR}/${FULL_NAME}.tar.tgz" -C "$DIST_DIR" "$FULL_NAME"
    done
done


