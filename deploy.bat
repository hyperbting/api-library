@echo off
rem Entry point; the logic is in deploy.py (cross-OS Python).
where py >nul 2>nul
if %ERRORLEVEL%==0 (py -3 "%~dp0deploy.py" public %*) else (python "%~dp0deploy.py" public %*)
exit /b %ERRORLEVEL%
