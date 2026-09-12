$ErrorActionPreference = "Stop"
$Python = "C:\conda\python.exe"

& $Python -m pip install --upgrade pyinstaller
& $Python -m PyInstaller --noconfirm --onefile --windowed --name "Sub2API Key Manager" --add-data "..\ai_key_market.ico;." --icon "..\ai_key_market.ico" app.py
Copy-Item -LiteralPath ".\sub2api_key_manager.env.example" -Destination ".\dist\sub2api_key_manager.env.example" -Force

Write-Host "Built dist\Sub2API Key Manager.exe"
