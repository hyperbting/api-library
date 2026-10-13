@echo off
rem One command: build the image once, deploy api-library (public: game API only, no /admin routes, no panel),
rem then api-library-admin (same image: game API + /admin routes; panel only with ADMIN_PANEL_ENABLED=true).
rem See deploy-admin.bat for the ADMIN_* values.

echo === 1/2 Build + deploy api-library ===
call "%~dp0deploy.bat"
if %ERRORLEVEL% NEQ 0 (
    echo api-library deploy failed, admin deploy skipped.
    exit /b %ERRORLEVEL%
)

rem deploy.bat already sets ADMIN_ENABLED=false and ADMIN_PANEL_ENABLED=false on the public service.

echo === 2/2 Deploy api-library-admin (ADMIN_ENABLED=true) ===
call "%~dp0deploy-admin.bat"
exit /b %ERRORLEVEL%
