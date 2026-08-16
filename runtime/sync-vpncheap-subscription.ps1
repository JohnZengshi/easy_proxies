param(
    [string]$StatePath = (Join-Path $env:APPDATA 'vpncheap\app_state.json'),
    [string]$OutputPath = ''
)

$ErrorActionPreference = 'Stop'
$ScriptRoot = if ($PSScriptRoot) { $PSScriptRoot } else { Split-Path -Parent $MyInvocation.MyCommand.Path }
if (-not $OutputPath) {
    $OutputPath = Join-Path $ScriptRoot 'easy_proxies-config.yaml'
}

function Write-Failure([string]$Message) {
    Write-Error $Message
    exit 1
}

function Remove-OutputOnFailure([string]$Path) {
    if (Test-Path -LiteralPath $Path -PathType Leaf) {
        Remove-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
    }
}

function Set-OwnerOnlyAcl([string]$Path) {
    if (-not ([System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT)) {
        throw 'sync-vpncheap-subscription.ps1 is Windows-only'
    }

    $currentUser = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    & icacls.exe $Path /inheritance:r /grant:r "*${currentUser}:(F)" '*S-1-5-18:(F)' | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw 'failed to protect generated config ACL'
    }
}

function Write-ProtectedConfig([string]$Path, [string]$Content) {
    if (Test-Path -LiteralPath $Path -PathType Container) {
        throw 'output config path is not a file'
    }

    $dir = Split-Path -Parent $Path
    if (-not (Test-Path -LiteralPath $dir -PathType Container)) {
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
    }

    $temp = Join-Path $dir ('.' + [System.IO.Path]::GetRandomFileName())
    try {
        [System.IO.File]::WriteAllText($temp, $Content, [System.Text.UTF8Encoding]::new($false))
        Set-OwnerOnlyAcl $temp
        Move-Item -LiteralPath $temp -Destination $Path -Force
        Set-OwnerOnlyAcl $Path
    } finally {
        if (Test-Path -LiteralPath $temp -PathType Leaf) {
            Remove-Item -LiteralPath $temp -Force -ErrorAction SilentlyContinue
        }
    }
}

function Resolve-SubscriptionUrl([string]$StatePath) {
    if (-not (Test-Path -LiteralPath $StatePath -PathType Leaf)) {
        throw 'VPNCheap state file is not available'
    }

    try {
        $state = Get-Content -LiteralPath $StatePath -Raw -ErrorAction Stop | ConvertFrom-Json -ErrorAction Stop
    } catch {
        throw 'VPNCheap state file is not valid JSON'
    }

    if ($null -eq $state -or $state -isnot [System.Management.Automation.PSCustomObject]) {
        throw 'VPNCheap state file has an unexpected shape'
    }

    $subscription = $state.xboard_subscription
    if ($null -eq $subscription -or $subscription -isnot [System.Management.Automation.PSCustomObject]) {
        throw 'VPNCheap subscription data is missing'
    }

    $url = $subscription.subscribe_url
    if ($null -eq $url -or $url -isnot [string] -or [string]::IsNullOrWhiteSpace($url)) {
        throw 'VPNCheap subscription URL is missing or not a string'
    }

    $parsed = [System.Uri]::new($url, [System.UriKind]::Absolute)
    if ($parsed.Scheme -ne 'https') {
        throw 'VPNCheap subscription URL must use https'
    }

    return $url
}

function New-DirectConfig([string]$Url, [string]$ExistingContent = '') {
    $escaped = $Url.Replace('\', '\\').Replace('"', '\"')
    $routingBlock = ''
    if ($ExistingContent -match '(?ms)^routing:.*?(?=^\S|\z)') {
        $routingBlock = $Matches[0].TrimEnd("`r", "`n")
    }
    $config = @"
mode: hybrid
log_level: info

log:
  output: file
  file: easy_proxies.log
  max_size: 20
  max_backups: 3
  max_age: 7
  compress: true

skip_cert_verify: false

listener:
  address: "127.0.0.1"
  port: 2323

multi_port:
  address: "127.0.0.1"
  base_port: 24000

pool:
  mode: latency
  failure_threshold: 3
  blacklist_duration: 24h
  retry_enabled: true
  retry_attempts: 3

sticky:
  enabled: false
  port: 2324

management:
  listen: "127.0.0.1:9091"
  probe_target: "http://cp.cloudflare.com/generate_204"

nodes_file: nodes.txt

subscriptions:
  - "$escaped"

subscription_refresh:
  enabled: true
  interval: 1h
  timeout: 30s
  fetch_concurrency: 8
  health_check_timeout: 60s
  drain_timeout: 30s
  min_available_nodes: 1
"@
    if ($routingBlock -ne '') {
        $config += "`n$routingBlock`n"
    }
    return $config
}

try {
    if (-not ([System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT)) {
        throw 'sync-vpncheap-subscription.ps1 is Windows-only'
    }

    $url = Resolve-SubscriptionUrl $StatePath
    $existing = ''
    if (Test-Path -LiteralPath $OutputPath -PathType Leaf) {
        $existing = Get-Content -LiteralPath $OutputPath -Raw
    }
    $content = New-DirectConfig $url $existing
    Write-ProtectedConfig $OutputPath $content
    Write-Host 'VPNCheap subscription resolved and direct config generated'
} catch {
    Remove-OutputOnFailure $OutputPath
    Write-Failure $_.Exception.Message
}

exit 0
