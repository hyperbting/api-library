#!/usr/bin/env python3
"""Runs the API locally with live reload (`air`), with the development environment variables set (admin API and panel on).
Entry points: dev.bat / dev.sh. Needs `air` on PATH (go install github.com/air-verse/air@latest) and the Firebase
admin SDK key file next to this script. Variables already set in your shell are kept. The panel's Firebase Web values
(ADMIN_WEB_*) and Google sign-in are copied from the deployed admin panel's public config, so the local panel can sign in."""
import json
import os
import shutil
import subprocess
import sys
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
KEY_FILE = os.path.join(HERE, "additive-idle-simulation-firebase-adminsdk-fbsvc-ac30a17c67.json")

# The deployed admin panel's public config: the Firebase Web app values the local panel needs to sign in. They are
# public (every browser that opens the panel downloads them), but read from the live service instead of written here.
PANEL_CONFIG_URL = "https://api-library-admin-794820781634.us-central1.run.app/admin/config.json"


def panel_web_config():
    """ADMIN_WEB_* and ADMIN_LOGIN_GOOGLE from the deployed admin panel, or {} when it does not answer."""
    url = os.environ.get("ADMIN_WEB_CONFIG_URL", PANEL_CONFIG_URL)
    try:
        with urllib.request.urlopen(url, timeout=5) as res:
            cfg = json.load(res)
    except Exception as e:
        print(f"dev.py: could not read the panel's web config from {url} ({e}); the local panel cannot sign in.", file=sys.stderr)
        return {}
    web = cfg.get("firebase") or {}
    values = {
        "ADMIN_WEB_API_KEY": web.get("apiKey") or "",
        "ADMIN_WEB_AUTH_DOMAIN": web.get("authDomain") or "",
        "ADMIN_WEB_APP_ID": web.get("appId") or "",
    }
    values = {k: v for k, v in values.items() if v}
    if (cfg.get("login") or {}).get("google"):
        values["ADMIN_LOGIN_GOOGLE"] = "true"
    return values


def find_air():
    """`air` on PATH, else in the Go bin folder (go install puts it there, and that folder is often not on PATH)."""
    found = shutil.which("air")
    if found:
        return found
    gobin = os.environ.get("GOBIN") or os.path.join(os.environ.get("GOPATH") or os.path.expanduser("~/go"), "bin")
    return shutil.which("air", path=gobin)


def main():
    air = find_air()
    if not air:
        print("ERROR: `air` not found on PATH or in the Go bin folder (go install github.com/air-verse/air@latest).", file=sys.stderr)
        return 1
    env = dict(os.environ)
    env.setdefault("GOOGLE_APPLICATION_CREDENTIALS", KEY_FILE)
    env.setdefault("APP_ENV", "development")
    env.setdefault("APP_PORT", "3000")
    # The admin API and the embedded panel (http://localhost:3000/admin) are off by default (config.yaml); on for local runs
    env.setdefault("ADMIN_ENABLED", "true")
    env.setdefault("ADMIN_PANEL_ENABLED", "true")
    # So the local panel can sign in like the deployed one (email, and Google when the live panel offers it).
    # localhost is an authorized domain by default; set any of these in the shell to override, or
    # ADMIN_PANEL_ENABLED=false to run the API without the panel (then nothing is fetched).
    if env.get("ADMIN_PANEL_ENABLED", "").lower() == "true":
        for name, value in panel_web_config().items():
            env.setdefault(name, value)
    # .air.toml builds tmp/main.exe (Windows); its entrypoint is tmp/main, which does not exist on macOS / Linux
    exe = "./tmp/main.exe" if os.name == "nt" else "./tmp/main"
    build = ["--build.cmd", f"go build -o {exe} ./internal/cmd/gcp_cloud_run", "--build.bin", exe, "--build.entrypoint", exe]
    return subprocess.call([air] + build + sys.argv[1:], cwd=HERE, env=env)


if __name__ == "__main__":
    sys.exit(main())
