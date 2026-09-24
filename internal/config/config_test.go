package config_test

import (
	"api-library/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_FromProjectRoot(t *testing.T) {
	// 1. 取得當前測試檔案所在目錄 (internal/config)
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	t.Logf("currentDir: %#+v", currentDir)

	// 2. 推算專案根目錄 (往上兩層: internal/config -> internal -> root)
	rootDir := filepath.Join(currentDir, "..", "..")
	t.Logf("rootDir: %#+v", rootDir)

	// 3. 將工作目錄切換至專案根目錄
	origWd, _ := os.Getwd()
	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("Failed to change working directory to root: %v", err)
	}
	defer os.Chdir(origWd) // 測試結束後還原工作目錄

	// 4. 隔離環境變數，避免污染
	t.Setenv("APP_ENV", "test-root")

	// 5. 執行載入 (此時會讀取根目錄下的 config.yaml / config.local.yaml)
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Expected successful config load from root, got error: %v", err)
	}

	// 6. 驗證產出不為 nil
	if cfg == nil {
		t.Fatal("Expected non-nil AppConfig")
	}

	// 驗證環境變數覆蓋邏輯仍正常運作
	if cfg.App.Env != "test-root" {
		t.Errorf("Expected App.Env to be 'test-root', got '%s'", cfg.App.Env)
	}

	_ = cfg.DebugPrint()
}

// 建立暫存檔輔助函式
func createTempConfigFile(t *testing.T, dir string, filename string, content string) string {
	t.Helper()
	filePath := filepath.Join(dir, filename)
	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp config file %s: %v", filename, err)
	}
	return filePath
}

func TestLoadConfig_Success(t *testing.T) {
	// 1. 建立測試用的暫存目錄並切換 Working Directory
	tempDir := t.TempDir()
	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	// 2. 寫入基礎 config.yaml
	baseYAML := `
app:
  env: "development"
  port: 8080
  version: "v1.0.0"

database:
  master:
    host: "localhost"
    port: 5432
    user: "postgres"
    password: "password"
    dbname: "mydb"
`
	createTempConfigFile(t, tempDir, "config.yaml", baseYAML)

	// 3. 寫入本地覆蓋檔 config.local.yaml (驗證 Merge 機制)
	localYAML := `
app:
  port: 9090
database:
  master:
    password: "local_secret_password"
`
	createTempConfigFile(t, tempDir, "config.local.yaml", localYAML)

	// 4. 設定 OS 環境變數 (驗證 AutomaticEnv 最高優先權覆蓋)
	// Go 1.17+ 的 t.Setenv 會在測試結束後自動還原環境變數
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_MASTER_HOST", "prod-db-cluster")

	// 5. 執行測試目標
	cfg, err := config.LoadConfig()

	// 6. 斷言驗證
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// 驗證 1: OS 環境變數覆蓋 Successful (APP_ENV = production)
	if cfg.App.Env != "production" {
		t.Errorf("Expected App.Env to be 'production', got '%s'", cfg.App.Env)
	}

	// 驗證 2: local.yaml 覆蓋 Successful (App.Port = 9090)
	if cfg.App.Port != 9090 {
		t.Errorf("Expected App.Port to be 9090, got %d", cfg.App.Port)
	}

	// 驗證 3: config.yaml 預設值保持 (App.Version = v1.0.0)
	if cfg.App.Version != "v1.0.0" {
		t.Errorf("Expected App.Version to be 'v1.0.0', got '%s'", cfg.App.Version)
	}

	// 驗證 4: OS 環境變數覆蓋 Nested Field (Database.MasterDB.Host = prod-db-cluster)
	if cfg.Database.MasterDB.Host != "prod-db-cluster" {
		t.Errorf("Expected Database.MasterDB.Host to be 'prod-db-cluster', got '%s'", cfg.Database.MasterDB.Host)
	}

	// 驗證 5: local.yaml 覆蓋 Password Successful (Database.MasterDB.Password = local_secret_password)
	if cfg.Database.MasterDB.Password != "local_secret_password" {
		t.Errorf("Expected Database.MasterDB.Password to be 'local_secret_password', got '%s'", cfg.Database.MasterDB.Password)
	}
}

func TestLoadConfig_UnmarshalError(t *testing.T) {
	tempDir := t.TempDir()
	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer os.Chdir(origWd)

	// 寫入無效的 YAML 資料型別 (Port 給予字串而非整數，觸發 Unmarshal 錯誤)
	invalidYAML := `
app:
  port: "invalid_port_string"
`
	createTempConfigFile(t, tempDir, "config.yaml", invalidYAML)

	cfg, err := config.LoadConfig()

	if err == nil {
		t.Fatal("Expected error due to type mismatch during Unmarshal, got nil")
	}
	if cfg != nil {
		t.Errorf("Expected nil config on error, got %+v", cfg)
	}
}
