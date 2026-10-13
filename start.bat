@echo off
rem Use UTF-8 in the console for consistent output.
chcp 65001 >nul 2>&1
rem ARTEX supervisor launcher (Windows)
rem
rem Usage:
rem   start.bat                  Run in the foreground (Ctrl-C to stop)
rem   start.bat -addr :9000      Pass additional arguments through to artex
rem
rem Starts artex.exe and uses its exit code to decide whether to restart it.
rem
rem   0      Normal stop requested by user -> exit loop
rem   75     Restart requested             -> restart immediately (update or rollback)
rem   other  Crash                         -> restart with backoff (1->2->4...up to 60 seconds)
rem
rem Downloads, SHA256 verification, and binary replacement are handled by artex at startup
rem (selfupdate package). Keep this script simple; see the comments at the top of start.sh.

setlocal enabledelayedexpansion
cd /d "%~dp0"

set "BIN=artex.exe"
if not exist "%BIN%" (
	echo [artex] Executable not found: %BIN% 1>&2
	exit /b 1
)

set "RESTART_CODE=75"
set "MAX_DELAY=60"
set /a delay=1

:loop
"%BIN%" %*
set "code=!ERRORLEVEL!"

if "!code!"=="0" (
	echo [artex] Exited normally
	exit /b 0
)

if "!code!"=="%RESTART_CODE%" (
	rem Update/rollback staged: artex will apply it during startup.
	echo [artex] Restart requested (applying update)...
	set /a delay=1
	goto loop
)

echo [artex] Exited unexpectedly ^(code=!code!^); restarting in !delay!s 1>&2
rem timeout may fail when console output is redirected; use ping as a fallback (N+1 pings for N seconds).
set /a pings=!delay!+1
ping -n !pings! 127.0.0.1 >nul 2>&1
set /a delay=!delay!*2
if !delay! gtr %MAX_DELAY% set /a delay=%MAX_DELAY%
goto loop
