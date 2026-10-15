mod:
	go mod tidy
	go mod download
	go mod vendor

check-build:
	go build ./...

# 靜態分析與型別檢查
vet:
	go vet ./...

# 完整的靜態檢查 (檢查全專案編譯 + 靜態語法分析)
check: check-build vet

# 完整測試 (包含單元測試)
test: check
	go test -v ./...

# Python launcher: `py -3` on Windows, python3 elsewhere. The logic is in deploy.py (cross-OS, standard library only).
ifeq ($(OS),Windows_NT)
PY := py -3
else
PY := python3
endif

# Public API only: no /admin routes, no panel.
deploy:
	$(PY) deploy.py public

# Same image as a second service: game API + /admin routes. ADMIN_PANEL_ENABLED=true also serves the panel.
# Run `make deploy` first (it pushes the image), or use deploy-all.
deploy-admin:
	$(PY) deploy.py admin

deploy-all:
	$(PY) deploy.py all
