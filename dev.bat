@echo off
rem %~dp0 代表當前 .bat 檔所在目錄的絕對路徑
set GOOGLE_APPLICATION_CREDENTIALS=%~dp0additive-idle-simulation-firebase-adminsdk-fbsvc-ac30a17c67.json

rem 可順便設定其他開發期的環境變數
set APP_ENV=development
set APP_PORT=3000

rem 啟動 Go 應用程式
air