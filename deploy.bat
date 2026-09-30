@echo off
set PROJECT_ID=YOUR_GCP_PROJECT_ID
set REGION=asia-east1
set REPO=my-game-repo
set SERVICE_NAME=my-go-backend
set IMAGE_TAG=%REGION%-docker.pkg.dev/%PROJECT_ID%/%REPO%/%SERVICE_NAME%:latest

echo === 1. 開始使用 Cloud Build 打包 Docker Image (%IMAGE_TAG%) ===
call gcloud builds submit --tag %IMAGE_TAG% --dockerfile=Dockerfile.GCPCloudRun .
if %ERRORLEVEL% NEQ 0 (
    echo Cloud Build 失敗！
    exit /b %ERRORLEVEL%
)

echo === 2. 部署至 Cloud Run ===
call gcloud run deploy %SERVICE_NAME% ^
    --image=%IMAGE_TAG% ^
    --platform=managed ^
    --region=%REGION% ^
    --allow-unauthenticated ^
    --min-instances=0 ^
    --max-instances=10 ^
    --cpu=1 ^
    --memory=512Mi

echo === 部署完成！ ===