#!/usr/bin/env bash
# Builds a signed apt repository from .deb files.
#
#   build-repo.sh <dir-with-debs> <out-dir> <gpg-key-id>
#
# <dir-with-debs> is searched recursively. The result is served as-is:
#   deb [signed-by=/usr/share/keyrings/slat.gpg] <url-of-out-dir> stable main
# Needs dpkg-deb, apt-ftparchive (apt-utils) and gpg with the secret key.
set -euo pipefail

debs=$1 out=$2 key=$3
pool=pool/main/s/slat

mkdir -p "$out/$pool"
find "$debs" -name '*.deb' -print0 | while IFS= read -r -d '' f; do
  ver=$(dpkg-deb -f "$f" Version)
  arch=$(dpkg-deb -f "$f" Architecture)
  cp "$f" "$out/$pool/slat_${ver}_${arch}.deb"
done

cd "$out"
for arch in amd64 arm64; do
  dir=dists/stable/main/binary-$arch
  mkdir -p "$dir"
  apt-ftparchive --arch "$arch" packages pool > "$dir/Packages"
  gzip -9kf "$dir/Packages"
done

apt-ftparchive \
  -o APT::FTPArchive::Release::Origin=slat \
  -o APT::FTPArchive::Release::Label=slat \
  -o APT::FTPArchive::Release::Suite=stable \
  -o APT::FTPArchive::Release::Codename=stable \
  -o APT::FTPArchive::Release::Architectures="amd64 arm64" \
  -o APT::FTPArchive::Release::Components=main \
  -o APT::FTPArchive::Release::Description="slat terminal multiplexer" \
  release dists/stable > dists/stable/Release

gpg --batch --yes --local-user "$key" --clearsign -o dists/stable/InRelease dists/stable/Release
gpg --batch --yes --local-user "$key" --armor --detach-sign -o dists/stable/Release.gpg dists/stable/Release

# Public key for users: binary for signed-by=, armored for humans.
gpg --batch --yes --export "$key" > slat.gpg
gpg --batch --yes --armor --export "$key" > slat.asc
