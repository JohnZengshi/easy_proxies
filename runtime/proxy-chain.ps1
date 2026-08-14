[CmdletBinding()]
param(
    [ValidateSet('start', 'stop', 'restart', 'status')]
    [string]$Action = 'status',

    [switch]$Legacy,

    [string]$ResolverScript = '',
    [string]$ResolverStatePath = '',
    [string]$EasyProxiesBin = '',
    [string]$EasyProxiesArgumentPrefix = '',
    [int]$WebUiPort = 9091,
    [switch]$SkipLogRedirect
)

$ErrorActionPreference = 'Stop'
$Runtime = if ($PSScriptRoot) { $PSScriptRoot } else { Split-Path -Parent $MyInvocation.MyCommand.Path }
if (-not $ResolverScript) {
    $ResolverScript = Join-Path $Runtime 'sync-vpncheap-subscription.ps1'
}
$PPCfg = Join-Path $Runtime 'proxypool-config.yaml'
$EPCfg = Join-Path $Runtime 'config.yaml'
$EPEasyCfg = Join-Path $Runtime 'easy_proxies-config.yaml'
$PPBin = if (Test-Path (Join-Path $Runtime 'proxypool.exe')) { Join-Path $Runtime 'proxypool.exe' } else { Join-Path $Runtime 'proxypool' }
$EPBin = if (-not [string]::IsNullOrWhiteSpace($EasyProxiesBin)) {
    $EasyProxiesBin
} elseif (Test-Path (Join-Path $Runtime 'easy_proxies.exe')) {
    Join-Path $Runtime 'easy_proxies.exe'
} else {
    Join-Path $Runtime 'easy_proxies'
}
$PPId = Join-Path $Runtime 'proxypool.pid'
$EPId = Join-Path $Runtime 'easy_proxies.pid'
$Nodes = Join-Path $Runtime 'nodes.txt'
$PPLog = Join-Path $Runtime 'proxypool.log'
$PPErrLog = Join-Path $Runtime 'proxypool.err.log'
$EPLog = Join-Path $Runtime 'easy_proxies.log'
$EPErrLog = Join-Path $Runtime 'easy_proxies.err.log'
$StatusPort = 18080

function Write-Log([string]$Message) {
    Write-Host "[proxy-chain] $Message"
}

function Test-ProxypoolHealthy {
    try {
        $code = (Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$StatusPort/healthz" -TimeoutSec 3).StatusCode
        return ($code -eq 200)
    } catch {
        return $false
    }
}

function Test-HasVpnCheapNodes {
    try {
        $status = Invoke-RestMethod -Uri "http://127.0.0.1:$StatusPort/status" -TimeoutSec 5
        return [bool]($status | Where-Object { $_.tag -eq 'vpncheap' -and $_.healthy })
    } catch {
        return $false
    }
}

function Test-EasyProxiesHealthy {
    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$WebUiPort/" -TimeoutSec 2
        return ($response.StatusCode -eq 200)
    } catch {
        return $false
    }
}

function Read-Pid([string]$Path) {
    if (Test-Path $Path) {
        $raw = Get-Content $Path -Raw
        if ($raw) { return [int]$raw.Trim() }
    }
    return 0
}

function Test-PidAlive([int]$ProcessId) {
    if ($ProcessId -le 0) { return $false }
    try {
        Get-Process -Id $ProcessId -ErrorAction Stop | Out-Null
        return $true
    } catch {
        return $false
    }
}

function Start-Proxypool {
    if ((Test-ProxypoolHealthy) -and (Test-HasVpnCheapNodes)) {
        Write-Log "reusing healthy proxypool on $StatusPort"
        return
    }
    if (-not (Test-Path $PPBin)) { throw "proxypool binary missing: $PPBin" }
    Write-Log 'starting proxypool'
    $proc = Start-Process -FilePath $PPBin -ArgumentList '-config', $PPCfg -RedirectStandardOutput $PPLog -RedirectStandardError $PPErrLog -WindowStyle Hidden -PassThru
    Set-Content -Path $PPId -Value $proc.Id
    for ($i = 0; $i -lt 30; $i++) {
        if (Test-ProxypoolHealthy) {
            Write-Log 'proxypool healthy'
            return
        }
        Start-Sleep -Seconds 2
    }
    throw "proxypool did not become healthy; tail $PPLog"
}

function Update-NodesFile {
    Write-Log "generating $Nodes"
    $status = Invoke-RestMethod -Uri "http://127.0.0.1:$StatusPort/status" -TimeoutSec 10
    $lines = @($status | Where-Object { $_.healthy } | Sort-Object port | ForEach-Object {
        "http://127.0.0.1:$($_.port)#vpncheap-$($_.port)"
    })
    if ($lines.Count -eq 0) { throw "no healthy VPNCheap nodes in /status" }
    Set-Content -Path $Nodes -Value $lines
    Write-Log "$($lines.Count) healthy nodes"
}

function Start-EasyProxies {
    param([string]$ConfigPath)

    $procId = Read-Pid $EPId
    if (Test-PidAlive $procId) {
        Write-Log 'easy_proxies already running'
        return
    }
    if (-not (Test-Path $EPBin)) { throw "easy_proxies binary missing: $EPBin" }
    if (-not (Test-Path $ConfigPath)) { throw "easy_proxies config missing: $ConfigPath" }

    Write-Log 'starting easy_proxies'
    $argumentList = @()
    if ($EasyProxiesArgumentPrefix) {
        $argumentList += @($EasyProxiesArgumentPrefix -split '\s+')
    }
    $argumentList += @('-config', $ConfigPath)
    $startParams = @{
        FilePath = $EPBin
        ArgumentList = $argumentList
        WindowStyle = 'Hidden'
        PassThru = $true
    }
    if (-not $SkipLogRedirect) {
        $startParams.RedirectStandardOutput = $EPLog
        $startParams.RedirectStandardError = $EPErrLog
    }
    $proc = Start-Process @startParams
    Set-Content -Path $EPId -Value $proc.Id
    if ($WebUiPort -gt 0) {
        for ($i = 0; $i -lt 20; $i++) {
            if (Test-EasyProxiesHealthy) {
                Write-Log "easy_proxies ready on $WebUiPort"
                return
            }
            Start-Sleep -Seconds 2
        }
        throw "easy_proxies did not become ready; tail $EPLog"
    } else {
        Write-Log 'started easy_proxies without readiness check'
    }
}

function Stop-EasyProxies {
    $procId = Read-Pid $EPId
    if (Test-PidAlive $procId) {
        Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue
        Write-Log 'stopped easy_proxies'
    }
}

function Stop-Proxypool {
    $procId = Read-Pid $PPId
    if (Test-PidAlive $procId) {
        Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue
        Write-Log 'stopped proxypool'
    }
}

function Start-Direct {
    Stop-Proxypool
    Stop-EasyProxies

    if (-not (Test-Path $ResolverScript)) { throw "subscription resolver missing: $ResolverScript" }
    try {
        if ($ResolverStatePath) {
            & $ResolverScript -StatePath $ResolverStatePath -OutputPath $EPEasyCfg
        } else {
            & $ResolverScript -OutputPath $EPEasyCfg
        }
        if ($LASTEXITCODE -ne 0) { throw 'failed to regenerate VPNCheap direct config' }
    } catch {
        Remove-Item -LiteralPath $EPEasyCfg -Force -ErrorAction SilentlyContinue
        throw
    }
    if (-not (Test-Path $EPEasyCfg)) { throw 'subscription resolver did not create direct config' }

    Start-EasyProxies -ConfigPath $EPEasyCfg
    Write-Log 'direct mode: proxypool stopped, port 18080 not used'
    if ($WebUiPort -gt 0) {
        Write-Log "9router export http://127.0.0.1:$WebUiPort/api/export?target=9router"
    }
}

function Start-Legacy {
    Start-Proxypool
    Update-NodesFile
    Start-EasyProxies -ConfigPath $EPCfg
}

switch ($Action) {
    'start' {
        if ($Legacy) {
            Start-Legacy
        } else {
            Start-Direct
        }
        Write-Log "HTTP  http://127.0.0.1:2323"
        Write-Log "SOCKS socks5://127.0.0.1:2323"
    }
    'stop' {
        Stop-EasyProxies
        Stop-Proxypool
    }
    'restart' {
        & $PSCommandPath stop
        if ($Legacy) {
            & $PSCommandPath start -Legacy
        } else {
            & $PSCommandPath start
        }
    }
    'status' {
        $ep = Read-Pid $EPId
        if ($Legacy) {
            $pp = Read-Pid $PPId
            if ((Test-PidAlive $ep) -or (Test-PidAlive $pp) -or (Test-ProxypoolHealthy)) {
                Write-Log "running (legacy proxypool health=$StatusPort easy_proxies_pid=$ep)"
            } else {
                Write-Log 'stopped'
            }
        } else {
            if ((Test-PidAlive $ep) -or (Test-EasyProxiesHealthy)) {
                Write-Log "running (easy_proxies_pid=$ep)"
            } else {
                Write-Log 'stopped'
            }
        }
    }
}
