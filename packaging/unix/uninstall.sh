#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$SCRIPT_DIR"

if [ ! -x "./netferry" ]; then
  echo "NetFerry executable not found or not executable in this folder." >&2
  exit 1
fi

echo "NetFerry uninstaller"
echo
echo "Previewing uninstall changes..."
./netferry uninstall --dry-run
echo

printf "Remove NetFerry startup and managed environment? [y/N] "
read -r answer
case "$answer" in
  y|Y|yes|YES)
    ;;
  *)
    echo "Uninstall cancelled."
    exit 0
    ;;
esac

./netferry uninstall
echo
echo "NetFerry uninstall completed."
echo "Restart Agent applications and terminals to clear inherited settings."
