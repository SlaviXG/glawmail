@echo off
setlocal

set GLAWMAIL_DIR=%~dp0
set GLAWMAIL_EXE=%GLAWMAIL_DIR%glawmaild.exe

if "%1"=="up" goto start
if "%1"=="start" goto start
if "%1"=="down" goto stop
if "%1"=="stop" goto stop
if "%1"=="build" goto build
if "%1"=="install" goto install
if "%1"=="uninstall" goto uninstall
if "%1"=="status" goto status
goto usage

:start
taskkill /IM glawmaild.exe /F >nul 2>&1
start "GlawMail" /B /D "%GLAWMAIL_DIR%" "%GLAWMAIL_EXE%"
echo GlawMail started
goto end

:stop
taskkill /IM glawmaild.exe /F >nul 2>&1
echo GlawMail stopped
goto end

:status
tasklist /FI "IMAGENAME eq glawmaild.exe" | find "glawmaild.exe" >nul
if %errorlevel%==0 (
    echo GlawMail is running
) else (
    echo GlawMail is not running
)
goto end

:build
echo Building...
go build -o "%GLAWMAIL_EXE%" ./cmd/glawmail
echo Done: %GLAWMAIL_EXE%
goto end

:install
echo Building...
go build -o "%GLAWMAIL_EXE%" ./cmd/glawmail

echo Setting up auto-start...
schtasks /delete /tn "GlawMail" /f >nul 2>&1
schtasks /create /tn "GlawMail" /tr "\"%GLAWMAIL_EXE%\"" /sc onstart /ru SYSTEM /f
if %errorlevel%==0 (
    echo Auto-start enabled (runs at system startup)
) else (
    echo Warning: Could not set auto-start. Run as Administrator.
)

echo.
echo Install complete. Run: glawmail.bat up
goto end

:uninstall
echo Stopping...
taskkill /IM glawmaild.exe /F >nul 2>&1
echo Removing auto-start...
schtasks /delete /tn "GlawMail" /f >nul 2>&1
echo Uninstalled
goto end

:usage
echo Usage: glawmail {up^|down^|status^|build^|install^|uninstall}
goto end

:end
endlocal
