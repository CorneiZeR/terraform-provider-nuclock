#!/usr/bin/env bash
# One-time, after the GitHub repository exists: hands the release workflow the
# signing key and its passphrase as repository secrets. Both go straight from
# gpg and the keychain into `gh`, without passing through a file or a screen.
set -euo pipefail

uid="terraform-provider-nuclock releases <kolosyuk1@gmail.com>"
service="terraform-provider-nuclock-gpg"
repo="CorneiZeR/terraform-provider-nuclock"

security find-generic-password -a "$USER" -s "$service" -w | gh secret set PASSPHRASE -R "$repo"
security find-generic-password -a "$USER" -s "$service" -w \
  | gpg --batch --pinentry-mode loopback --passphrase-fd 0 --armor --export-secret-keys "$uid" \
  | gh secret set GPG_PRIVATE_KEY -R "$repo"
gh secret list -R "$repo"
