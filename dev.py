#!/usr/bin/env python3
"""Runs the API locally with live reload (`air`), with the development environment variables set (admin API and panel on).
Entry points: dev.bat / dev.sh. Needs `air` on PATH (go install github.com/air-verse/air@latest) and the Firebase
admin SDK key file next to this script. Variables already set in your shell are kept."""
import os
import shutil
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
KEY_FILE = os.path.join(HERE, "additive-idle-simulation-firebase-adminsdk-fbsvc-ac30a17c67.json")


def main():
    air = shutil.which("air")
    if not air:
        print("ERROR: `air` is not on PATH.", file=sys.stderr)
        return 1
    env = dict(os.environ)
    env.setdefault("GOOGLE_APPLICATION_CREDENTIALS", KEY_FILE)
    env.setdefault("APP_ENV", "development")
    env.setdefault("APP_PORT", "3000")
    # The admin API and the embedded panel (http://localhost:3000/admin) are off by default (config.yaml); on for local runs
    env.setdefault("ADMIN_ENABLED", "true")
    env.setdefault("ADMIN_PANEL_ENABLED", "true")
    # .air.toml builds tmp/main.exe (Windows); its entrypoint is tmp/main, which does not exist on macOS / Linux
    exe = "./tmp/main.exe" if os.name == "nt" else "./tmp/main"
    build = ["--build.cmd", f"go build -o {exe} ./internal/cmd/gcp_cloud_run", "--build.bin", exe, "--build.entrypoint", exe]
    return subprocess.call([air] + build + sys.argv[1:], cwd=HERE, env=env)


if __name__ == "__main__":
    sys.exit(main())
