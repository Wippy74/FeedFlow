#!/bin/sh
set -eu
umask 077

task_key_dir="${1:-.secrets/jwt}"
if [ -e "$task_key_dir/private.pem" ] || [ -L "$task_key_dir/private.pem" ] || [ -e "$task_key_dir/public.pem" ] || [ -L "$task_key_dir/public.pem" ]; then
    echo "Refusing to overwrite existing JWT keys." >&2
    exit 1
fi

mkdir -p "$task_key_dir"
openssl genpkey -algorithm ED25519 -out "$task_key_dir/private.pem"
openssl pkey -in "$task_key_dir/private.pem" -pubout -out "$task_key_dir/public.pem"
echo "Development JWT keys created. Keep private.pem only in the API issuer."
