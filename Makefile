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