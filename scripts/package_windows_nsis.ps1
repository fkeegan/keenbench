$ErrorActionPreference = "Stop"

$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
$Arch = if ($env:KEENBENCH_WINDOWS_ARCH) { $env:KEENBENCH_WINDOWS_ARCH } else { "" }
$InstallerScript = Join-Path $Root "installer\keenbench.nsi"
$IconPath = Join-Path $Root "app\windows\runner\resources\app_icon.ico"
$PubspecPath = Join-Path $Root "app\pubspec.yaml"

function Resolve-Arch {
  param([string]$RootPath, [string]$Preferred)
  if ($Preferred) { return $Preferred }
  $x64Dir = Join-Path $RootPath "app\build\windows\x64\runner\Release"
  $armDir = Join-Path $RootPath "app\build\windows\arm64\runner\Release"
  $hasX64 = Test-Path $x64Dir
  $hasArm = Test-Path $armDir
  if ($hasX64 -and -not $hasArm) { return "x64" }
  if ($hasArm -and -not $hasX64) { return "arm64" }
  if ($hasX64 -and $hasArm) {
    throw "Both x64 and arm64 builds exist. Set KEENBENCH_WINDOWS_ARCH to choose."
  }
  return "x64"
}

function Read-AppVersion {
  param([string]$Path)
  if (-not (Test-Path $Path)) { return "0.0.0" }
  $line = Select-String -Path $Path -Pattern "^version:" -SimpleMatch | Select-Object -First 1
  if (-not $line) { return "0.0.0" }
  $raw = ($line.Line -replace "^version:\\s*", "").Trim()
  if ($raw -match "^[0-9]+\\.[0-9]+\\.[0-9]+") {
    return ($raw -split "\\+")[0]
  }
  return "0.0.0"
}

function Require-Command($Name, $Hint) {
  $cmd = Get-Command $Name -ErrorAction SilentlyContinue
  if (-not $cmd) {
    throw "$Name not found. $Hint"
  }
  return $cmd.Source
}

Write-Host "==> Building Flutter Windows app..."
Push-Location (Join-Path $Root "app")
& flutter build windows --release
Pop-Location

$Arch = Resolve-Arch -RootPath $Root -Preferred $Arch
$ReleaseDir = Join-Path $Root "app\build\windows\$Arch\runner\Release"
$AppVersion = Read-AppVersion -Path $PubspecPath

if (-not (Test-Path $ReleaseDir)) {
  throw "Flutter build output not found: $ReleaseDir"
}

Write-Host "==> Building Go engine..."
Push-Location (Join-Path $Root "engine")
& go build -ldflags "-H=windowsgui" -o (Join-Path $ReleaseDir "keenbench-engine.exe") ".\cmd\keenbench-engine"
Pop-Location

Write-Host "==> Building Python tool worker (PyInstaller)..."
$PyWorkerDir = Join-Path $Root "engine\tools\pyworker"
Push-Location $PyWorkerDir
if (-not (Test-Path ".\.venv")) {
  & py -3 -m venv .venv
}
& .\.venv\Scripts\pip install -r requirements.txt
& .\.venv\Scripts\pip install pyinstaller
& .\.venv\Scripts\pyinstaller --onefile --noconsole --name keenbench-tool-worker worker.py
Pop-Location

$WorkerExe = Join-Path $PyWorkerDir "dist\keenbench-tool-worker.exe"
if (-not (Test-Path $WorkerExe)) {
  throw "PyInstaller output not found: $WorkerExe"
}
Copy-Item -Force $WorkerExe (Join-Path $ReleaseDir "keenbench-tool-worker.exe")

if (-not (Test-Path $InstallerScript)) {
  throw "NSIS script not found: $InstallerScript"
}

Write-Host "==> Building NSIS installer..."
$Nsis = if ($env:NSIS_PATH) { $env:NSIS_PATH } else { "" }
if (-not $Nsis) {
  $cmd = Get-Command makensis -ErrorAction SilentlyContinue
  if ($cmd) {
    $Nsis = $cmd.Source
  } elseif (Test-Path "C:\Program Files (x86)\NSIS\makensis.exe") {
    $Nsis = "C:\Program Files (x86)\NSIS\makensis.exe"
  } else {
    throw "makensis not found. Set NSIS_PATH or install NSIS."
  }
}

if (-not (Test-Path $IconPath)) {
  throw "App icon not found: $IconPath"
}

Push-Location $Root
& $Nsis `
  "/DAPP_VERSION=$AppVersion" `
  "/DAPP_ICON=$IconPath" `
  "/DAPP_BUILD_DIR=$ReleaseDir" `
  $InstallerScript
Pop-Location

Write-Host "==> Done. Installer created in repo root as KeenBench-Setup.exe"
