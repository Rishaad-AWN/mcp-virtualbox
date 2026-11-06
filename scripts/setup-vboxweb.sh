#!/usr/bin/env bash
# Set up and start the VirtualBox web service so vm_mouse_click can send
# absolute mouse events. Safe to re-run.
set -euo pipefail

HOST="${VBOXWEB_HOST:-127.0.0.1}"
PORT="${VBOXWEB_PORT:-18083}"

if ! command -v vboxwebsrv >/dev/null 2>&1; then
  echo "vboxwebsrv not found on PATH. Install the VirtualBox package that ships it." >&2
  exit 1
fi

echo "Configuring anonymous logon (websrvauthlibrary=null)..."
VBoxManage setproperty websrvauthlibrary null

if pgrep -f "vboxwebsrv" >/dev/null 2>&1; then
  echo "vboxwebsrv already running."
else
  echo "Starting vboxwebsrv on ${HOST}:${PORT}..."
  nohup vboxwebsrv --host "$HOST" --port "$PORT" >/tmp/vboxwebsrv.log 2>&1 &
  sleep 1
  echo "Started (log: /tmp/vboxwebsrv.log)."
fi

echo "Done. mcp-virtualbox will use http://${HOST}:${PORT}/ for mouse control."
echo "Set VBOXWEB_ENDPOINT if you use a different host/port."
