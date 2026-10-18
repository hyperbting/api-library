#!/usr/bin/env python3
"""Builds and deploys api-library to Cloud Run (Windows, macOS and Linux; standard library only).

  python deploy.py public    Build the image with Cloud Build, deploy `api-library` (game API only: no /admin routes,
                             no panel; ADMIN_* are forced off).
  python deploy.py admin     Deploy the SAME image (the digest `:latest` points at now) as `api-library-admin`: the
                             game API plus the /admin routes. It never builds: run `public` first, or use `all`.
  python deploy.py all       public, then admin (one build, two services).

  --dry-run                  Read-only lookups still run; the build and deploy commands are only printed.

The entry points deploy.bat / deploy-admin.bat / deploy-all.bat / deploy*.sh call this file.

Environment variables of the admin service are MERGED (`--update-env-vars`): only the ones set in your shell are
written, everything already on the service stays, so a new image keeps its settings. Unset = unchanged (and off by
default, see config.yaml). To switch one off, set it to false. Set them in cmd with `set NAME=value`, in PowerShell
with `$env:NAME = "value"`, in sh with `export NAME=value`:
  ADMIN_PANEL_ENABLED=true     serve the embedded panel under /admin (needs the three ADMIN_WEB_* below)
  ADMIN_WEB_API_KEY, ADMIN_WEB_AUTH_DOMAIN (<project>.firebaseapp.com), ADMIN_WEB_APP_ID   Firebase Web app config.
      These three are written ONLY WHEN THE SERVICE HAS NO VALUE for that name yet, so a deploy from a stale or
      different shell cannot overwrite a good value. To replace one: set ADMIN_WEB_OVERWRITE=true.
  ADMIN_REGISTER_EMAIL=true    POST /api/<v>/admin/register (creates a sign-in account, no role) + panel link
  ADMIN_LOGIN_GOOGLE=true      Google sign-in (off by default; applies to the /admin routes too)
  ADMIN_LOGIN_EMAIL=false      email/password sign-in (on by default)
  ADMIN_ACCEPT_CUSTOM_TOKEN=true  the /admin routes also accept an ID token from a custom token signed with the
                               Firebase Admin service account (Tools/publish_build.py in the Unity repo). Off by default.

  CLIENT_MIN_VERSION=1.1       forced client update: clients with a lower version NAME get 426 Upgrade Required on every
                               API call and the Splash shows "please update". CLIENT_LATEST_VERSION=1.1 is shown by
                               GET /version. Both apply to the public and the admin service; unset = unchanged. Publish the
                               new client BEFORE raising the minimum, or every player is blocked with nothing to update to.

Why the digest: deploying the bare `:latest` tag can make Cloud Run re-serve the digest it resolved earlier, i.e. the
old image.
"""
import argparse
import json
import os
import shutil
import subprocess
import sys

PROJECT_ID = "additive-idle-simulation"
REGION = "us-central1"
REPO = "api"
IMAGE_NAME = "api-library"
PUBLIC_SERVICE = "api-library"
ADMIN_SERVICE = "api-library-admin"
RUNTIME_SA = f"api-runtime@{PROJECT_ID}.iam.gserviceaccount.com"
IMAGE_BASE = f"{REGION}-docker.pkg.dev/{PROJECT_ID}/{REPO}/{IMAGE_NAME}"
IMAGE_TAG = f"{IMAGE_BASE}:latest"
HERE = os.path.dirname(os.path.abspath(__file__))

# Set in the shell; written to the admin service only when set. Order is the order on the command line.
ADMIN_FLAGS = ["ADMIN_PANEL_ENABLED", "ADMIN_REGISTER_EMAIL", "ADMIN_LOGIN_GOOGLE", "ADMIN_LOGIN_EMAIL",
               "ADMIN_ACCEPT_CUSTOM_TOKEN"]
ADMIN_WEB = ["ADMIN_WEB_API_KEY", "ADMIN_WEB_AUTH_DOMAIN", "ADMIN_WEB_APP_ID"]


def say(message):
    print(message, flush=True)


def gcloud_path():
    path = shutil.which("gcloud")
    if not path:
        say("ERROR: gcloud is not on PATH (install the Google Cloud SDK and run `gcloud auth login`).")
        sys.exit(1)
    return path


def run_gcloud(args, dry_run=False, capture=False):
    """Returns (exit code, stdout). Lookups (capture=True) always run; changes are printed under --dry-run."""
    command = [gcloud_path()] + args
    if dry_run and not capture:
        say("[dry-run] gcloud " + " ".join(args))
        return 0, ""
    if capture:
        done = subprocess.run(command, cwd=HERE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
        return done.returncode, done.stdout.strip()
    return subprocess.call(command, cwd=HERE), ""


def git_output(args):
    try:
        done = subprocess.run(["git"] + args, cwd=HERE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    except OSError:
        return ""
    return done.stdout.strip() if done.returncode == 0 else ""


def commit_stamp():
    """Short git hash of what is built, plus "-dirty" when the tree has uncommitted changes (GET /version, the
    [BOOT] log line, the image label and a second image tag)."""
    commit = git_output(["rev-parse", "--short", "HEAD"]) or "dev"
    if git_output(["status", "--porcelain"]):
        commit += "-dirty"
    return commit


CLIENT_VERSION_ENV = ("CLIENT_MIN_VERSION", "CLIENT_LATEST_VERSION")


def client_version_env():
    """The forced-update settings set in the shell, as NAME=value pairs (merged into the service, so unset = unchanged)."""
    return [f"{name}={os.environ[name]}" for name in CLIENT_VERSION_ENV if os.environ.get(name)]


def deploy_public(dry_run):
    commit = commit_stamp()
    commit_image = f"{IMAGE_BASE}:{commit}"
    say(f"=== 1. Using Cloud Build to build the Docker image ({IMAGE_TAG}, commit {commit}) ===")
    code, _ = run_gcloud([
        "builds", "submit", f"--project={PROJECT_ID}", f"--region={REGION}", "--config=cloudbuild.yaml",
        f"--substitutions=_IMAGE={IMAGE_TAG},_COMMIT_IMAGE={commit_image},_COMMIT={commit}", ".",
    ], dry_run)
    if code != 0:
        say("Cloud Build failed.")
        return code

    say("=== 2. Deploying to Cloud Run ===")
    code, _ = run_gcloud([
        "run", "deploy", PUBLIC_SERVICE, f"--project={PROJECT_ID}", f"--image={IMAGE_TAG}", "--platform=managed",
        f"--region={REGION}", f"--service-account={RUNTIME_SA}", "--port=3000", "--allow-unauthenticated",
        "--min-instances=0", "--max-instances=10", "--cpu=1", "--memory=256Mi",
        "--update-env-vars=" + ",".join(
            ["ADMIN_ENABLED=false", "ADMIN_PANEL_ENABLED=false", "ADMIN_REGISTER_EMAIL=false"] + client_version_env()),
    ], dry_run)
    say("=== Deploy completed ===" if code == 0 else f"=== Deploy failed (exit {code}) ===")
    return code


def service_env_names():
    """The env var names the admin service has now (empty on the first deploy, when it does not exist yet)."""
    code, out = run_gcloud(["run", "services", "describe", ADMIN_SERVICE, f"--project={PROJECT_ID}",
                            f"--region={REGION}", "--format=json"], capture=True)
    if code != 0 or not out:
        return set()
    try:
        env = json.loads(out)["spec"]["template"]["spec"]["containers"][0].get("env", [])
    except (ValueError, KeyError, IndexError):
        return set()
    return {item.get("name") for item in env}


def deploy_admin(dry_run):
    code, image_ref = run_gcloud(["artifacts", "docker", "images", "describe", IMAGE_TAG, f"--project={PROJECT_ID}",
                                  "--format=value(image_summary.fully_qualified_digest)"], capture=True)
    if code != 0 or not image_ref:
        say(f"ERROR: no image found for {IMAGE_TAG}. Run `deploy.py public` first, so there is a build to deploy.")
        return 1
    say(f"Image to deploy: {image_ref}")

    env_vars = ["ADMIN_ENABLED=true"] + client_version_env()
    for name in ADMIN_FLAGS:
        if os.environ.get(name):
            env_vars.append(f"{name}={os.environ[name]}")

    wanted = [name for name in ADMIN_WEB if os.environ.get(name)]
    if wanted:
        overwrite = os.environ.get("ADMIN_WEB_OVERWRITE", "").lower() == "true"
        present = set() if overwrite else service_env_names()
        for name in wanted:
            if name in present:
                say(f"{name} is already set on the service, left alone (set ADMIN_WEB_OVERWRITE=true to replace it).")
            else:
                env_vars.append(f"{name}={os.environ[name]}")

    say(f"=== Deploying {ADMIN_SERVICE} (game API + /admin routes) ===")
    say("Env vars written (the rest stay as they are on the service): " + ",".join(env_vars))
    code, _ = run_gcloud([
        "run", "deploy", ADMIN_SERVICE, f"--project={PROJECT_ID}", f"--image={image_ref}", "--platform=managed",
        f"--region={REGION}", f"--service-account={RUNTIME_SA}", "--port=3000", "--allow-unauthenticated",
        "--min-instances=0", "--max-instances=2", "--cpu=1", "--memory=256Mi",
        "--update-env-vars=" + ",".join(env_vars),
    ], dry_run)
    say("=== Done. The panel needs its hostname in Firebase Auth authorized domains. ===" if code == 0
        else f"=== Deploy failed (exit {code}) ===")
    return code


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("target", choices=["public", "admin", "all"])
    parser.add_argument("--dry-run", action="store_true", help="print the build and deploy commands, change nothing")
    args = parser.parse_args()

    if args.target in ("public", "all"):
        say(f"=== {'1/2 ' if args.target == 'all' else ''}Build + deploy {PUBLIC_SERVICE} ===")
        code = deploy_public(args.dry_run)
        if code != 0:
            if args.target == "all":
                say("api-library deploy failed, admin deploy skipped.")
            return code
    if args.target in ("admin", "all"):
        if args.target == "all":
            say(f"=== 2/2 Deploy {ADMIN_SERVICE} (ADMIN_ENABLED=true) ===")
        return deploy_admin(args.dry_run)
    return 0


if __name__ == "__main__":
    sys.exit(main())
