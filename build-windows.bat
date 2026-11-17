@echo off
rem Builds nxpress.exe on Windows. Needs Go and a C compiler (MinGW-w64 gcc, 64-bit).
rem   build-windows.bat          build only
rem   build-windows.bat check    also run go vet and go test first
rem Everything it prints is also saved in build-log.txt next to this file. The window stays open.
setlocal
cd /d "%~dp0"
set "LOG=%~dp0build-log.txt"
echo Build started %DATE% %TIME% > "%LOG%"

call :step "Looking for Go" "go version" || goto :failed

rem gcc not on the PATH: try the usual places MinGW-w64 is installed
where gcc >nul 2>nul && goto :havegcc
for %%D in ("C:\msys64\ucrt64\bin" "C:\msys64\mingw64\bin" "C:\mingw64\bin" "C:\mingw32\bin" "C:\TDM-GCC-64\bin" "C:\winlibs\mingw64\bin" "C:\Program Files\mingw-w64\mingw64\bin" "%USERPROFILE%\mingw64\bin" "%USERPROFILE%\scoop\apps\gcc\current\bin" "%USERPROFILE%\scoop\apps\mingw\current\bin" "C:\ProgramData\mingw64\mingw64\bin" "C:\ProgramData\chocolatey\bin") do if exist "%%~D\gcc.exe" set "PATH=%%~D;%PATH%"
:havegcc

call :step "Looking for gcc" "gcc --version" || goto :nogcc

set CGO_ENABLED=1
call :step "Icon and manifest" "go run github.com/tc-hib/go-winres@v0.3.3 make --in build/winres/winres.json --arch amd64" || goto :failed

if /i not "%~1"=="check" goto :build
call :step "go vet" "go vet -tags desktop,production ./..." || goto :failed
call :step "go test" "go test -tags desktop,production ./..." || goto :failed

:build
echo.
echo === Building nxpress.exe
echo === Building nxpress.exe >> "%LOG%"
go build -tags desktop,production -trimpath -ldflags "-s -w -X main.version=1.7.19-details" -o nxpress.exe . >> "%LOG%" 2>&1
if errorlevel 1 goto :buildfailed
echo ok

echo.
echo DONE: %~dp0nxpress.exe
echo.
pause
exit /b 0

:nogcc
echo.
echo gcc (the C compiler) was not found on the PATH or in the usual folders.
echo Find where it is installed (look for a folder with gcc.exe in a "bin" folder,
echo for example C:\msys64\ucrt64\bin), then either add that folder to the PATH,
echo or run this in a Command Prompt in this folder:
echo     set PATH=THAT_FOLDER;%%PATH%%
echo     build-windows.bat
echo If you installed MSYS2 but not gcc, open "MSYS2 UCRT64" and run:
echo     pacman -S mingw-w64-ucrt-x86_64-gcc
echo.
pause
exit /b 1

:buildfailed
echo.
echo --- last lines of the output:
powershell -NoProfile -Command "Get-Content -Tail 40 -LiteralPath $env:LOG"
:failed
echo.
echo FAILED at the step above. The full output is in build-log.txt next to this file.
echo.
pause
exit /b 1

:step
echo.
echo === %~1
echo === %~1 >> "%LOG%"
cmd /c %~2 >> "%LOG%" 2>&1
if errorlevel 1 goto :stepfailed
echo ok
exit /b 0
:stepfailed
echo.
echo --- last lines of the output:
powershell -NoProfile -Command "Get-Content -Tail 25 -LiteralPath $env:LOG"
exit /b 1
