@echo off
set PROJECT_ID=additive-idle-simulation
set REGION=us-central1
set REPO=api
set SERVICE_NAME=api-library
set RUNTIME_SA=api-runtime@%PROJECT_ID%.iam.gserviceaccount.com
set IMAGE_BASE=%REGION%-docker.pkg.dev/%PROJECT_ID%/%REPO%/%SERVICE_NAME%
set IMAGE_TAG=%IMAGE_BASE%:latest

rem The short git hash of what is built (+ "-dirty" when the tree has uncommitted changes), stamped into the binary
rem (GET /version, the [BOOT] log line), the image label and a second image tag.
set COMMIT=dev
for /f %%H in ('git rev-parse --short HEAD 2^>nul') do set COMMIT=%%H
for /f %%S in ('git status --porcelain 2^>nul') do set COMMIT=%COMMIT%-dirty
set COMMIT_IMAGE=%IMAGE_BASE%:%COMMIT%

echo === 1. Using Cloud Build to Build Docker Image (%IMAGE_TAG%, commit %COMMIT%) ===
call gcloud builds submit --project=%PROJECT_ID% --region=%REGION% --config=cloudbuild.yaml --substitutions=_IMAGE=%IMAGE_TAG%,_COMMIT_IMAGE=%COMMIT_IMAGE%,_COMMIT=%COMMIT% .
if %ERRORLEVEL% NEQ 0 (
    echo Cloud Build Failed！
    exit /b %ERRORLEVEL%
)

echo === 2. Deploying to Cloud Run ===
call gcloud run deploy %SERVICE_NAME% ^
    --project=%PROJECT_ID% ^
    --image=%IMAGE_TAG% ^
    --platform=managed ^
    --region=%REGION% ^
    --service-account=%RUNTIME_SA% ^
    --port=3000 ^
    --allow-unauthenticated ^
    --min-instances=0 ^
    --max-instances=10 ^
    --cpu=1 ^
    --memory=256Mi ^
    --update-env-vars=ADMIN_ENABLED=false,ADMIN_PANEL_ENABLED=false,ADMIN_REGISTER_EMAIL=false

echo === Deploy Completed！ ===
