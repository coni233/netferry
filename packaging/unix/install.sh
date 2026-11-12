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
echo "Start your proxy app before continuing."
echo "You do not need to turn on its system proxy setting."

SETUP_FLAGS=""
while :; do
  echo
  echo "Checking local proxy ports..."
  if ./netferry detect; then
    break
  fi
  echo
  echo "No local proxy detected."
  echo "Start your proxy app, then press R to retry."
  echo "You can also run setup manually (replace the port with your proxy port):"
  echo "  ./netferry setup --upstream http://127.0.0.1:10808"
  printf "Press R to retry, S to skip detection, or Q to quit: "
  read -r answer
  case "$answer" in
    r|R)
      continue
      ;;
    s|S)
      SETUP_FLAGS="--no-detect"
      echo "Continuing with the default upstream."
      echo "You can update it later with:"
      echo "  ./netferry setup --upstream http://127.0.0.1:10808"
      break
      ;;
    q|Q)
      echo "Installation cancelled. Start your proxy app and run install.sh again."
      exit 1
      ;;
  esac
done
echo
echo "Previewing installation changes..."
./netferry setup --dry-run ${SETUP_FLAGS}
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

./netferry setup ${SETUP_FLAGS}
echo
echo "NetFerry installation completed."
echo "Sign out and back in so new desktop sessions receive the environment."
echo
./netferry status || true
