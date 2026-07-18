@echo off
cd /d "%~dp0"
if not exist logs mkdir logs
grok-only-gateway.exe > logs\server.log 2>&1
