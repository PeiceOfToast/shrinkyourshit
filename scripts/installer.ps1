# SPDX-License-Identifier: AGPL-3.0-only
#
# ShrinkYourShit worker installer for Windows (task 09). Downloads the
# matching release binary, installs it under Program Files, writes the
# data-dir config (role=worker, server, token) and registers a Windows
# service via sc.exe.
#
# The server serves this file at /install.ps1 with __SHRINK_SERVER_URL__ and
# __SHRINK_VERSION__ replaced. Because piped scripts cannot take
# parameters, the server URL and token are also read from the
# SHRINK_SERVER and SHRINK_TOKEN environment variables:
#
#   $env:SHRINK_SERVER = 'http://192.168.1.10:8765'
#   $env:SHRINK_TOKEN  = 'TOKEN'
#   irm http://192.168.1.10:8765/install.ps1 | iex
#
# Running the file directly allows all parameters:
#   .\installer.ps1 -Server http://server:8765 -Token TOKEN -NoService

param(
    [string]$Server = '',
    [string]$Token = '',
    [string]$NodeName = $env:COMPUTERNAME,
    [string]$DataDir = (Join-Path $env:USERPROFILE '.shrinkyourshit'),
    [string]$Version = '',
    [string]$ReleaseUrl = 'https://github.com/PeiceOfToast/shrinkyourshit/releases/download',
    [string]$InstallDir = (Join-Path $env:ProgramFiles 'ShrinkYourShit'),
    [string]$ServiceDir = '',
    [switch]$NoService
)

$ErrorActionPreference = 'Stop'

# The served copy has these two values baked in; the raw source keeps the
# placeholder text, which is deliberately not a URL / release tag.
$bakedServer = '__SHRINK_SERVER_URL__'
$bakedVersion = '__SHRINK_VERSION__'

if ($Server -eq '') { $Server = $env:SHRINK_SERVER }
if ($Server -eq '') { $Server = $bakedServer }
$Server = $Server.TrimEnd('/')
if ($Token -eq '') { $Token = $env:SHRINK_TOKEN }
if ($Version -eq '') { $Version = $bakedVersion }
if ($Server -notlike 'http://*' -and $Server -notlike 'https://*') {
    throw 'installer: no server known - pass -Server URL or set $env:SHRINK_SERVER (e.g. http://192.168.1.10:8765)'
}
if ($Version -notlike 'v[0-9]*') {
    throw 'installer: no release version known - serve this script from the server (GET /install.ps1), or pass -Version vX.Y.Z'
}

# --- platform ---------------------------------------------------------------

$goArch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "installer: unsupported architecture $env:PROCESSOR_ARCHITECTURE (want amd64 or arm64)" }
}

# --- download and install ----------------------------------------------------

$assetUrl = "$ReleaseUrl/$Version/shrinkyourshit-windows-$goArch.zip"
$tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("shrinkyourshit-install-" + [System.Guid]::NewGuid().ToString('N'))
Write-Output "Downloading ShrinkYourShit $Version for windows/$goArch..."
try {
    New-Item -ItemType Directory -Path $tmpDir | Out-Null
    $zipPath = Join-Path $tmpDir 'binary.zip'
    Invoke-WebRequest -Uri $assetUrl -OutFile $zipPath -UseBasicParsing
    Expand-Archive -Path $zipPath -DestinationPath (Join-Path $tmpDir 'unpack') -Force
    $exeName = "shrinkyourshit-windows-$goArch.exe"
    $exeSrc = Join-Path (Join-Path $tmpDir 'unpack') $exeName
    if (-not (Test-Path $exeSrc)) {
        throw "installer: the release archive did not contain $exeName"
    }
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $exePath = Join-Path $InstallDir 'shrinkyourshit.exe'
    Move-Item -Path $exeSrc -Destination $exePath -Force
    Write-Output "Installed $exePath"
}
finally {
    if (Test-Path $tmpDir) { Remove-Item -Path $tmpDir -Recurse -Force }
}

# --- configuration ------------------------------------------------------------

New-Item -ItemType Directory -Path $DataDir -Force | Out-Null
$configPath = Join-Path $DataDir 'config.yaml'
$lines = @(
    'role: worker',
    "server: $Server",
    "node_name: $NodeName"
)
if ($Token -ne '') { $lines += "token: $Token" }
Set-Content -Path $configPath -Value $lines -Encoding Ascii
Write-Output "Wrote $configPath (role: worker, server: $Server)"

# --- service -------------------------------------------------------------------

if ($ServiceDir -ne '') {
    # Check mode: describe the service without touching the system.
    New-Item -ItemType Directory -Path $ServiceDir -Force | Out-Null
    $desc = Join-Path $ServiceDir 'shrinkyourshit-service.txt'
    Set-Content -Path $desc -Value @(
        "ServiceName: shrinkyourshit",
        "BinPath: `"$exePath`" serve",
        "StartType: auto"
    ) -Encoding Ascii
    Write-Output "Wrote $desc (service not installed)"
    exit 0
}

if ($NoService) {
    Write-Output 'Skipping service registration (-NoService).'
}
else {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    $isAdmin = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    if (-not $isAdmin) {
        Write-Warning 'Not administrator: register the service from an elevated prompt, or start the worker manually:'
        Write-Output "    `"$exePath`" serve"
        exit 0
    }
    $binPath = "`"$exePath`" serve"
    $existing = sc.exe query shrinkyourshit 2>$null
    if ($LASTEXITCODE -eq 0) {
        sc.exe stop shrinkyourshit | Out-Null
        sc.exe delete shrinkyourshit | Out-Null
    }
    sc.exe create shrinkyourshit binPath= $binPath start= auto DisplayName= 'ShrinkYourShit worker' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "installer: sc.exe create failed with exit code $LASTEXITCODE" }
    sc.exe failure shrinkyourshit reset= 86400 actions= restart/5000/restart/5000/restart/5000 | Out-Null
    sc.exe start shrinkyourshit | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "installer: sc.exe start failed with exit code $LASTEXITCODE" }
    Write-Output 'Service shrinkyourshit is registered and started.'
}

# --- what happens next -----------------------------------------------------------

Write-Output ''
Write-Output "Node name: $NodeName"
if ($Token -ne '') {
    Write-Output 'The node pairs with the server using its token and starts working.'
}
else {
    Write-Output "The node is asking to join $Server; approve it in the server UI"
    Write-Output '(Nodes page). It is waiting for approval.'
}
