@echo off
setlocal EnableDelayedExpansion

rem -----------------------------------------------------------------------------
rem CLIProxyAPI one-shot source launcher for Windows.
rem
rem Responsibilities:
rem   1. Ensure a Go toolchain is available; install a portable copy under
rem      .tools\go when none is found.
rem   2. Bootstrap config.yaml from config.example.yaml on first run.
rem   3. Show a provider menu and detect existing auth files for that provider.
rem   4. If no auth file exists for the chosen provider, run the matching
rem      `go run ./cmd/server -<provider>-login` flow.
rem   5. Boot the proxy with `go run ./cmd/server` (source build, no install).
rem
rem Flags:
rem   --no-menu          Skip the menu, jump straight to running the server.
rem   --provider <name>  Login non-interactively (gemini/codex/codex-device/
rem                      claude/antigravity/kimi).
rem   --auth-dir <path>  Override the auth directory used for login detection.
rem   --config <path>    Override the config file path passed to the server.
rem   --no-browser       Pass `-no-browser` to the login flows.
rem   --no-go-install    Do not auto-install Go; just fail when missing.
rem   --kill-port        Stop any process listening on port 8317 before start.
rem -----------------------------------------------------------------------------

set "SCRIPT_DIR=%~dp0"
pushd "%SCRIPT_DIR%" >nul

set "CLI_PROXY_NO_MENU="
set "CLI_PROXY_PROVIDER="
set "CLI_PROXY_AUTH_DIR=%USERPROFILE%\.cli-proxy-api"
set "CLI_PROXY_CONFIG=%SCRIPT_DIR%config.yaml"
set "CLI_PROXY_EXAMPLE_CONFIG=%SCRIPT_DIR%config.example.yaml"
set "CLI_PROXY_NO_BROWSER="
set "CLI_PROXY_NO_GO_INSTALL="
set "CLI_PROXY_KILL_PORT="
set "CLI_PROXY_PORT=8317"
set "CLI_PROXY_LOCAL_GO=%SCRIPT_DIR%.tools\go"

:parse_args
if "%~1"=="" goto args_done
if /i "%~1"=="--no-menu" (
    set "CLI_PROXY_NO_MENU=1"
    shift
    goto parse_args
)
if /i "%~1"=="--no-browser" (
    set "CLI_PROXY_NO_BROWSER=1"
    shift
    goto parse_args
)
if /i "%~1"=="--no-go-install" (
    set "CLI_PROXY_NO_GO_INSTALL=1"
    shift
    goto parse_args
)
if /i "%~1"=="--kill-port" (
    set "CLI_PROXY_KILL_PORT=1"
    shift
    goto parse_args
)
if /i "%~1"=="--provider" (
    set "CLI_PROXY_PROVIDER=%~2"
    shift
    shift
    goto parse_args
)
if /i "%~1"=="--auth-dir" (
    set "CLI_PROXY_AUTH_DIR=%~2"
    shift
    shift
    goto parse_args
)
if /i "%~1"=="--config" (
    set "CLI_PROXY_CONFIG=%~2"
    shift
    shift
    goto parse_args
)
echo [run-source] unknown argument: %~1
echo Usage: run-source.bat [--no-menu] [--no-browser] [--no-go-install] [--kill-port]
echo                       [--provider name] [--auth-dir path] [--config path]
popd >nul
exit /b 2

:args_done

call :require_go || goto exit_err
call :ensure_config || goto exit_err

if not exist "%CLI_PROXY_AUTH_DIR%" (
    echo [run-source] creating auth directory: %CLI_PROXY_AUTH_DIR%
    mkdir "%CLI_PROXY_AUTH_DIR%" 2>nul
)

if defined CLI_PROXY_NO_MENU goto run_server
if defined CLI_PROXY_PROVIDER goto handle_provider

:show_menu
echo.
echo ==== CLIProxyAPI source launcher ====
echo Auth dir: %CLI_PROXY_AUTH_DIR%
echo Config:   %CLI_PROXY_CONFIG%
echo.
echo Choose a provider to ensure login:
echo   1) Gemini       (-login)
echo   2) Codex        (-codex-login)
echo   3) Codex Device (-codex-device-login)
echo   4) Claude       (-claude-login)
echo   5) Antigravity  (-antigravity-login)
echo   6) Kimi         (-kimi-login)
echo   7) Skip login, run server now
echo   0) Exit
echo.
set "MENU_CHOICE="
set /p MENU_CHOICE=Enter choice [1-7, Enter to skip, 0 to exit]: 

if "%MENU_CHOICE%"=="" goto run_server
if "%MENU_CHOICE%"=="0" goto exit_ok
if "%MENU_CHOICE%"=="1" (set "CLI_PROXY_PROVIDER=gemini" & goto handle_provider)
if "%MENU_CHOICE%"=="2" (set "CLI_PROXY_PROVIDER=codex" & goto handle_provider)
if "%MENU_CHOICE%"=="3" (set "CLI_PROXY_PROVIDER=codex-device" & goto handle_provider)
if "%MENU_CHOICE%"=="4" (set "CLI_PROXY_PROVIDER=claude" & goto handle_provider)
if "%MENU_CHOICE%"=="5" (set "CLI_PROXY_PROVIDER=antigravity" & goto handle_provider)
if "%MENU_CHOICE%"=="6" (set "CLI_PROXY_PROVIDER=kimi" & goto handle_provider)
if "%MENU_CHOICE%"=="7" goto run_server

echo Invalid choice: %MENU_CHOICE%
goto show_menu

:handle_provider
call :ensure_login "%CLI_PROXY_PROVIDER%" || goto exit_err
goto run_server

:run_server
echo.
call :ensure_port_available || goto exit_err
echo [run-source] starting proxy via `go run ./cmd/server`
echo [run-source] press Ctrl+C to stop.
go run ./cmd/server --config "%CLI_PROXY_CONFIG%"
set "EXIT_CODE=%ERRORLEVEL%"
popd >nul
exit /b %EXIT_CODE%

:exit_ok
popd >nul
exit /b 0

:exit_err
popd >nul
exit /b 1

rem -------------------- helpers --------------------

:require_go
call :try_locate_go
if not errorlevel 1 (
    for /f "tokens=*" %%V in ('go version 2^>nul') do echo [run-source] using %%V
    exit /b 0
)
if defined CLI_PROXY_NO_GO_INSTALL (
    echo [run-source] ERROR: Go toolchain not found in PATH and --no-go-install was given.
    echo [run-source] Install Go 1.26+ from https://go.dev/dl/ and re-run this script.
    exit /b 1
)
echo [run-source] Go toolchain not detected; installing a portable copy to:
echo [run-source]   %CLI_PROXY_LOCAL_GO%
call :install_portable_go
if errorlevel 1 (
    echo [run-source] ERROR: failed to install Go automatically.
    echo [run-source] You can install it manually from https://go.dev/dl/
    echo [run-source] or re-run with `--no-go-install` after putting `go` on PATH.
    exit /b 1
)
call :try_locate_go
if errorlevel 1 (
    echo [run-source] ERROR: Go installation finished but `go` is still not detected.
    exit /b 1
)
for /f "tokens=*" %%V in ('go version 2^>nul') do echo [run-source] using %%V
exit /b 0

:try_locate_go
where go >nul 2>nul
if not errorlevel 1 exit /b 0
call :prepend_if_has_go "%CLI_PROXY_LOCAL_GO%\bin"
call :prepend_if_has_go "%ProgramFiles%\Go\bin"
call :prepend_if_has_go "%ProgramW6432%\Go\bin"
call :prepend_if_has_go "%LocalAppData%\Programs\Go\bin"
call :prepend_if_has_go "%USERPROFILE%\go\bin"
call :prepend_if_has_go "%SystemDrive%\Go\bin"
where go >nul 2>nul
exit /b %ERRORLEVEL%

:prepend_if_has_go
if "%~1"=="" exit /b 0
if exist "%~1\go.exe" set "PATH=%~1;%PATH%"
exit /b 0

:install_portable_go
set "CLI_PROXY_TOOLS_DIR=%SCRIPT_DIR%.tools"
if not exist "%CLI_PROXY_TOOLS_DIR%" mkdir "%CLI_PROXY_TOOLS_DIR%" 2>nul
if exist "%CLI_PROXY_LOCAL_GO%\bin\go.exe" (
    set "PATH=%CLI_PROXY_LOCAL_GO%\bin;%PATH%"
    exit /b 0
)
set "CLI_PROXY_GO_ARCH=amd64"
if /i "%PROCESSOR_ARCHITECTURE%"=="ARM64" set "CLI_PROXY_GO_ARCH=arm64"
if /i "%PROCESSOR_ARCHITECTURE%"=="AMD64" set "CLI_PROXY_GO_ARCH=amd64"
if /i "%PROCESSOR_ARCHITEW6432%"=="AMD64" set "CLI_PROXY_GO_ARCH=amd64"

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$ErrorActionPreference='Stop';" ^
  "$tools = $env:CLI_PROXY_TOOLS_DIR;" ^
  "$arch  = $env:CLI_PROXY_GO_ARCH;" ^
  "[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12;" ^
  "try { $ver = (Invoke-WebRequest -Uri 'https://go.dev/VERSION?m=text' -UseBasicParsing -TimeoutSec 30).Content.Split([char]10)[0].Trim() } catch { $ver = 'go1.26.0' };" ^
  "if (-not $ver.StartsWith('go')) { throw \"unexpected version response: $ver\" };" ^
  "$url = 'https://go.dev/dl/' + $ver + '.windows-' + $arch + '.zip';" ^
  "$zip = Join-Path $env:TEMP ('cliproxy-go-' + [System.IO.Path]::GetRandomFileName() + '.zip');" ^
  "Write-Host ('[run-source] downloading ' + $url);" ^
  "Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing -TimeoutSec 600;" ^
  "Write-Host '[run-source] extracting Go archive ...';" ^
  "Expand-Archive -LiteralPath $zip -DestinationPath $tools -Force;" ^
  "Remove-Item $zip -Force -ErrorAction SilentlyContinue;" ^
  "if (-not (Test-Path (Join-Path $tools 'go\bin\go.exe'))) { throw 'go.exe missing after extract' };"
if errorlevel 1 exit /b 1
set "PATH=%CLI_PROXY_LOCAL_GO%\bin;%PATH%"
exit /b 0

:ensure_config
if exist "%CLI_PROXY_CONFIG%" exit /b 0
if not exist "%CLI_PROXY_EXAMPLE_CONFIG%" (
    echo [run-source] ERROR: %CLI_PROXY_EXAMPLE_CONFIG% missing; cannot bootstrap config.
    exit /b 1
)
echo [run-source] bootstrapping config.yaml from config.example.yaml
copy /Y "%CLI_PROXY_EXAMPLE_CONFIG%" "%CLI_PROXY_CONFIG%" >nul
if errorlevel 1 (
    echo [run-source] ERROR: failed to copy config.example.yaml to %CLI_PROXY_CONFIG%
    exit /b 1
)
exit /b 0

:ensure_port_available
set "PORT_PID="
set "PORT_NAME="
set "PORT_COMMAND="
for /f "usebackq tokens=1,* delims=|" %%A in (`powershell -NoProfile -ExecutionPolicy Bypass -Command "$conn = Get-NetTCPConnection -LocalPort $env:CLI_PROXY_PORT -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1; if (-not $conn) { exit 1 }; $p = Get-CimInstance Win32_Process -Filter ('ProcessId=' + $conn.OwningProcess) -ErrorAction SilentlyContinue; $name = if ($p) { $p.Name } else { '' }; $cmd = if ($p) { $p.CommandLine } else { '' }; Write-Output ([string]$conn.OwningProcess + '|' + $name + '|' + $cmd)"`) do (
    set "PORT_PID=%%A"
    set "PORT_DETAIL=%%B"
)
if "%PORT_PID%"=="" exit /b 0
for /f "tokens=1,* delims=|" %%A in ("!PORT_DETAIL!") do (
    set "PORT_NAME=%%A"
    set "PORT_COMMAND=%%B"
)

echo [run-source] port %CLI_PROXY_PORT% is already in use.
echo [run-source] pid: !PORT_PID!
if not "!PORT_NAME!"=="" echo [run-source] process: !PORT_NAME!
if not "!PORT_COMMAND!"=="" echo [run-source] command: !PORT_COMMAND!

if defined CLI_PROXY_KILL_PORT goto kill_port_process

set "KILL_CHOICE="
set /p KILL_CHOICE=Stop this process and start CLIProxyAPI? [y/N]: 
if /i not "!KILL_CHOICE!"=="Y" (
    echo [run-source] start canceled because port %CLI_PROXY_PORT% is in use.
    exit /b 1
)

:kill_port_process
echo [run-source] stopping pid !PORT_PID! ...
taskkill /PID !PORT_PID! /F >nul
if errorlevel 1 (
    echo [run-source] ERROR: failed to stop pid !PORT_PID!.
    exit /b 1
)
exit /b 0

:ensure_login
set "PROVIDER=%~1"
set "LOGIN_FLAG="
if /i "%PROVIDER%"=="gemini"       set "LOGIN_FLAG=-login"
if /i "%PROVIDER%"=="codex"        set "LOGIN_FLAG=-codex-login"
if /i "%PROVIDER%"=="codex-device" set "LOGIN_FLAG=-codex-device-login"
if /i "%PROVIDER%"=="claude"       set "LOGIN_FLAG=-claude-login"
if /i "%PROVIDER%"=="antigravity"  set "LOGIN_FLAG=-antigravity-login"
if /i "%PROVIDER%"=="kimi"         set "LOGIN_FLAG=-kimi-login"
if "%LOGIN_FLAG%"=="" (
    echo [run-source] ERROR: unknown provider "%PROVIDER%"
    exit /b 1
)

set "TYPE_HINT=%PROVIDER%"
if /i "%PROVIDER%"=="codex-device" set "TYPE_HINT=codex"

call :has_auth_for "%TYPE_HINT%"
if not errorlevel 1 (
    echo [run-source] existing %TYPE_HINT% auth detected, skipping login.
    exit /b 0
)

set "EXTRA_ARGS="
if defined CLI_PROXY_NO_BROWSER set "EXTRA_ARGS=-no-browser"
echo [run-source] no %TYPE_HINT% auth found, launching `go run ./cmd/server %LOGIN_FLAG% %EXTRA_ARGS%`
go run ./cmd/server --config "%CLI_PROXY_CONFIG%" %LOGIN_FLAG% %EXTRA_ARGS%
if errorlevel 1 (
    echo [run-source] ERROR: login flow exited with non-zero status.
    exit /b 1
)
exit /b 0

:has_auth_for
rem Returns 0 if at least one *.json file under CLI_PROXY_AUTH_DIR has "type": "<arg>".
set "TARGET_TYPE=%~1"
if not exist "%CLI_PROXY_AUTH_DIR%" exit /b 1
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$dir = $env:CLI_PROXY_AUTH_DIR; $target = $env:TARGET_TYPE;" ^
  "if (-not (Test-Path -LiteralPath $dir)) { exit 1 };" ^
  "$files = Get-ChildItem -LiteralPath $dir -Filter *.json -File -ErrorAction SilentlyContinue;" ^
  "foreach ($f in $files) {" ^
  "  try { $obj = Get-Content -LiteralPath $f.FullName -Raw -ErrorAction Stop | ConvertFrom-Json -ErrorAction Stop } catch { continue };" ^
  "  if ($obj -and $obj.type -and ($obj.type.ToString().ToLower() -eq $target.ToLower())) { exit 0 };" ^
  "}; exit 1"
exit /b %ERRORLEVEL%
