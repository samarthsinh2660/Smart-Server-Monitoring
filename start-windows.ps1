param(
    [ValidateSet('primary', 'standby')][string]$Role = 'primary',
    [switch]$Presentation,
    [string]$PiAddress,
    [switch]$SkipBuild
)
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
if ($Presentation -and [string]::IsNullOrWhiteSpace($PiAddress)) { throw 'Pass -PiAddress with the Pi LAN address for presentation mode.' }
if ($Presentation -and $Role -ne 'primary') { throw 'Presentation mode runs on the primary laptop.' }
New-Item -ItemType Directory -Force bin/windows-amd64, .logs | Out-Null
if (-not $SkipBuild -and (Get-Command go -ErrorAction SilentlyContinue)) {
    $env:CGO_ENABLED = '0'
    foreach ($service in @('agent', 'demoapp')) {
        & go build -o "bin/windows-amd64/$service.exe" "./cmd/$service"
        if ($LASTEXITCODE -ne 0) { throw "Build failed for $service" }
    }
    if ($Presentation) {
        & go build -o bin/windows-amd64/traffic-lab.exe ./demo/traffic-lab
        if ($LASTEXITCODE -ne 0) { throw 'Traffic Lab build failed.' }
    }
}
foreach ($service in @('agent', 'demoapp')) {
    if (-not (Test-Path "bin/windows-amd64/$service.exe")) { throw "Missing $service.exe. Install Go and run again, or copy the Windows binaries built on Linux." }
}
if ($Presentation -and -not (Test-Path bin/windows-amd64/traffic-lab.exe)) { throw 'Missing traffic-lab.exe; build or copy the Windows binaries first.' }
if ($Presentation) {
    if (-not (Get-Command npm.cmd -ErrorAction SilentlyContinue)) { throw 'Install Node.js 20.9 or newer (current LTS recommended) for the dashboard.' }
    if (-not (Test-Path web/node_modules/@next/swc-win32-x64-msvc)) {
        Push-Location web
        try { & npm.cmd ci; if ($LASTEXITCODE -ne 0) { throw 'npm ci failed.' } } finally { Pop-Location }
    }
}
$env:SERVER_NAME = $Role
$env:AGENT_ADDR = '0.0.0.0:8081'
$env:DEMOAPP_ADDR = '0.0.0.0:8000'
if (-not $env:METRICS_DISK_PATH) { $env:METRICS_DISK_PATH = "$env:SystemDrive\" }
if ($Presentation) {
    $env:CONTROLLER_API_URL = "http://${PiAddress}:8080"
    $env:TRAFFIC_LAB_URL = 'http://127.0.0.1:8091'
    $env:TRAFFIC_LAB_TARGET = "http://${PiAddress}:8088"
}
$children = @()
$reportedFailures = @{}
try {
    foreach ($service in @('agent', 'demoapp')) {
        $children += Start-Process -FilePath "$PSScriptRoot/bin/windows-amd64/$service.exe" -WorkingDirectory $PSScriptRoot -NoNewWindow -PassThru -RedirectStandardOutput ".logs/$Role-$service.log" -RedirectStandardError ".logs/$Role-$service.err.log"
    }
    if ($Presentation) {
        $children += Start-Process -FilePath "$PSScriptRoot/bin/windows-amd64/traffic-lab.exe" -WorkingDirectory $PSScriptRoot -NoNewWindow -PassThru -RedirectStandardOutput .logs/traffic-lab.log -RedirectStandardError .logs/traffic-lab.err.log
        $children += Start-Process -FilePath $env:ComSpec -ArgumentList @('/d', '/c', 'npm run dev') -WorkingDirectory "$PSScriptRoot/web" -NoNewWindow -PassThru -RedirectStandardOutput "$PSScriptRoot/.logs/dashboard.log" -RedirectStandardError "$PSScriptRoot/.logs/dashboard.err.log"
        Write-Host 'Dashboard: http://127.0.0.1:3000. Traffic goes through the Pi proxy.'
    }
    Write-Host "Windows $Role app and metrics agent started. Logs: $PSScriptRoot/.logs. Ctrl+C stops these processes."
    while ($true) {
        foreach ($child in $children) {
            $child.Refresh()
            if ($child.HasExited) {
                if (-not $Presentation) { throw "A service exited (PID $($child.Id)); check .logs before restarting." }
                if (-not $reportedFailures.ContainsKey($child.Id)) {
                    Write-Warning "Service PID $($child.Id) exited; check .logs. Other presentation services stay running so app failure can be observed. Ctrl+C stops the launcher."
                    $reportedFailures[$child.Id] = $true
                }
            }
        }
        Start-Sleep -Seconds 1
    }
} finally {
    foreach ($child in $children) {
        $child.Refresh()
        if (-not $child.HasExited) { & taskkill.exe /PID $child.Id /T /F 2>$null | Out-Null }
    }
}
