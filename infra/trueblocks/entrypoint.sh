#!/bin/sh
set -eu

data_dir=/var/lib/trueblocks
config_dir="$data_dir/config"
config_file="$config_dir/trueBlocks.toml"
mode="${TRUEBLOCKS_INIT_MODE:-filters}"

case "$mode" in
  filters) init_args= ;;
  all) init_args=--all ;;
  *) echo "TRUEBLOCKS_INIT_MODE must be filters or all" >&2; exit 1 ;;
esac

: "${TRUEBLOCKS_RPC_URL:?TRUEBLOCKS_RPC_URL is required}"
mkdir -p "$config_dir/config/mainnet" "$data_dir/cache" "$data_dir/unchained"
envsubst '${TRUEBLOCKS_RPC_URL}' < /opt/openchain/trueBlocks.toml.template > "$config_file"

marker="$data_dir/.openchain-initialized-$mode"
if [ ! -f "$marker" ]; then
  chifra init $init_args
  touch "$marker"
fi

exec chifra daemon --url 0.0.0.0:8080
