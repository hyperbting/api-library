#!/usr/bin/env sh
# Entry point; the logic is in dev.py (cross-OS Python).
exec python3 "$(dirname "$0")/dev.py" "$@"
