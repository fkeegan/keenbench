$ErrorActionPreference = "Stop"

$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
$Arch = if ($env:KEENBENCH_WINDOWS_ARCH) { $env:KEENBENCH_WINDOWS_ARCH } else { "x64" }
$ReleaseDir = Join-Path $Root "app\build\windows\$Arch\runner\Release"
$InstallerScript = Join-Path $Root "installer\keenbench.nsi"

function Require-Command($Name, $Hint) {
  $cmd = Get-Command $Name -ErrorAction SilentlyContinue
  if (-not $cmd) {
    throw "$Name not found. $Hint"
  }
  return $cmd.Source
}

Write-Host "==> Building Flutter Windows app ($Arch)..."
Push-Location (Join-Path $Root "app")
& flutter build windows --release
Pop-Location

if (-not (Test-Path $ReleaseDir)) {
  throw "Flutter build output not found: $ReleaseDir"
}

Write-Host "==> Building Go engine..."
Push-Location (Join-Path $Root "engine")
& go build -o (Join-Path $ReleaseDir "keenbench-engine.exe") ".\cmd\keenbench-engine"
Pop-Location

if (Test-Path (Join-Path $ReleaseDir "keenbench-engine.exe")) {
  Move-Item -Force (Join-Path $ReleaseDir "keenbench-engine.exe") (Join-Path $ReleaseDir "keenbench-engine")
}

Write-Host "==> Building Python tool worker (PyInstaller)..."
$PyWorkerDir = Join-Path $Root "engine\tools\pyworker"
Push-Location $PyWorkerDir
if (-not (Test-Path ".\.venv")) {
  & py -3 -m venv .venv
}
& .\.venv\Scripts\pip install -r requirements.txt
& .\.venv\Scripts\pip install pyinstaller
& .\.venv\Scripts\pyinstaller --onefile --name keenbench-tool-worker worker.py
Pop-Location

$WorkerExe = Join-Path $PyWorkerDir "dist\keenbench-tool-worker.exe"
if (-not (Test-Path $WorkerExe)) {
  throw "PyInstaller output not found: $WorkerExe"
}
Copy-Item -Force $WorkerExe (Join-Path $ReleaseDir "keenbench-tool-worker.exe")
Move-Item -Force (Join-Path $ReleaseDir "keenbench-tool-worker.exe") (Join-Path $ReleaseDir "keenbench-tool-worker")

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

Push-Location $Root
& $Nsis $InstallerScript
Pop-Location

Write-Host "==> Done. Installer created in repo root as KeenBench-Setup.exe"
