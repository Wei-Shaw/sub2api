#!/usr/bin/env bash
# Certbot deploy hook: publish only this hostname's certificate to the app.
set -euo pipefail

source_dir=/etc/letsencrypt/live/direct.buaga.dpdns.org
destination=/opt/sub2api-return/direct-api-tls

install -d -o root -g 1000 -m 0750 "$destination"
stage="$(mktemp -d "$destination/version.XXXXXXXX")"
chown root:1000 "$stage"
chmod 0750 "$stage"
install -o root -g 1000 -m 0640 "$source_dir/fullchain.pem" "$stage/fullchain.pem"
install -o root -g 1000 -m 0640 "$source_dir/privkey.pem" "$stage/privkey.pem"
openssl x509 -in "$stage/fullchain.pem" -noout -checkend 86400 >/dev/null
cert_public_key="$(openssl x509 -in "$stage/fullchain.pem" -pubkey -noout | openssl pkey -pubin -outform DER | sha256sum | cut -d' ' -f1)"
key_public_key="$(openssl pkey -in "$stage/privkey.pem" -pubout -outform DER | sha256sum | cut -d' ' -f1)"
[[ "$cert_public_key" == "$key_public_key" ]] || { echo 'Direct API certificate/key mismatch' >&2; exit 1; }
ln -sfn "$(basename "$stage")" "$destination/.current-next"
mv -Tf "$destination/.current-next" "$destination/current"
