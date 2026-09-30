@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0"

where go >nul 2>nul
if errorlevel 1 (
  echo [错误] 未找到 Go，请先安装 Go 1.22+：https://go.dev/dl/
  exit /b 1
)

where wails >nul 2>nul
if errorlevel 1 (
  echo [1/4] 未找到 Wails CLI，正在安装...
  go install github.com/wailsapp/wails/v2/cmd/wails@latest || exit /b 1
  for /f "delims=" %%i in ('go env GOPATH') do set "PATH=%%i\bin;%PATH%"
) else (
  echo [1/4] 已安装 Wails CLI
)

echo [2/4] 下载依赖 (go mod tidy)...
go mod tidy || exit /b 1

echo [3/4] 运行单元测试...
go test ./internal/... || exit /b 1

echo [4/4] 打包 exe...
wails build -clean -trimpath || exit /b 1

echo.
echo 完成：%~dp0build\bin\ntrip-tester.exe
endlocal
