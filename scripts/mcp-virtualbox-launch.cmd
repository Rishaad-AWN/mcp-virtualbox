@echo off
setlocal
rem Windows launcher for the mcp-virtualbox Claude Code plugin.
rem Uses a mcp-virtualbox binary from PATH if present; otherwise downloads the
rem matching prebuilt release binary into the plugin's bin\ directory (cached).
rem No Go toolchain required. Needs curl.exe and tar.exe (bundled with Windows 10+).

where mcp-virtualbox >nul 2>&1
if %ERRORLEVEL%==0 (
	mcp-virtualbox %*
	exit /b %ERRORLEVEL%
)

set "ROOT=%CLAUDE_PLUGIN_ROOT%"
if "%ROOT%"=="" set "ROOT=%~dp0.."
set "ARCH=amd64"
if /I "%PROCESSOR_ARCHITECTURE%"=="ARM64" set "ARCH=arm64"
set "BIN=%ROOT%\bin\mcp-virtualbox.exe"

if not exist "%BIN%" (
	if not exist "%ROOT%\bin" mkdir "%ROOT%\bin"
	echo mcp-virtualbox: downloading prebuilt binary ^(windows/%ARCH%^)... 1>&2
	curl -fsSL "https://github.com/Rishaad-AWN/mcp-virtualbox/releases/latest/download/mcp-virtualbox_windows_%ARCH%.zip" -o "%TEMP%\mcpvb.zip"
	if errorlevel 1 (
		echo mcp-virtualbox: download failed 1>&2
		echo Install manually: go install github.com/Rishaad-AWN/mcp-virtualbox@latest 1>&2
		exit /b 1
	)
	tar -xf "%TEMP%\mcpvb.zip" -C "%ROOT%\bin"
	del "%TEMP%\mcpvb.zip" >nul 2>&1
)

"%BIN%" %*
