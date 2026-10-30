#!/usr/bin/env bash
# One-time: the GPG key releases are signed with, the one the Terraform Registry
# is told about. Its passphrase is random and lives only in the macOS keychain
# (service "terraform-provider-nuclock-gpg"); nothing here prints it or the key.
# Leaves the PUBLIC key in signing-key.asc — that file is what the Registry wants.
set -euo pipefail

uid="terraform-provider-nuclock releases <kolosyuk1@gmail.com>"
service="terraform-provider-nuclock-gpg"

if gpg --list-secret-keys "$uid" >/dev/null 2>&1; then
  echo "A signing key for $uid already exists."
else
  passphrase=$(openssl rand -base64 32)
  security add-generic-password -U -a "$USER" -s "$service" -w "$passphrase"
  gpg --batch --pinentry-mode loopback --passphrase-fd 0 \
    --quick-generate-key "$uid" rsa4096 sign 0 <<<"$passphrase"
  unset passphrase
  echo "Signing key made; its passphrase is in the keychain as $service."
fi

gpg --armor --export "$uid" > signing-key.asc
gpg --list-keys --keyid-format long "$uid" | sed -n 's/^pub *//p'
echo "Public key written to signing-key.asc."
