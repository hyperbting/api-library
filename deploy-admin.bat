@echo off
rem Deploys the same image as api-library as a second Cloud Run service: the game API plus the /admin routes
rem (ADMIN_ENABLED=true). Run deploy.bat first (it builds and pushes the image), or use deploy-all.bat.
rem
rem Environment variables are MERGED into the service (--update-env-vars): only the ones you set in this shell are
rem written, everything already on the service stays, so a new image keeps its settings. Unset = unchanged (and
rem off by default, see config.yaml). To switch one off, set it to false. Set them in cmd with "set NAME=value",
rem in PowerShell with $env:NAME = "value":
rem   ADMIN_PANEL_ENABLED=true     serve the embedded panel under /admin (needs the three ADMIN_WEB_* below)
rem   ADMIN_WEB_API_KEY, ADMIN_WEB_AUTH_DOMAIN (<project>.firebaseapp.com), ADMIN_WEB_APP_ID   Firebase Web app config.
rem       These three are written ONLY WHEN THE SERVICE HAS NO VALUE for that name yet, so a deploy from a stale
rem       or different shell cannot overwrite a good value. To replace one: set ADMIN_WEB_OVERWRITE=true.
rem   ADMIN_REGISTER_EMAIL=true    POST /api/<v>/admin/register (creates a sign-in account, no role) + panel link
rem   ADMIN_LOGIN_GOOGLE=true      Google sign-in (off by default; applies to the /admin routes too)
rem   ADMIN_LOGIN_EMAIL=false      email/password sign-in (on by default)
set PROJECT_ID=additive-idle-simulation
set REGION=us-central1
set REPO=api
set IMAGE_NAME=api-library
set SERVICE_NAME=api-library-admin
set RUNTIME_SA=api-runtime@%PROJECT_ID%.iam.gserviceaccount.com
set IMAGE_TAG=%REGION%-docker.pkg.dev/%PROJECT_ID%/%REPO%/%IMAGE_NAME%:latest

set ENV_VARS=ADMIN_ENABLED=true
if not "%ADMIN_PANEL_ENABLED%"=="" set ENV_VARS=%ENV_VARS%,ADMIN_PANEL_ENABLED=%ADMIN_PANEL_ENABLED%
if not "%ADMIN_REGISTER_EMAIL%"=="" set ENV_VARS=%ENV_VARS%,ADMIN_REGISTER_EMAIL=%ADMIN_REGISTER_EMAIL%
if not "%ADMIN_LOGIN_GOOGLE%"=="" set ENV_VARS=%ENV_VARS%,ADMIN_LOGIN_GOOGLE=%ADMIN_LOGIN_GOOGLE%
if not "%ADMIN_LOGIN_EMAIL%"=="" set ENV_VARS=%ENV_VARS%,ADMIN_LOGIN_EMAIL=%ADMIN_LOGIN_EMAIL%

rem What the service has now (empty on the first deploy, when the service does not exist yet).
set CURRENT_ENV=%TEMP%\%SERVICE_NAME%-env.txt
call gcloud run services describe %SERVICE_NAME% --project=%PROJECT_ID% --region=%REGION% --format="value(spec.template.spec.containers[0].env)" > "%CURRENT_ENV%" 2>nul
call :web_value ADMIN_WEB_API_KEY
call :web_value ADMIN_WEB_AUTH_DOMAIN
call :web_value ADMIN_WEB_APP_ID
del "%CURRENT_ENV%" 2>nul

echo === Deploying %SERVICE_NAME% (game API + /admin routes) ===
echo Env vars written (the rest stay as they are on the service): %ENV_VARS%
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
    --update-env-vars=%ENV_VARS%

set DEPLOY_RC=%ERRORLEVEL%

echo === Done. The panel needs its hostname in Firebase Auth authorized domains. ===
exit /b %DEPLOY_RC%

rem Adds NAME=value to ENV_VARS when it is set in this shell and the service has no value for it.
:web_value
set "WEB_NAME=%~1"
call set "WEB_VALUE=%%%WEB_NAME%%%"
if "%WEB_VALUE%"=="" goto :eof
if /i "%ADMIN_WEB_OVERWRITE%"=="true" goto :web_add
findstr /C:"'%WEB_NAME%'" "%CURRENT_ENV%" >nul 2>nul
if not errorlevel 1 (
    echo %WEB_NAME% is already set on the service, left alone ^(set ADMIN_WEB_OVERWRITE=true to replace it^).
    goto :eof
)
:web_add
set ENV_VARS=%ENV_VARS%,%WEB_NAME%=%WEB_VALUE%
goto :eof
