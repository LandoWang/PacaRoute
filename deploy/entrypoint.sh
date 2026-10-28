#!/bin/sh
# ---------------------------------------------------------------------------
# copilot-bridge container entrypoint.
#
# The token store derives its AES key from /etc/machine-id (+ username). A fresh
# container image has an empty /etc/machine-id, and the bridge FAILS CLOSED if it
# can't read a stable machine identity — so we must seed one before `serve`.
#
# IMPORTANT: the machine-id is part of the encryption key. If you persist the
# token file (github_token.dat) across restarts to avoid re-authenticating, the
# machine-id MUST be identical on every start or the saved token won't decrypt.
# Pass a fixed value via COPILOT_BRIDGE_MACHINE_ID to guarantee that; otherwise
# we fall back to dbus-uuidgen, which is stable only within a single container's
# lifetime (fine for an ephemeral run where you re-auth each start).
# ---------------------------------------------------------------------------
set -eu

if [ ! -s /etc/machine-id ]; then
    if [ -n "${COPILOT_BRIDGE_MACHINE_ID:-}" ]; then
        printf '%s\n' "$COPILOT_BRIDGE_MACHINE_ID" > /etc/machine-id
        echo "[entrypoint] seeded /etc/machine-id from COPILOT_BRIDGE_MACHINE_ID"
    else
        dbus-uuidgen > /etc/machine-id
        echo "[entrypoint] seeded /etc/machine-id via dbus-uuidgen (ephemeral;" \
             "set COPILOT_BRIDGE_MACHINE_ID to persist a saved token across restarts)"
    fi
fi

exec /app/copilot-bridge "$@"
