#!/bin/bash
set -e

echo "Setting up Alpine rootfs..."
mkdir -p rootfs
cd rootfs

echo "Downloading Alpine Linux minirootfs..."
curl -o alpine.tar.gz -L https://dl-cdn.alpinelinux.org/alpine/v3.18/releases/x86_64/alpine-minirootfs-3.18.4-x86_64.tar.gz

echo "Extracting..."
tar xf alpine.tar.gz
rm alpine.tar.gz

echo "Rootfs ready in ./rootfs"
