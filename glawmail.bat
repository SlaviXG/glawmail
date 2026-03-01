@echo off
setlocal

set GLAWMAIL_DIR=%~dp0
set GLAWMAIL_EXE=%GLAWMAIL_DIR%glawmail.exe

if "%1"=="up" goto start
if "%1"=="start" goto start
if "%1"=="down" goto stop
if "%1"=="stop" goto stop
if "%1"=="build" goto build
if "%1"=="install" goto install
if "%1"=="status" goto status
goto usage

:start
start "GlawMail" /B "%GLAWMAIL_EXE%"
echo GlawMail started
goto end

:stop
taskkill /IM glawmail.exe /F >nul 2>&1
echo GlawMail stopped
goto end

:status
tasklist /FI "IMAGENAME eq glawmail.exe" | find "glawmail.exe" >nul
if %errorlevel%==0 (
    echo GlawMail is running
) else (
    echo GlawMail is not running
)
goto end

:build
echo Building...
go build -o glawmail.exe ./cmd/glawmail
echo Done: glawmail.exe
goto end

:install
echo Building...
go build -o glawmail.exe ./cmd/glawmail

echo.
echo To auto-start on login, run:
echo   schtasks /create /tn "GlawMail" /tr "%GLAWMAIL_EXE%" /sc onlogon /rl highest
echo.
echo To remove auto-start:
echo   schtasks /delete /tn "GlawMail" /f
echo.
goto end

:usage
echo Usage: glawmail {up^|down^|status^|build^|install}
goto end

:end
endlocal
