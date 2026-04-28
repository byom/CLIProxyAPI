@echo off
setlocal EnableDelayedExpansion

rem -----------------------------------------------------------------------------
rem CLIProxyAPI one-shot test launcher for Windows (Go + browser UI).
rem
rem Responsibilities:
rem   1. Ensure the Go toolchain is available (reuse the portable install from
rem      run-source.bat when the repo already has `.tools\go`).
rem   2. Run the offline unit tests: go test ./internal/modeltest/...
rem      ./internal/api/handlers/management/...
rem   3. Open the Model Tests dashboard in the default browser so the user can
rem      drive the live tests interactively.
rem
rem Flags:
rem   Default (no flags): probe the server; if it is not listening, launch
rem                       run-source.bat --no-menu in a new window and wait
rem                       for readiness. Then open the dashboard in the default
rem                       browser with ?apiKey=sk-firefly seeded. Skip `go test`.
rem   --with-go-tests             Also run `go test` before opening the browser.
rem   --offline-only              Only run `go test`, do not open the browser.
rem   --probe                     Probe /healthz (default).
rem   --no-probe                  Skip the probe and open the URL unconditionally.
rem   --start-server              Force auto-launch run-source.bat --no-menu
rem                               in a new window even if the probe succeeds.
rem                               (Usually you do not need this flag; auto-start
rem                                is already the default when the probe fails.)
rem   --no-start-server           If the probe fails, do not auto-start the
rem                               server; exit with an error instead.
rem   --api-key <key>             Upstream API key to seed into the dashboard
rem                               (default: sk-firefly). Use --no-default-key
rem                               to omit it.
rem   --no-default-key            Do not append ?apiKey=... to the URL.
rem   --mgmt-key <key>            Management key to seed into the dashboard.
rem   --base-url <url>            Override the base URL passed to the browser
rem                               (default: http://127.0.0.1:8317).
rem   --path <path>               Override the dashboard path (default:
rem                               /management/model-tests).
rem   --server-timeout <seconds>  Wait for the server after auto-start
rem                               (default 90).
rem   --verbose                   Pass `-v` to `go test`.
rem -----------------------------------------------------------------------------

set "SCRIPT_DIR=%~dp0"
set "CLI_PROXY_LOCAL_GO=%SCRIPT_DIR%.tools\go"

set "RUN_GO_TESTS=0"
set "OPEN_BROWSER=1"
set "GO_TEST_VERBOSE="
set "BASE_URL=http://127.0.0.1:8317"
set "DASHBOARD_PATH=/management/model-tests"
set "SKIP_PROBE=0"
set "AUTO_START_SERVER=1"
set "FORCE_START_SERVER=0"
set "SERVER_READY_TIMEOUT=90"
set "DEFAULT_API_KEY=sk-firefly"
set "DEFAULT_MGMT_KEY=cliproxy-dev-mgmt"
set "SEED_API_KEY=%DEFAULT_API_KEY%"
set "SEED_MGMT_KEY=%DEFAULT_MGMT_KEY%"

:parse_args
if "%~1"=="" goto args_done
if /i "%~1"=="--with-go-tests"    (set "RUN_GO_TESTS=1"& shift & goto parse_args)
if /i "%~1"=="--offline-only"     (set "RUN_GO_TESTS=1"& set "OPEN_BROWSER=0"& shift & goto parse_args)
if /i "%~1"=="--no-open"          (set "OPEN_BROWSER=0"& shift & goto parse_args)
if /i "%~1"=="--open-only"        (set "RUN_GO_TESTS=0"& shift & goto parse_args)
if /i "%~1"=="--no-go-tests"      (set "RUN_GO_TESTS=0"& shift & goto parse_args)
if /i "%~1"=="--probe"            (set "SKIP_PROBE=0"& shift & goto parse_args)
if /i "%~1"=="--no-probe"         (set "SKIP_PROBE=1"& shift & goto parse_args)
if /i "%~1"=="--start-server"     (set "AUTO_START_SERVER=1"& set "FORCE_START_SERVER=1"& set "SKIP_PROBE=0"& shift & goto parse_args)
if /i "%~1"=="--no-start-server"  (set "AUTO_START_SERVER=0"& shift & goto parse_args)
if /i "%~1"=="--verbose"          (set "GO_TEST_VERBOSE=-v"& shift & goto parse_args)
if /i "%~1"=="--no-default-key"      (set "SEED_API_KEY="& shift & goto parse_args)
if /i "%~1"=="--no-default-mgmt-key" (set "SEED_MGMT_KEY="& shift & goto parse_args)
if /i "%~1"=="--api-key" (
    if "%~2"=="" (echo [run-tests] --api-key requires a value & exit /b 2)
    set "SEED_API_KEY=%~2"
    shift & shift & goto parse_args
)
if /i "%~1"=="--mgmt-key" (
    if "%~2"=="" (echo [run-tests] --mgmt-key requires a value & exit /b 2)
    set "SEED_MGMT_KEY=%~2"
    shift & shift & goto parse_args
)
if /i "%~1"=="--base-url" (
    if "%~2"=="" (echo [run-tests] --base-url requires a value & exit /b 2)
    set "BASE_URL=%~2"
    shift & shift & goto parse_args
)
if /i "%~1"=="--path" (
    if "%~2"=="" (echo [run-tests] --path requires a value & exit /b 2)
    set "DASHBOARD_PATH=%~2"
    shift & shift & goto parse_args
)
if /i "%~1"=="--server-timeout" (
    if "%~2"=="" (echo [run-tests] --server-timeout requires a value & exit /b 2)
    set "SERVER_READY_TIMEOUT=%~2"
    shift & shift & goto parse_args
)
echo [run-tests] unknown argument: %~1
echo Usage: run-tests.bat [--with-go-tests^|--offline-only]
echo                      [--no-probe] [--start-server^|--no-start-server]
echo                      [--api-key KEY^|--no-default-key] [--mgmt-key KEY]
echo                      [--base-url URL] [--path PATH]
echo                      [--server-timeout SECONDS] [--verbose]
exit /b 2

:args_done

if "%RUN_GO_TESTS%"=="1" (
    call :locate_go || exit /b 1
    call :run_go_tests
    if errorlevel 1 (
        echo.
        echo [run-tests] go test failed; skipping browser launch.
        exit /b 1
    )
)

if "%OPEN_BROWSER%"=="1" (
    call :open_dashboard
    if errorlevel 1 exit /b 1
)

echo.
echo [run-tests] done.
exit /b 0

rem -------------------- helpers --------------------

:locate_go
where go >nul 2>nul
if not errorlevel 1 exit /b 0
if exist "%CLI_PROXY_LOCAL_GO%\bin\go.exe" (
    set "PATH=%CLI_PROXY_LOCAL_GO%\bin;%PATH%"
    where go >nul 2>nul
    if not errorlevel 1 exit /b 0
)
echo [run-tests] ERROR: Go toolchain not found.
echo [run-tests] Install it manually or run `run-source.bat` once to bootstrap a portable copy under .tools\go.
exit /b 1

:run_go_tests
echo.
echo [run-tests] === go test ===
go test %GO_TEST_VERBOSE% -count=1 -timeout=240s ./internal/modeltest/... ./internal/api/handlers/management/...
set "RC=%ERRORLEVEL%"
if not "%RC%"=="0" (
    echo [run-tests] go test failed with exit code %RC%
    exit /b %RC%
)
exit /b 0

:open_dashboard
set "DASHBOARD_URL=%BASE_URL%%DASHBOARD_PATH%"
set "QUERY="
if not "%SEED_API_KEY%"=="" (
    if defined QUERY (set "QUERY=!QUERY!^&apiKey=!SEED_API_KEY!") else (set "QUERY=?apiKey=!SEED_API_KEY!")
)
if not "%SEED_MGMT_KEY%"=="" (
    if defined QUERY (set "QUERY=!QUERY!^&mgmtKey=!SEED_MGMT_KEY!") else (set "QUERY=?mgmtKey=!SEED_MGMT_KEY!")
)
if defined QUERY set "DASHBOARD_URL=!DASHBOARD_URL!!QUERY!"
echo.
echo [run-tests] === live model-tests dashboard ===
echo [run-tests] URL: "!DASHBOARD_URL!"

if "%FORCE_START_SERVER%"=="1" (
    call :start_server_in_new_window
    call :wait_server_ready
    if errorlevel 1 (
        echo [run-tests] ERROR: server did not become ready within %SERVER_READY_TIMEOUT% seconds.
        echo [run-tests]        Check the server window for errors, or rerun with --no-probe to skip the check.
        exit /b 1
    )
    goto launch_browser
)

if "%SKIP_PROBE%"=="1" goto launch_browser

call :probe_server
if not errorlevel 1 (
    call :check_mgmt_routes
    goto launch_browser
)

if "%AUTO_START_SERVER%"=="1" (
    echo [run-tests] server not listening at %BASE_URL%; auto-starting ...
    call :start_server_in_new_window
    call :wait_server_ready
    if errorlevel 1 (
        echo [run-tests] ERROR: server did not become ready within %SERVER_READY_TIMEOUT% seconds.
        echo [run-tests]        Check the server window for errors, or rerun with --no-probe to skip the check.
        exit /b 1
    )
    goto launch_browser
)

echo [run-tests] ERROR: server not reachable at %BASE_URL%.
echo [run-tests]        Start it in another terminal:
echo [run-tests]            .\run-source.bat
echo [run-tests]        Or drop --no-start-server so this script launches it for you,
echo [run-tests]        or pass --no-probe to open the URL anyway.
exit /b 1

:launch_browser
rem `start` with empty title slot opens the default browser. The URL is
rem quoted so `&` inside query strings is not interpreted as a command separator.
start "" "!DASHBOARD_URL!"
if errorlevel 1 (
    echo [run-tests] WARN: could not launch default browser; open this URL manually:
    echo [run-tests]       "!DASHBOARD_URL!"
)
exit /b 0

:probe_server
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "[System.Net.WebRequest]::DefaultWebProxy = $null;" ^
  "try { $r = Invoke-WebRequest -Uri ($env:BASE_URL + '/healthz') -UseBasicParsing -TimeoutSec 3; if ($r.StatusCode -ge 200 -and $r.StatusCode -lt 500) { exit 0 } else { exit 1 } } catch { exit 1 }"
exit /b %ERRORLEVEL%

:check_mgmt_routes
rem Detects whether /v0/management/* is actually registered on the running
rem server. When it returns 404 we stop the stale instance and auto-start a
rem fresh one with MANAGEMENT_PASSWORD set, so the dashboard just works.
set "MGMT_PROBE=%SCRIPT_DIR%.probe-mgmt.ps1"
>"%MGMT_PROBE%" echo [System.Net.WebRequest]::DefaultWebProxy = $null
>>"%MGMT_PROBE%" echo try {
>>"%MGMT_PROBE%" echo   $h = @{}
>>"%MGMT_PROBE%" echo   if ($env:SEED_MGMT_KEY) { $h['Authorization'] = 'Bearer ' + $env:SEED_MGMT_KEY }
>>"%MGMT_PROBE%" echo   $r = Invoke-WebRequest -Uri ($env:BASE_URL + '/v0/management/model-tests/suites') -Headers $h -UseBasicParsing -TimeoutSec 3
>>"%MGMT_PROBE%" echo   if ($r.StatusCode -eq 200) { exit 0 } else { exit 1 }
>>"%MGMT_PROBE%" echo } catch {
>>"%MGMT_PROBE%" echo   $resp = $_.Exception.Response
>>"%MGMT_PROBE%" echo   if ($resp) {
>>"%MGMT_PROBE%" echo     $code = [int]$resp.StatusCode
>>"%MGMT_PROBE%" echo     if ($code -eq 404) { exit 2 }
>>"%MGMT_PROBE%" echo     if ($code -eq 401 -or $code -eq 403) { exit 3 }
>>"%MGMT_PROBE%" echo     exit 1
>>"%MGMT_PROBE%" echo   } else { exit 1 }
>>"%MGMT_PROBE%" echo }
powershell -NoProfile -ExecutionPolicy Bypass -File "%MGMT_PROBE%"
set "MGMT_RC=%ERRORLEVEL%"
del "%MGMT_PROBE%" >nul 2>nul
if "%MGMT_RC%"=="0" exit /b 0
if "%MGMT_RC%"=="2" goto mgmt_restart
if "%MGMT_RC%"=="3" (
    echo [run-tests] WARN: management endpoint returned 401/403.
    echo [run-tests]       Your --mgmt-key does not match the key the running server was started with.
    echo [run-tests]       Either pass the right --mgmt-key, or stop that server window and rerun.
    exit /b 0
)
echo [run-tests] WARN: could not verify /v0/management/* (rc=%MGMT_RC%); continuing anyway.
exit /b 0

:mgmt_restart
echo [run-tests] /v0/management/* is unavailable on the running server.
echo [run-tests] Stopping it and relaunching with MANAGEMENT_PASSWORD ...
call :stop_running_server
call :start_server_in_new_window
call :wait_server_ready
if errorlevel 1 (
    echo [run-tests] ERROR: restarted server did not become ready within %SERVER_READY_TIMEOUT% seconds.
    exit /b 1
)
exit /b 0

:stop_running_server
set "STOP_SCRIPT=%SCRIPT_DIR%.stop-port.ps1"
>"%STOP_SCRIPT%" echo $conns = Get-NetTCPConnection -LocalPort 8317 -State Listen -ErrorAction SilentlyContinue
>>"%STOP_SCRIPT%" echo foreach ($c in $conns) {
>>"%STOP_SCRIPT%" echo   Stop-Process -Id $c.OwningProcess -Force -ErrorAction SilentlyContinue
>>"%STOP_SCRIPT%" echo   Write-Host ('[run-tests] stopped pid=' + $c.OwningProcess)
>>"%STOP_SCRIPT%" echo }
powershell -NoProfile -ExecutionPolicy Bypass -File "%STOP_SCRIPT%"
del "%STOP_SCRIPT%" >nul 2>nul
rem Wait briefly for the socket to free.
powershell -NoProfile -Command "Start-Sleep -Seconds 2"
exit /b 0

:start_server_in_new_window
echo [run-tests] launching run-source.bat --no-menu in a new window ...
if not "%SEED_MGMT_KEY%"=="" (
    echo [run-tests] exporting MANAGEMENT_PASSWORD=%SEED_MGMT_KEY% so /v0/management routes register
    set "MANAGEMENT_PASSWORD=%SEED_MGMT_KEY%"
)
start "CLIProxyAPI" cmd /k ""%SCRIPT_DIR%run-source.bat" --no-menu"
exit /b 0

:wait_server_ready
echo [run-tests] waiting for %BASE_URL%/healthz to respond (timeout=%SERVER_READY_TIMEOUT%s) ...
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "[System.Net.WebRequest]::DefaultWebProxy = $null;" ^
  "$deadline = (Get-Date).AddSeconds([int]$env:SERVER_READY_TIMEOUT);" ^
  "while ((Get-Date) -lt $deadline) {" ^
  "  try { $r = Invoke-WebRequest -Uri ($env:BASE_URL + '/healthz') -UseBasicParsing -TimeoutSec 2;" ^
  "    if ($r.StatusCode -ge 200 -and $r.StatusCode -lt 500) { Write-Host '[run-tests] server is ready.'; exit 0 } } catch {};" ^
  "  Start-Sleep -Milliseconds 750 }; exit 1"
exit /b %ERRORLEVEL%
