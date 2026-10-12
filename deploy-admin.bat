@echo off
rem Deploys the same image as api-library, with the admin panel switched on, as a second Cloud Run service.
rem Run deploy.bat first (it builds and pushes the image). Set the Firebase Web app values before running:
rem   set ADMIN_WEB_API_KEY=...   set ADMIN_WEB_AUTH_DOMAIN=<project>.firebaseapp.com   set ADMIN_WEB_APP_ID=...
rem Google login is off unless you also set ADMIN_LOGIN_GOOGLE=true.
set PROJECT_ID=additive-idle-simulation
set REGION=us-central1
set REPO=api
set IMAGE_NAME=api-library
set SERVICE_NAME=api-library-admin
set RUNTIME_SA=api-runtime@%PROJECT_ID%.iam.gserviceaccount.com
set IMAGE_TAG=%REGION%-docker.pkg.dev/%PROJECT_ID%/%REPO%/%IMAGE_NAME%:latest
if "%ADMIN_LOGIN_GOOGLE%"=="" set ADMIN_LOGIN_GOOGLE=false
if "%ADMIN_LOGIN_EMAIL%"=="" set ADMIN_LOGIN_EMAIL=true

echo === Deploying %SERVICE_NAME% (admin panel on) ===
call gcloud run deploy %SERVICE_NAME% ^
    --project=%PROJECT_ID% ^
    --image=%IMAGE_TAG% ^
    --platform=managed ^
    --region=%REGION% ^
    --service-account=%RUNTIME_SA% ^
    --port=3000 ^
    --allow-unauthenticated ^
    --min-instances=0 ^
    --max-instances=2 ^
    --cpu=1 ^
    --memory=256Mi ^
    --set-env-vars=ADMIN_ENABLED=true,ADMIN_LOGIN_GOOGLE=%ADMIN_LOGIN_GOOGLE%,ADMIN_LOGIN_EMAIL=%ADMIN_LOGIN_EMAIL%,ADMIN_WEB_API_KEY=%ADMIN_WEB_API_KEY%,ADMIN_WEB_AUTH_DOMAIN=%ADMIN_WEB_AUTH_DOMAIN%,ADMIN_WEB_APP_ID=%ADMIN_WEB_APP_ID%

echo === Done. Add the service hostname to Firebase Auth authorized domains. ===
