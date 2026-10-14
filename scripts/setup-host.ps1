<#
.SYNOPSIS
    Prepare a Windows host for the Satark on-prem lab.

.DESCRIPTION
    Installs VirtualBox, Vagrant and Git through winget, and enables WSL2 with an
    Ubuntu distribution that acts as the Ansible control node.

    The script is idempotent: packages that are already installed are skipped.
    Run it from an elevated PowerShell prompt. A reboot may be required after
    the WSL features are enabled for the first time.

.PARAMETER UbuntuDistro
    The WSL distribution to install. Defaults to Ubuntu-24.04.

.PARAMETER SkipWsl
    Skip enabling WSL2 and installing the Ubuntu distribution.

.EXAMPLE
    PS> Set-ExecutionPolicy -Scope Process Bypass
    PS> .\scripts\setup-host.ps1
#>
[CmdletBinding()]
param(
    [string]$UbuntuDistro = 'Ubuntu-24.04',
    [switch]$SkipWsl
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Write-Step([string]$Message) {
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Assert-Administrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw 'Run this script from an elevated (Administrator) PowerShell prompt.'
    }
}

function Assert-Winget {
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
        throw 'winget was not found. Install "App Installer" from the Microsoft Store and re-run.'
    }
}

function Install-WingetPackage([string]$Id) {
    $listed = winget list --id $Id --exact --accept-source-agreements 2>$null | Out-String
    if ($LASTEXITCODE -eq 0 -and $listed -match [regex]::Escape($Id)) {
        Write-Host "    $Id is already installed, skipping"
        return
    }
    Write-Step "Installing $Id"
    winget install --id $Id --exact --silent --accept-package-agreements --accept-source-agreements
    if ($LASTEXITCODE -ne 0) {
        throw "winget failed to install $Id (exit code $LASTEXITCODE)"
    }
}

function Enable-Wsl2([string]$Distro) {
    $rebootNeeded = $false
    foreach ($feature in 'Microsoft-Windows-Subsystem-Linux', 'VirtualMachinePlatform') {
        $state = (Get-WindowsOptionalFeature -Online -FeatureName $feature).State
        if ($state -ne 'Enabled') {
            Write-Step "Enabling Windows feature $feature"
            $result = Enable-WindowsOptionalFeature -Online -FeatureName $feature -All -NoRestart
            if ($result.RestartNeeded) { $rebootNeeded = $true }
        }
    }
    if ($rebootNeeded) {
        Write-Warning 'Reboot Windows, then run this script again to finish the WSL2 setup.'
        return
    }

    wsl --set-default-version 2 | Out-Null
    $installed = (wsl --list --quiet 2>$null) -replace "`0", '' | Where-Object { $_ -eq $Distro }
    if ($installed) {
        Write-Host "    WSL distribution $Distro is already installed, skipping"
    } else {
        Write-Step "Installing WSL distribution $Distro"
        wsl --install --distribution $Distro --no-launch
    }
}

Assert-Administrator
Assert-Winget

foreach ($package in 'Oracle.VirtualBox', 'Hashicorp.Vagrant', 'Git.Git') {
    Install-WingetPackage $package
}

if (-not $SkipWsl) {
    Enable-Wsl2 -Distro $UbuntuDistro
}

Write-Step 'Host setup complete'
Write-Host @"
Next steps:
  1. Open a new terminal so PATH changes for Vagrant and Git take effect.
  2. Start the Ubuntu distribution and run scripts/setup-control-node.sh inside it.
  3. Run scripts/bootstrap.sh from WSL to build the cluster.
"@
