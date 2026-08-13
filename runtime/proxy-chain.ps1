param(
    [ValidateSet('start','stop','restart','status')]
    [string]$Action = 'status'
)

$ErrorActionPreference = 'Stop'
$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$Runtime = $PSScriptRoot
$PPCfg = Join-Path $Runtime 'proxypool-config.yaml'
$EPCfg = Join-Path $Runtime 'config.yaml'
$PPBin = if (Test-Path (Join-Path $Runtime 'proxypool.exe')) { Join-Path $Runtime 'proxypool.exe' } else { Join-Path $Runtime 'proxypool' }
$EPBin = if (Test-Path (Join-Path $Runtime 'easy_proxies.exe')) { Join-Path $Runtime 'easy_proxies.exe' } else { Join-Path $Runtime 'easy_proxies' }
$PPId = Join-Path $Runtime 'proxypool.pid'
$EPId = Join-Path $Runtime 'easy_proxies.pid'
$Nodes = Join-Path $Runtime 'nodes.txt'
$PPLog = Join-Path $Runtime 'proxypool.log'
$EPLog = Join-Path $Runtime 'easy_proxies.log'
$StatusPort = 18080
$WebUiPort = 9091

function Write-Log([string]$Message) {
    Write-Host "[proxy-chain] $Message"
}

function Test-PortFree([int]$Port) {
    $conn = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
    return -not [bool]$conn
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

function Read-Pid([string]$Path) {
    if (Test-Path $Path) {
        $raw = Get-Content $Path -Raw
        if ($raw) { return [int]$raw.Trim() }
    }
    return 0
}

function Test-PidAlive([int]$Pid) {
    if ($Pid -le 0) { return $false }
    try { Get-Process -Id $Pid -ErrorAction Stop | Out-Null; return $true } catch { return $false }
}

function Start-Proxypool {
    if ((Test-ProxypoolHealthy) -and (Test-HasVpnCheapNodes)) {
        Write-Log "reusing healthy proxypool on $StatusPort"
        return
    }
    if (-not (Test-Path $PPBin)) { throw "proxypool binary missing: $PPBin" }
    Write-Log 'starting proxypool'
    $proc = Start-Process -FilePath $PPBin -ArgumentList '-config', $PPCfg -RedirectStandardOutput $PPLog -RedirectStandardError $PPLog -WindowStyle Hidden -PassThru
    Set-Content -Path $PPId -Value $proc.Id
    for ($i = 0; $i -lt 30; $i++) {
        if (Test-ProxypoolHealthy) { Write-Log 'proxypool healthy'; return }
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
    $pid = Read-Pid $EPId
    if (Test-PidAlive $pid) { Write-Log 'easy_proxies already running'; return }
    if (-not (Test-Path $EPBin)) { throw "easy_proxies binary missing: $EPBin" }
    Write-Log 'starting easy_proxies'
    $proc = Start-Process -FilePath $EPBin -ArgumentList '-config', $EPCfg -RedirectStandardOutput $EPLog -RedirectStandardError $EPLog -WindowStyle Hidden -PassThru
    Set-Content -Path $EPId -Value $proc.Id
    for ($i = 0; $i -lt 20; $i++) {
        try {
            Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$WebUiPort/" -TimeoutSec 2 | Out-Null
            Write-Log "easy_proxies ready on $WebUiPort"
            return
        } catch {}
        Start-Sleep -Seconds 2
    }
    throw "easy_proxies did not become ready; tail $EPLog"
}

function Stop-EasyProxies {
    $pid = Read-Pid $EPId
    if (Test-PidAlive $pid) {
        Stop-Process -Id $pid -Force -ErrorAction SilentlyContinue
        Write-Log 'stopped easy_proxies'
    }
}

function Stop-Proxypool {
    $pid = Read-Pid $PPId
    if (Test-PidAlive $pid) {
        Stop-Process -Id $pid -Force -ErrorAction SilentlyContinue
        Write-Log 'stopped proxypool'
    }
}

switch ($Action) {
    'start' {
        Start-Proxypool
        Update-NodesFile
        Start-EasyProxies
        Write-Log "HTTP  http://127.0.0.1:2323"
        Write-Log "SOCKS socks5://127.0.0.1:2323"
    }
    'stop' {
        Stop-EasyProxies
        Stop-Proxypool
    }
    'restart' {
        & $PSCommandPath stop
        & $PSCommandPath start
    }
    'status' {
        $pp = Read-Pid $PPId
        $ep = Read-Pid $EPId
        if ((Test-PidAlive $ep) -or (Test-ProxypoolHealthy)) {
            Write-Log "running (proxypool health=$StatusPort easy_proxies_pid=$ep)"
        } else {
            Write-Log 'stopped'
        }
    }
}
