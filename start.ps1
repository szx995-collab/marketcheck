param([switch]$NoBrowser)
$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
$env:PYTHONUTF8 = '1'
if (-not $env:MARKETCHECK_PORT) { $env:MARKETCHECK_PORT = '8765' }
$marketUrl = "http://127.0.0.1:$($env:MARKETCHECK_PORT)"
try {
    $marketExisting = Invoke-RestMethod ($marketUrl + '/api/bootstrap') -TimeoutSec 2
    if ($marketExisting.settings -and $marketExisting.catalog) {
        Write-Host "MarketCheck is already running at $marketUrl"
        if (-not $NoBrowser) { Start-Process $marketUrl -WindowStyle Hidden }
        exit 0
    }
} catch { }
$env:GOCACHE = Join-Path $PSScriptRoot '.tools\go-cache'
$env:GOPATH = Join-Path $PSScriptRoot '.tools\go-path'
$marketGo = Join-Path $PSScriptRoot '.tools\go\bin\go.exe'
if (-not (Test-Path -LiteralPath $marketGo)) {
    $goCommand = Get-Command go -ErrorAction SilentlyContinue
    if (-not $goCommand) { throw 'Go is missing. Run setup.ps1 first.' }
    $marketGo = $goCommand.Source
}
New-Item -ItemType Directory -Path (Join-Path $PSScriptRoot '.local') -Force | Out-Null
& $marketGo build -o .local/marketcheck.exe .
if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
Write-Host "Open $marketUrl"
if (-not $NoBrowser) { Start-Process $marketUrl -WindowStyle Hidden }
& .local/marketcheck.exe
