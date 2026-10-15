# api-library (Go backend of the Unity game "Lettuce Make Money")

Fiber v3 + Firestore on Cloud Run: game API, admin API and the embedded Vue admin panel (`admin/` -> `internal/adminui/dist`). Go 1.26.9, deps vendored.

## Paired project
- The Unity client lives in `../Additive-Idle-Simulation` (override with env `LMM_UNITY_PROJECT`; the Unity repo's `.claude/pairing.json` and `Tools/paired_project.py` are the source of truth for the pairing). Its docs (`docs/GDD.md`, `docs/ARCHITECTURE.md`, `docs/SESSION_HANDOFF.md`) hold the design and the handoff.
- Contract changes (DTOs, endpoints, quest catalog version) need both sides: the quest catalog is exported from the Unity assets into `internal/quest/data/quests.json`.
- `Tools/publish_build.py` in the Unity repo calls this service's admin API (news) and needs `ADMIN_ACCEPT_CUSTOM_TOKEN=true` on the admin service.

## Commands (cross-OS)
- `make check` / `make test`: build, vet, tests.
- `python deploy.py public|admin|all [--dry-run]` (launchers `deploy.bat` / `deploy.sh`, `deploy-admin.*`, `deploy-all.*`; `make deploy`, `make deploy-admin`, `make deploy-all`): Cloud Build image, public service, then the admin service on the same image digest. `--dry-run` prints the gcloud commands.
- `python dev.py` (`dev.bat` / `dev.sh`): local run with live reload (`air`).

## Git
- Branch `master`. Conventional commits (`feat(scope): ...`). Commit only when asked.
