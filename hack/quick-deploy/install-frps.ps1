param(
    [string]$Repo = $env:FRP_REPO,
    [string]$ReleaseTag = $env:FRP_RELEASE_TAG,
    [string]$RawBase = $env:FRP_RAW_BASE
)

$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($Repo)) {
    $Repo = 'lovitus/frp'
}
if ([string]::IsNullOrWhiteSpace($RawBase)) {
    $RawBase = "https://raw.githubusercontent.com/$Repo/codex/mix-transport-release/hack/quick-deploy"
}

$BIND_PORT = 7000
$MIX_BIND_PORT = 7001
$CONNECT_PASSWORD = ''
$DASHBOARD_ADDR = '0.0.0.0'
$DASHBOARD_PORT = $null
$DASHBOARD_USER = 'admin'
$DASHBOARD_PASSWORD = ''

function Trim-Value([string]$Value) {
    if ($null -eq $Value) { return '' }
    return $Value.Trim()
}

function Prompt-Line {
    param(
        [string]$Label,
        [string]$DefaultValue = ''
    )
    while ($true) {
        $prompt = if ($DefaultValue) { "$Label [$DefaultValue]" } else { $Label }
        $value = Trim-Value (Read-Host $prompt)
        if (-not [string]::IsNullOrWhiteSpace($value)) { return $value }
        if ($DefaultValue) { return $DefaultValue }
        Write-Host 'This value is required.' -ForegroundColor Yellow
    }
}

function Prompt-Port {
    param(
        [string]$Label,
        [int]$DefaultValue
    )
    while ($true) {
        $value = Prompt-Line -Label $Label -DefaultValue ([string]$DefaultValue)
        $port = 0
        if ([int]::TryParse($value, [ref]$port) -and $port -ge 1 -and $port -le 65535) {
            return $port
        }
        Write-Host "Invalid port '$value'. Expected 1..65535." -ForegroundColor Yellow
    }
}

function Prompt-Password {
    param(
        [string]$Label,
        [string]$DefaultValue = ''
    )
    while ($true) {
        $prompt = if ($DefaultValue) { "$Label [press Enter to use default]" } else { $Label }
        $value = Trim-Value (Read-Host $prompt)
        if (-not [string]::IsNullOrWhiteSpace($value)) { return $value }
        if ($DefaultValue) { return $DefaultValue }
        Write-Host 'Password cannot be empty.' -ForegroundColor Yellow
    }
}

function Escape-Toml([string]$Value) {
    return $Value.Replace('\', '\\').Replace('"', '\"')
}

function Write-Utf8File {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Content
    )
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText((Resolve-Path -LiteralPath .).Path + [System.IO.Path]::DirectorySeparatorChar + $Path.TrimStart('.','\','/'), $Content, $utf8NoBom)
}

function Resolve-Release {
    if ($ReleaseTag) {
        $json = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/tags/$ReleaseTag"
    } else {
        $json = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    }
    if (-not $json.tag_name) {
        throw 'Unable to resolve release tag from GitHub API.'
    }
    return $json
}

function Get-WindowsSuffix {
    $arch = ''
    if ($env:PROCESSOR_ARCHITEW6432) {
        $arch = $env:PROCESSOR_ARCHITEW6432.ToLowerInvariant()
    } elseif ($env:PROCESSOR_ARCHITECTURE) {
        $arch = $env:PROCESSOR_ARCHITECTURE.ToLowerInvariant()
    }
    switch ($arch) {
        'amd64' { return 'windows_amd64' }
        'x64' { return 'windows_amd64' }
        'arm64' { return 'windows_arm64' }
        default { throw "Unsupported Windows architecture: $arch" }
    }
}

function Find-AssetUrl {
    param(
        [Parameter(Mandatory = $true)]$ReleaseJson,
        [Parameter(Mandatory = $true)][string]$Suffix
    )
    $version = $ReleaseJson.tag_name.TrimStart('v')
    $exact = "frp_${version}_${Suffix}.zip"
    $asset = $ReleaseJson.assets | Where-Object { $_.name -eq $exact } | Select-Object -First 1
    if (-not $asset) {
        $asset = $ReleaseJson.assets | Where-Object { $_.name -match "^frp_.+_${Suffix}\.zip$" } | Select-Object -First 1
        if ($asset) {
            Write-Warning "No exact asset for $($ReleaseJson.tag_name); using $($asset.name)"
        }
    }
    if (-not $asset) {
        throw "No matching asset found for suffix $Suffix in release $($ReleaseJson.tag_name)."
    }
    return $asset.browser_download_url
}

function Build-DefaultMixToken([string]$Password) {
    $escaped = Escape-Toml $Password
    return "ss://chacha20-ietf-poly1305:$escaped,kcp://$escaped,ssh://frp:$escaped"
}

function Download-And-Extract {
    param(
        [Parameter(Mandatory = $true)][string]$Url,
        [Parameter(Mandatory = $true)][string]$ReleaseVersion
    )
    $tmpZip = Join-Path ([System.IO.Path]::GetTempPath()) ("frp-$ReleaseVersion-windows.zip")
    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("frp-extract-$([guid]::NewGuid().ToString('N'))")
    Invoke-WebRequest -UseBasicParsing -Uri $Url -OutFile $tmpZip
    New-Item -ItemType Directory -Path $tmpDir | Out-Null
    Expand-Archive -Path $tmpZip -DestinationPath $tmpDir -Force
    $packageDir = Get-ChildItem -Path $tmpDir -Directory | Select-Object -First 1
    if (-not $packageDir) {
        throw 'Extracted package directory not found.'
    }
    Get-ChildItem -Path $packageDir.FullName | ForEach-Object {
        Copy-Item -Path $_.FullName -Destination (Join-Path (Get-Location) $_.Name) -Recurse -Force
    }
    Remove-Item -Path $tmpZip -Force -ErrorAction SilentlyContinue
    Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
}

function Generate-Config {
    $mixToken = Escape-Toml (Build-DefaultMixToken $CONNECT_PASSWORD)
    $dashPwd = Escape-Toml $DASHBOARD_PASSWORD
    $dashAddr = Escape-Toml $DASHBOARD_ADDR
    $dashUser = Escape-Toml $DASHBOARD_USER
    $content = @"
bindPort = $BIND_PORT
mixBindPort = $MIX_BIND_PORT
mixToken = "$mixToken"

webServer.addr = "$dashAddr"
webServer.port = $DASHBOARD_PORT
webServer.user = "$dashUser"
webServer.password = "$dashPwd"
"@
    Write-Utf8File -Path 'frps.toml' -Content $content
}

function Verify-And-SmokeRun {
    $verify = & .\frps.exe verify -c .\frps.toml 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "Config verification failed:`n$verify"
    }

    $outLog = Join-Path ([System.IO.Path]::GetTempPath()) "frps-smoke-out.log"
    $errLog = Join-Path ([System.IO.Path]::GetTempPath()) "frps-smoke-err.log"
    $proc = Start-Process -FilePath .\frps.exe -ArgumentList '-c', '.\frps.toml' -PassThru -WindowStyle Hidden -RedirectStandardOutput $outLog -RedirectStandardError $errLog
    Start-Sleep -Seconds 3
    if (-not $proc.HasExited) {
        Stop-Process -Id $proc.Id -Force
        Write-Host 'Smoke start check passed.'
        return
    }
    $recent = ''
    if (Test-Path $errLog) { $recent += (Get-Content $errLog -Raw) }
    if (Test-Path $outLog) { $recent += (Get-Content $outLog -Raw) }
    throw "Smoke start check failed:`n$recent"
}

$release = Resolve-Release
$releaseVersion = $release.tag_name.TrimStart('v')
$suffix = Get-WindowsSuffix
$assetUrl = Find-AssetUrl -ReleaseJson $release -Suffix $suffix

Write-Host "Repo: $Repo"
Write-Host "Detected platform: $suffix"
Write-Host "Using release: $($release.tag_name)"
Write-Host "Downloading asset: $assetUrl"

Download-And-Extract -Url $assetUrl -ReleaseVersion $releaseVersion

Write-Host 'Configure frps (press Enter to accept defaults)'
$BIND_PORT = Prompt-Port -Label 'bindPort' -DefaultValue $BIND_PORT
$MIX_BIND_PORT = Prompt-Port -Label 'mixBindPort' -DefaultValue $MIX_BIND_PORT
$CONNECT_PASSWORD = Prompt-Password -Label 'Connection password (used for ss/kcp/ssh)'
$DASHBOARD_ADDR = Prompt-Line -Label 'dashboard/webServer addr' -DefaultValue $DASHBOARD_ADDR
$defaultDashboardPort = $MIX_BIND_PORT + 1
if ($defaultDashboardPort -gt 65535) {
    $defaultDashboardPort = 65535
}
$DASHBOARD_PORT = Prompt-Port -Label 'dashboard/webServer port' -DefaultValue $defaultDashboardPort
$DASHBOARD_PASSWORD = Prompt-Password -Label 'dashboard password' -DefaultValue $CONNECT_PASSWORD

Generate-Config
Verify-And-SmokeRun

$mixToken = Build-DefaultMixToken $CONNECT_PASSWORD
$mixTokenB64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($mixToken))

Write-Host ''
Write-Host 'Generated .\frps.toml'
Write-Host 'Start command:'
Write-Host '.\frps.exe -c .\frps.toml'
Write-Host ''
Write-Host 'Windows frpc quick-deploy command:'
Write-Host "`$env:FRP_REPO='$Repo'; `$env:FRP_RELEASE_TAG='$($release.tag_name)'; `$env:FRP_SERVER_ADDR='YOUR_SERVER_ADDR'; `$env:FRP_MIX_BIND_PORT='$MIX_BIND_PORT'; `$env:FRP_MIX_TOKEN_B64='$mixTokenB64'; powershell -ExecutionPolicy Bypass -Command `"iwr -UseBasicParsing $RawBase/install-frpc.ps1 | iex`""
