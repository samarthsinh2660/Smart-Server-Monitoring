param([Parameter(Mandatory=$true)][string]$PiAddress)
$ErrorActionPreference = 'Stop'
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
if (-not ([Security.Principal.WindowsPrincipal]$identity).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Run PowerShell as Administrator.' }
$parsedIP = $null
if (-not [Net.IPAddress]::TryParse($PiAddress, [ref]$parsedIP)) { throw 'PiAddress must be the actual Pi IP address.' }
foreach ($service in @('agent', 'demoapp')) {
    if (-not (Test-Path "$PSScriptRoot/bin/windows-amd64/$service.exe")) { throw 'Run or build the Windows app/agent before installing startup.' }
}
foreach ($port in @(8000, 8081)) {
    $name = "SmartServer-Pi-$port"
    Get-NetFirewallRule -Name $name -ErrorAction SilentlyContinue | Remove-NetFirewallRule
    New-NetFirewallRule -Name $name -DisplayName "SmartServer from Pi ($port)" -Direction Inbound -Protocol TCP -LocalPort $port -RemoteAddress $PiAddress -Action Allow -Profile Private | Out-Null
}
$scriptPath = Join-Path $PSScriptRoot 'start-windows.ps1'
$action = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$scriptPath`" -Role standby -SkipBuild" -WorkingDirectory $PSScriptRoot
$trigger = New-ScheduledTaskTrigger -AtStartup
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask -TaskName 'SmartServerStandby' -Action $action -Trigger $trigger -Settings $settings -User 'SYSTEM' -RunLevel Highest -Force | Out-Null
Write-Host 'Standby startup installed. App/agent run after boot without login; wake capability still requires a physical sleep trial.'
Write-Host 'Use a Private profile for this trusted Wi-Fi network. Firewall rules accept app/agent connections only from the Pi.'
Write-Host 'Stop the interactive standby launcher before running Start-ScheduledTask -TaskName SmartServerStandby.'
