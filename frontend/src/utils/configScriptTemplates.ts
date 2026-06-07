export type ConfigScriptTemplateValues = Record<string, string>

export function renderConfigScriptTemplate(template: string, values: ConfigScriptTemplateValues): string {
  return template.replace(/\{\{([A-Z0-9_]+)\}\}/g, (match, key: string) => {
    if (Object.prototype.hasOwnProperty.call(values, key)) {
      return values[key]
    }
    return match
  })
}

export const WINDOWS_NODE_RUNTIME_NOTICE_TEMPLATE = `where node.exe >nul 2>nul
if errorlevel 1 (
  echo.
  echo [LOOK2EYE NOTICE] Windows 未检测到 Node.js；配置文件已写入，但 {{CLIENT_LABEL}} 可能无法安装或运行。
  set "LOOK2EYE_INSTALL_NODE="
  set /p "LOOK2EYE_INSTALL_NODE=是否现在通过 winget 安装 Node.js LTS？[Y/N] "
  if /i "!LOOK2EYE_INSTALL_NODE!"=="Y" (
    where winget.exe >nul 2>nul
    if errorlevel 1 (
      echo 未检测到 winget，无法自动安装 Node.js。
      echo 请手动下载安装 Node.js LTS：https://nodejs.org/
    ) else (
      winget install --id OpenJS.NodeJS.LTS --source winget --accept-source-agreements --accept-package-agreements
      where node.exe >nul 2>nul
      if errorlevel 1 (
        echo Node.js 安装后当前窗口仍未检测到 node.exe；请重新打开终端后再运行客户端。
      ) else (
        for /f "delims=" %%I in ('node --version 2^>nul') do echo Node.js: %%I
      )
    )
  ) else (
    echo 已跳过 Node.js 安装。请稍后安装 Node.js LTS：https://nodejs.org/
  )
) else (
  for /f "delims=" %%I in ('node --version 2^>nul') do echo Node.js: %%I
  where npm.cmd >nul 2>nul
  if errorlevel 1 (
    echo [LOOK2EYE NOTICE] 未检测到 npm；如客户端安装失败，请重新安装 Node.js LTS 并勾选 npm。
  ) else (
    for /f "delims=" %%I in ('npm --version 2^>nul') do echo npm: %%I
  )
)`

export const WINDOWS_CLIENT_INSTALL_NOTICE_TEMPLATE = `set "LOOK2EYE_CLIENT_FOUND=0"
where {{COMMAND_NAME}} >nul 2>nul
if not errorlevel 1 set "LOOK2EYE_CLIENT_FOUND=1"
if "!LOOK2EYE_CLIENT_FOUND!"=="0" (
  echo.
  echo [LOOK2EYE NOTICE] Windows 未检测到 {{CLIENT_LABEL}}，准备通过 npm 全局安装。
  where npm.cmd >nul 2>nul
  if errorlevel 1 (
    echo 未检测到 npm，无法自动安装 {{CLIENT_LABEL}}。
    echo 请安装 Node.js LTS 后重新运行本脚本，或手动执行：npm install -g {{NPM_PACKAGE}}
  ) else (
    npm install -g {{NPM_PACKAGE}}
    if errorlevel 1 (
      echo {{CLIENT_LABEL}} 自动安装失败，请手动执行：npm install -g {{NPM_PACKAGE}}
    ) else (
      where {{COMMAND_NAME}} >nul 2>nul
      if errorlevel 1 (
        echo {{CLIENT_LABEL}} 已安装，但当前窗口仍未检测到 {{COMMAND_NAME}}；请重新打开终端后再运行客户端。
      ) else (
        for /f "delims=" %%I in ('{{COMMAND_NAME}} --version 2^>nul') do echo {{CLIENT_LABEL}}: %%I
      )
    )
  )
) else (
  for /f "delims=" %%I in ('{{COMMAND_NAME}} --version 2^>nul') do echo {{CLIENT_LABEL}}: %%I
)`

export const WINDOWS_EMBEDDED_POWERSHELL_BATCH_TEMPLATE = `@echo off
chcp 65001 >nul
setlocal EnableDelayedExpansion
set "LOOK2EYE_SETUP_SCRIPT_PATH=%~f0"
set "LOOK2EYE_SETUP_PAYLOAD=%TEMP%\\{{TEMP_NAME}}-%RANDOM%-%RANDOM%.ps1"
set "LOOK2EYE_SETUP_EXIT=1"
set "LOOK2EYE_SETUP_MARKER={{MARKER}}"
powershell.exe -NoProfile -ExecutionPolicy Bypass -EncodedCommand {{EXTRACTOR_COMMAND}}
if errorlevel 1 (
  set "LOOK2EYE_SETUP_EXIT=1"
  goto :look2eye_setup_done
)
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%LOOK2EYE_SETUP_PAYLOAD%"{{FORWARDED_ARGS}}
set "LOOK2EYE_SETUP_EXIT=%ERRORLEVEL%"
if exist "%LOOK2EYE_SETUP_PAYLOAD%" del /f /q "%LOOK2EYE_SETUP_PAYLOAD%" >nul 2>nul
:look2eye_setup_done
echo.
if "%LOOK2EYE_SETUP_EXIT%"=="0" (
  echo ============================================================
  echo [LOOK2EYE SETUP SUCCESS] {{SUCCESS_MESSAGE}}
  echo {{SUCCESS_HINT}}
  echo ============================================================
) else (
  echo ============================================================
  echo [LOOK2EYE SETUP FAILED] {{FAILURE_MESSAGE}} Exit code: %LOOK2EYE_SETUP_EXIT%
  echo {{FAILURE_HINT}}
  echo ============================================================
)
if "%LOOK2EYE_SETUP_EXIT%"=="0" (
{{NODE_RUNTIME_NOTICE}}
{{CLIENT_INSTALL_NOTICE}}
)
pause
endlocal & exit /b %LOOK2EYE_SETUP_EXIT%

{{MARKER}}
{{POWERSHELL_PAYLOAD}}`

export const WINDOWS_BASIC_BATCH_TEMPLATE = `@echo off
setlocal EnableDelayedExpansion
echo Installing {{SITE_NAME}} {{PAYLOAD_LABEL}} configuration...
powershell.exe -NoProfile -ExecutionPolicy Bypass -EncodedCommand {{ENCODED_COMMAND}}
if errorlevel 1 (
  echo Installation failed.
  pause
  exit /b 1
)
echo Done.
pause
`
