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

function Download-Script {
    param(
        [Parameter(Mandatory = $true)][string]$Name
    )
    $tmp = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), "frp-$Name")
    Invoke-WebRequest -UseBasicParsing -Uri "$RawBase/$Name" -OutFile $tmp
    return $tmp
}

$target = Read-Host 'Deploy frps(server) or frpc(client)? [frps/frpc]'
if ($null -eq $target) { $target = '' }
$target = $target.Trim().ToLowerInvariant()

switch ($target) {
    'frps' { $scriptName = 'install-frps.ps1' }
    'frpc' { $scriptName = 'install-frpc.ps1' }
    default { throw "Unsupported target '$target'. Expected frps or frpc." }
}

$scriptPath = Download-Script -Name $scriptName
$args = @('-ExecutionPolicy', 'Bypass', '-File', $scriptPath, '-Repo', $Repo)
if (-not [string]::IsNullOrWhiteSpace($ReleaseTag)) {
    $args += @('-ReleaseTag', $ReleaseTag)
}
if (-not [string]::IsNullOrWhiteSpace($RawBase)) {
    $args += @('-RawBase', $RawBase)
}

Write-Host "Using repo: $Repo"
Write-Host "Fetching: $RawBase/$scriptName"
Write-Host "Starting $scriptName..."

& powershell @args
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}
