$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
$env:PYTHONUTF8 = '1'
New-Item -ItemType Directory -Path .tools -Force | Out-Null
$marketGo = Join-Path $PSScriptRoot '.tools\go\bin\go.exe'
if (-not (Test-Path -LiteralPath $marketGo) -and -not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host 'Downloading the official Go toolchain into .tools ...'
    $marketRelease = (Invoke-RestMethod 'https://go.dev/dl/?mode=json' | Where-Object stable | Select-Object -First 1)
    $marketPackage = $marketRelease.files | Where-Object { $_.os -eq 'windows' -and $_.arch -eq 'amd64' -and $_.kind -eq 'archive' } | Select-Object -First 1
    if (-not $marketPackage) { throw 'No Windows x64 Go package was found.' }
    $marketArchive = Join-Path $PSScriptRoot ('.tools\' + $marketPackage.filename)
    Invoke-WebRequest ('https://go.dev/dl/' + $marketPackage.filename) -OutFile $marketArchive
    if ((Get-FileHash -LiteralPath $marketArchive -Algorithm SHA256).Hash.ToLower() -ne $marketPackage.sha256) { throw 'Go checksum mismatch.' }
    Expand-Archive -LiteralPath $marketArchive -DestinationPath .tools -Force
    Remove-Item -LiteralPath $marketArchive
}
$marketPython = $null
$marketSavedPython = Join-Path $PSScriptRoot '.tools\python-path.txt'
if (Test-Path -LiteralPath $marketSavedPython) {
    $marketCandidate = (Get-Content -LiteralPath $marketSavedPython -Raw).Trim()
    if (Test-Path -LiteralPath $marketCandidate) { $marketPython = $marketCandidate }
}
if (-not $marketPython) {
    foreach ($marketName in @('python','python3')) {
        $marketCommand = Get-Command $marketName -ErrorAction SilentlyContinue
        if ($marketCommand -and $marketCommand.Source -notlike '*WindowsApps*') { $marketPython = $marketCommand.Source; break }
    }
}
if (-not $marketPython -and (Get-Command py -ErrorAction SilentlyContinue)) {
    $marketPython = (& py -3 -c 'import sys;print(sys.executable)').Trim()
}
if (-not $marketPython) { throw 'Install Python 3.11+ from python.org, enable Add Python to PATH, then run setup.ps1 again.' }
& $marketPython -c 'import sys; assert sys.version_info >= (3,11), "Python 3.11+ is required"'
if ($LASTEXITCODE -ne 0) { throw 'Python 3.11+ is required.' }
if ((Test-Path -LiteralPath '.tools\python-packages') -and (Test-Path -LiteralPath $marketSavedPython)) {
    Write-Host 'Checking existing local analysis packages ...'
    $marketHealthText = '{"op":"health"}' | & $marketPython analysis/engine.py
    $marketHealth = $marketHealthText | ConvertFrom-Json
    if ($LASTEXITCODE -eq 0 -and $marketHealth.ok) { Write-Host 'Ready. Run start.ps1 or start.bat.'; exit 0 }
}
if (-not (Test-Path -LiteralPath '.venv\Scripts\python.exe')) {
    & $marketPython -m venv .venv
    if ($LASTEXITCODE -ne 0) { throw 'Unable to create the Python virtual environment.' }
}
$marketVenv = Join-Path $PSScriptRoot '.venv\Scripts\python.exe'
Remove-Item Env:PIP_NO_INDEX -ErrorAction SilentlyContinue
& $marketVenv -m pip install -r requirements.txt
if ($LASTEXITCODE -ne 0) { throw 'Dependency installation failed. Check network access to pypi.org.' }
Write-Host 'Ready. Run start.ps1 or start.bat.'
