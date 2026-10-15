@echo off
rem Entry point; the logic is in dev.py (cross-OS Python).
where py >nul 2>nul
if %ERRORLEVEL%==0 (py -3 "%~dp0dev.py" %*) else (python "%~dp0dev.py" %*)
exit /b %ERRORLEVEL%
