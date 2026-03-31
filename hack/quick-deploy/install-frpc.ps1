param(
    [string]$Repo = $env:FRP_REPO,
    [string]$ReleaseTag = $env:FRP_RELEASE_TAG,
    [string]$RawBase = $env:FRP_RAW_BASE,
    [string]$ServerAddr = $env:FRP_SERVER_ADDR,
    [string]$MixBindPort = $env:FRP_MIX_BIND_PORT,
    [string]$MixToken = $env:FRP_MIX_TOKEN,
    [string]$MixTokenB64 = $env:FRP_MIX_TOKEN_B64,
    [string]$ClientID = $env:FRP_CLIENT_ID
)

$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($Repo)) {
    $Repo = 'lovitus/frp'
}
if ([string]::IsNullOrWhiteSpace($RawBase)) {
    $RawBase = "https://raw.githubusercontent.com/$Repo/codex/mix-transport-release/hack/quick-deploy"
}
if ([string]::IsNullOrWhiteSpace($MixBindPort)) {
    $MixBindPort = '7001'
}

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
        [string]$DefaultValue
    )
    while ($true) {
        $value = Prompt-Line -Label $Label -DefaultValue $DefaultValue
        $port = 0
        if ([int]::TryParse($value, [ref]$port) -and $port -ge 1 -and $port -le 65535) {
            return [string]$port
        }
        Write-Host "Invalid port '$value'. Expected 1..65535." -ForegroundColor Yellow
    }
}

function Prompt-Password([string]$Label) {
    while ($true) {
        $value = Trim-Value (Read-Host $Label)
        if (-not [string]::IsNullOrWhiteSpace($value)) { return $value }
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

function Decode-Base64([string]$Value) {
    if ([string]::IsNullOrWhiteSpace($Value)) { return '' }
    return [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($Value))
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
    $serverAddrEsc = Escape-Toml $ServerAddr
    $mixTokenEsc = Escape-Toml $MixToken
    $clientIDEsc = Escape-Toml $ClientID
    $content = @"
serverAddr = "$serverAddrEsc"
mixBindPort = $MixBindPort
mixToken = "$mixTokenEsc"
# mixFallbackHosts = "backup-a.example.com,backup-b.example.com:7002"

clientID = "$clientIDEsc"
allowGatewayTunnels = true
mixAllowGateway = true
loginFailExit = false
"@
    Write-Utf8File -Path 'frpc.toml' -Content $content
}

function Verify-And-SmokeRun {
    $verify = & .\frpc.exe verify -c .\frpc.toml 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "Config verification failed:`n$verify"
    }

    $outLog = Join-Path ([System.IO.Path]::GetTempPath()) "frpc-smoke-out.log"
    $errLog = Join-Path ([System.IO.Path]::GetTempPath()) "frpc-smoke-err.log"
    $proc = Start-Process -FilePath .\frpc.exe -ArgumentList '-c', '.\frpc.toml' -PassThru -WindowStyle Hidden -RedirectStandardOutput $outLog -RedirectStandardError $errLog
    Start-Sleep -Seconds 3
    if (-not $proc.HasExited) {
        Stop-Process -Id $proc.Id -Force
        Write-Host 'Smoke start check passed.'
        return
    }
    $recent = ''
    if (Test-Path $errLog) { $recent += (Get-Content $errLog -Raw) }
    if (Test-Path $outLog) { $recent += (Get-Content $outLog -Raw) }
    Write-Warning "Smoke start check warning: frpc exited quickly.`n$recent"
}

if ($MixTokenB64 -and -not $MixToken) {
    $MixToken = Decode-Base64 $MixTokenB64
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

$presetMode = -not [string]::IsNullOrWhiteSpace($MixToken)
if ($presetMode) {
    Write-Host 'Preset mode detected.'
    $ServerAddr = Prompt-Line -Label 'serverAddr' -DefaultValue $ServerAddr
    $ClientID = Prompt-Line -Label 'clientID' -DefaultValue $ClientID
    $MixBindPort = Prompt-Port -Label 'mixBindPort' -DefaultValue $MixBindPort
} else {
    Write-Host 'Standalone mode.'
    $MixBindPort = Prompt-Port -Label 'mixBindPort' -DefaultValue $MixBindPort
    $password = Prompt-Password 'Connection password (used for ss/kcp/ssh)'
    $MixToken = Build-DefaultMixToken $password
    $ServerAddr = Prompt-Line -Label 'serverAddr' -DefaultValue $ServerAddr
    $ClientID = Prompt-Line -Label 'clientID' -DefaultValue $ClientID
}

Generate-Config
Verify-And-SmokeRun

Write-Host ''
Write-Host 'Generated .\frpc.toml'
Write-Host 'Start command:'
Write-Host '.\frpc.exe -c .\frpc.toml'
