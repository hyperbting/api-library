@echo off
set PROJECT_ID=additive-idle-simulation
set REGION=us-central1
set REPO=api
set SERVICE_NAME=api-library
set RUNTIME_SA=api-runtime@%PROJECT_ID%.iam.gserviceaccount.com
set IMAGE_TAG=%REGION%-docker.pkg.dev/%PROJECT_ID%/%REPO%/%SERVICE_NAME%:latest

echo === 1. Using Cloud Build to Build Docker Image (%IMAGE_TAG%) ===
call gcloud builds submit --project=%PROJECT_ID% --region=%REGION% --config=cloudbuild.yaml --substitutions=_IMAGE=%IMAGE_TAG% .
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
    --memory=256Mi

echo === Deploy Completed！ ===
