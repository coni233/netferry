@echo off
setlocal
cd /d "%~dp0"

if not exist "netferry.exe" (
  echo NetFerry executable not found in this folder.
  pause
  exit /b 1
)

echo NetFerry installer
echo.
echo Start your proxy app before continuing.
echo You do not need to turn on its system proxy setting.
echo.

:detect
echo Checking local proxy ports...
netferry.exe detect
if not errorlevel 1 goto detected
echo.
echo No local proxy detected.
echo Start your proxy app, then press R to retry.
echo You can also run setup manually (replace the port with your proxy port):
echo   netferry.exe setup --upstream http://127.0.0.1:10808
echo.
choice /C RSQ /N /M "Press R to retry, S to skip detection, or Q to quit: "
if errorlevel 3 goto quit_no_proxy
if errorlevel 2 goto skip_detection
goto detect

:skip_detection
set "SETUP_FLAGS=--no-detect"
echo Continuing with the default upstream.
echo You can update it later with:
echo   netferry.exe setup --upstream http://127.0.0.1:10808
echo.

:detected
echo Previewing installation changes...
netferry.exe setup --dry-run %SETUP_FLAGS%
if errorlevel 1 goto fail
echo.

choice /C YN /N /M "Install NetFerry for the current user? [Y/N] "
if errorlevel 2 goto cancelled
if errorlevel 1 goto install

:install
netferry.exe setup %SETUP_FLAGS%
if errorlevel 1 goto fail
echo.
echo Restart Codex, Claude Code, DeepSeek harness, terminals, and IDEs.
echo.
netferry.exe status
pause
exit /b 0

:cancelled
echo Installation cancelled.
pause
exit /b 0

:quit_no_proxy
echo.
echo Installation cancelled. Start your proxy app and run install.bat again.
pause
exit /b 1

:fail
echo.
echo Review the messages above.
pause
exit /b 1
