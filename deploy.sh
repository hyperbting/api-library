#!/usr/bin/env sh
# Entry point; the logic is in deploy.py (cross-OS Python).
exec python3 "$(dirname "$0")/deploy.py" public "$@"
