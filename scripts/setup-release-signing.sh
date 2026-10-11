#!/usr/bin/env bash
# Creates the GPG key that signs provider releases and stores it as the repository secrets
# the Release workflow reads (GPG_PRIVATE_KEY, PASSPHRASE). Run it once, and again to rotate.
#
# The private key and the passphrase are never written to disk outside a temporary GnuPG
# home that is removed on exit, and never printed: they are piped straight into `gh secret
# set`. The public key is printed at the end, for the Terraform Registry.
#
# Needs gpg 2.1+, and gh signed in with admin rights on the repository.
#
#   scripts/setup-release-signing.sh [owner/repo]
set -euo pipefail

repo="${1:-segfaultpw/terraform-provider-sreagent}"
uid="SRE Agent Terraform Provider Releases <releases@sreagent.app>"

for tool in gpg gh; do
  command -v "$tool" >/dev/null || { echo "$tool is not installed" >&2; exit 1; }
done
gh auth status >/dev/null 2>&1 || { echo "gh is not signed in (gh auth login)" >&2; exit 1; }

home=$(mktemp -d)
chmod 700 "$home"
trap 'gpgconf --homedir "$home" --kill all >/dev/null 2>&1 || true; rm -rf "$home"' EXIT
export GNUPGHOME="$home"

pass=$(head -c 48 /dev/urandom | base64 | tr -d '\n/+=')

gpg --batch --pinentry-mode loopback --passphrase "$pass" \
  --quick-gen-key "$uid" rsa4096 sign never 2>/dev/null

fpr=$(gpg --list-secret-keys --with-colons | awk -F: '/^fpr/ {print $10; exit}')
[ -n "$fpr" ] || { echo "key generation failed" >&2; exit 1; }

gpg --batch --pinentry-mode loopback --passphrase "$pass" --armor --export-secret-keys "$fpr" |
  gh secret set GPG_PRIVATE_KEY --repo "$repo"
printf '%s' "$pass" | gh secret set PASSPHRASE --repo "$repo"

echo "Set GPG_PRIVATE_KEY and PASSPHRASE on $repo for key $fpr."
echo
echo "Add this public key on registry.terraform.io (User Settings, Signing Keys, namespace"
echo "segfaultpw), then publish the provider once. Later releases publish on their own."
echo
gpg --armor --export "$fpr"
