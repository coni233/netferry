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
echo Checking known local proxy ports...
netferry.exe detect
echo.
echo Previewing installation changes...
netferry.exe setup --dry-run
if errorlevel 1 goto fail
echo.

choice /C YN /N /M "Install NetFerry for the current user? [Y/N] "
if errorlevel 2 goto cancelled
if errorlevel 1 goto install

:install
netferry.exe setup
if errorlevel 1 goto fail
echo.
echo NetFerry installation completed.
echo Restart Codex, Claude Code, DeepSeek harness, terminals, and IDEs.
echo.
netferry.exe status
pause
exit /b 0

:cancelled
echo Installation cancelled.
pause
exit /b 0

:fail
echo.
echo NetFerry installation failed. Review the messages above.
pause
exit /b 1
