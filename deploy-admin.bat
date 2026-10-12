@echo off
rem Deploys the same image as api-library as a second Cloud Run service: the game API plus the /admin routes
rem (ADMIN_ENABLED=true). The embedded admin panel stays OFF unless you set ADMIN_PANEL_ENABLED=true first.
rem Run deploy.bat first (it builds and pushes the image), or use deploy-all.bat.
rem Only for the panel: set the Firebase Web app values before running
rem   set ADMIN_WEB_API_KEY=...   set ADMIN_WEB_AUTH_DOMAIN=<project>.firebaseapp.com   set ADMIN_WEB_APP_ID=...
rem Google login is off unless you also set ADMIN_LOGIN_GOOGLE=true (it applies to the /admin routes too).
rem ADMIN_REGISTER_EMAIL=true adds POST /api/<v>/admin/register (creates a sign-in account, no role) and, with the
rem panel on, a "create an account" link. Off unless set. Needs ADMIN_LOGIN_EMAIL (on by default).
set PROJECT_ID=additive-idle-simulation
set REGION=us-central1
set REPO=api
set IMAGE_NAME=api-library
set SERVICE_NAME=api-library-admin
set RUNTIME_SA=api-runtime@%PROJECT_ID%.iam.gserviceaccount.com
set IMAGE_TAG=%REGION%-docker.pkg.dev/%PROJECT_ID%/%REPO%/%IMAGE_NAME%:latest
if "%ADMIN_PANEL_ENABLED%"=="" set ADMIN_PANEL_ENABLED=false
if "%ADMIN_REGISTER_EMAIL%"=="" set ADMIN_REGISTER_EMAIL=false
if "%ADMIN_LOGIN_GOOGLE%"=="" set ADMIN_LOGIN_GOOGLE=false
if "%ADMIN_LOGIN_EMAIL%"=="" set ADMIN_LOGIN_EMAIL=true

set ENV_VARS=ADMIN_ENABLED=true,ADMIN_PANEL_ENABLED=%ADMIN_PANEL_ENABLED%,ADMIN_REGISTER_EMAIL=%ADMIN_REGISTER_EMAIL%,ADMIN_LOGIN_GOOGLE=%ADMIN_LOGIN_GOOGLE%,ADMIN_LOGIN_EMAIL=%ADMIN_LOGIN_EMAIL%
rem The Firebase Web app values are only read by the panel.
if "%ADMIN_PANEL_ENABLED%"=="true" set ENV_VARS=%ENV_VARS%,ADMIN_WEB_API_KEY=%ADMIN_WEB_API_KEY%,ADMIN_WEB_AUTH_DOMAIN=%ADMIN_WEB_AUTH_DOMAIN%,ADMIN_WEB_APP_ID=%ADMIN_WEB_APP_ID%

echo === Deploying %SERVICE_NAME% (game API + /admin routes, admin panel: %ADMIN_PANEL_ENABLED%, register: %ADMIN_REGISTER_EMAIL%) ===
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
    --set-env-vars=%ENV_VARS%

echo === Done. The panel needs its hostname in Firebase Auth authorized domains. ===
