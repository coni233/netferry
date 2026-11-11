@echo off
setlocal
cd /d "%~dp0"

if not exist "netferry.exe" (
  echo NetFerry executable not found in this folder.
  pause
  exit /b 1
)

echo NetFerry uninstaller
echo.
echo Previewing uninstall changes...
netferry.exe uninstall --dry-run
if errorlevel 1 goto fail
echo.

choice /C YN /N /M "Remove NetFerry startup and managed environment? [Y/N] "
if errorlevel 2 goto cancelled
if errorlevel 1 goto uninstall

:uninstall
netferry.exe uninstall
if errorlevel 1 goto fail
echo.
echo NetFerry uninstall completed.
echo Restart Agent applications and terminals to clear inherited settings.
pause
exit /b 0

:cancelled
echo Uninstall cancelled.
pause
exit /b 0

:fail
echo.
echo NetFerry uninstall failed. Review the messages above.
pause
exit /b 1
