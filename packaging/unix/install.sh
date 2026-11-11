#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$SCRIPT_DIR"

if [ ! -x "./netferry" ]; then
  echo "NetFerry executable not found or not executable in this folder." >&2
  exit 1
fi

echo "NetFerry installer"
echo
echo "Checking known local proxy ports..."
./netferry detect || true
echo
echo "Previewing installation changes..."
./netferry setup --dry-run
echo

printf "Install NetFerry for the current user? [y/N] "
read -r answer
case "$answer" in
  y|Y|yes|YES)
    ;;
  *)
    echo "Installation cancelled."
    exit 0
    ;;
esac

./netferry setup
echo
echo "NetFerry installation completed."
echo "Sign out and back in so new desktop sessions receive the environment."
echo
./netferry status || true
