#!/usr/bin/env bash
#
# make-signing-cert.sh — create the self-signed code-signing certificate that
# gives every orbit release the same designated requirement.
#
# Ad-hoc signatures ("-") are pinned to the binary's cdhash, so each update
# looks like a different app to macOS (TCC) and Full Disk Access has to be
# granted again. A release signed with one fixed certificate instead has the
# requirement `identifier "..." and certificate leaf = H"<cert hash>"`, which
# stays the same across versions as long as the certificate does.
#
# Run this ONCE and keep the output directory OUTSIDE the repository. Running
# it again would make a different certificate, and users would have to re-grant
# permissions once more, so the script refuses to overwrite an existing one.
#
# Usage:
#   scripts/make-signing-cert.sh "$HOME/Library/Application Support/mygo/orbit-signing"
#
# Writes to that directory (700), each file 600:
#   orbit-signing.p12            certificate + private key (password protected)
#   orbit-signing.p12.password   its password
#   orbit-signing.crt            the certificate alone (public)
#
# CI imports the .p12 from two repository secrets; the script prints the
# `gh secret set` commands.

set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "usage: $0 <output-dir outside the repo>" >&2
    exit 2
fi

name="orbit Self-Signed"
dir="$1"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "$dir"
dir="$(cd "$dir" && pwd)"
case "$dir/" in
    "$root"/*) echo "$0: $dir is inside the repository; pick a directory outside it" >&2; exit 2 ;;
esac

p12="$dir/orbit-signing.p12"
if [[ -e "$p12" ]]; then
    echo "$0: $p12 already exists; not overwriting (a new certificate resets users' permissions)" >&2
    exit 1
fi

chmod 700 "$dir"
umask 077
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

cat > "$work/openssl.cnf" <<CNF
[req]
distinguished_name = dn
x509_extensions = ext
prompt = no
[dn]
CN = $name
[ext]
basicConstraints = critical,CA:FALSE
keyUsage = critical,digitalSignature
extendedKeyUsage = critical,codeSigning
subjectKeyIdentifier = hash
CNF

password="$(openssl rand -base64 24)"
openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 3650 \
    -config "$work/openssl.cnf" -keyout "$work/key.pem" -out "$work/cert.pem" 2> /dev/null
# macOS `security import` cannot read OpenSSL 3's default AES/SHA-256 PKCS#12
# ("MAC verification failed"), so ask for the 3DES/SHA-1 form it understands.
openssl pkcs12 -export -name "$name" -inkey "$work/key.pem" -in "$work/cert.pem" \
    -certpbe PBE-SHA1-3DES -keypbe PBE-SHA1-3DES -macalg sha1 \
    -out "$p12" -passout "pass:$password"

printf '%s\n' "$password" > "$p12.password"
cp "$work/cert.pem" "$dir/orbit-signing.crt"
chmod 600 "$p12" "$p12.password" "$dir/orbit-signing.crt"

echo "created $p12 (CN=$name, valid 10 years)"
echo "SHA-256 fingerprint: $(openssl x509 -in "$work/cert.pem" -noout -fingerprint -sha256 | cut -d= -f2)"
echo
echo "Set the CI secrets (run these yourself, from this machine):"
echo "  base64 -i \"$p12\" | gh secret set MACOS_SIGNING_P12 --repo real-jacket/orbit"
echo "  gh secret set MACOS_SIGNING_P12_PASSWORD --repo real-jacket/orbit < \"$p12.password\""
