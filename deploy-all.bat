@echo off
rem One command: build the image once, deploy api-library (public, admin panel off), then deploy
rem api-library-admin (same image, ADMIN_ENABLED=true). Set the ADMIN_WEB_* values first, see deploy-admin.bat.

echo === 1/2 Build + deploy api-library ===
call deploy.bat
if %ERRORLEVEL% NEQ 0 (
    echo api-library deploy failed, admin deploy skipped.
    exit /b %ERRORLEVEL%
)

rem Make sure the public service keeps the panel off, whatever it had before.
call gcloud run services update api-library --project=additive-idle-simulation --region=us-central1 --update-env-vars=ADMIN_ENABLED=false

echo === 2/2 Deploy api-library-admin (ADMIN_ENABLED=true) ===
call deploy-admin.bat
exit /b %ERRORLEVEL%
