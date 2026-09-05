$ErrorActionPreference = 'Stop'
$marketExecutable = Join-Path $PSScriptRoot '.local\marketcheck.exe'
$marketProcesses = Get-Process marketcheck -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $marketExecutable }
if ($marketProcesses) {
    $marketProcesses | Stop-Process
    Write-Host 'MarketCheck stopped. A running analysis, if any, will need to be retried.'
} else {
    Write-Host 'MarketCheck is not running from this folder.'
}
