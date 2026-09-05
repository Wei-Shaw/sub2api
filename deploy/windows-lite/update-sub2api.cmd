@echo off
setlocal
cd /d "%~dp0"
powershell -NoProfile -ExecutionPolicy Bypass -File ".\update.ps1"
if errorlevel 1 (
    echo.
    echo Update failed. See the error above.
    pause
    exit /b 1
)
echo.
echo Update completed.
pause
