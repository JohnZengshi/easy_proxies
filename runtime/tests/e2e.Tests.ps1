$ErrorActionPreference = 'Stop'

$EasyExe = Resolve-Path (Join-Path $PSScriptRoot '..\easy_proxies.exe')
$MockScript = Resolve-Path (Join-Path $PSScriptRoot 'mock-proxy.py')
$Python = (Get-Command python).Source
$TempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('easy-proxies-e2e-' + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $TempRoot -Force | Out-Null

function Get-FreePort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    $port = ([System.Net.IPEndPoint]$listener.LocalEndpoint).Port
    $listener.Stop()
    return $port
}

function Wait-TcpPort {
    param([int]$Port)
    for ($i = 0; $i -lt 40; $i++) {
        $client = [System.Net.Sockets.TcpClient]::new()
        try {
            $client.Connect('127.0.0.1', $Port)
            return
        } catch {
            Start-Sleep -Milliseconds 250
        } finally {
            $client.Dispose()
        }
    }
    throw "port $Port did not become ready"
}

function Wait-PortDown {
    param([int]$Port)
    for ($i = 0; $i -lt 40; $i++) {
        if (-not (Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue)) {
            return
        }
        Start-Sleep -Milliseconds 250
    }
    throw "port $Port still listening"
}

$mockPort = Get-FreePort
$proxyPort = Get-FreePort
$nodePort = Get-FreePort
$mgmtPort = Get-FreePort
$mockLog = Join-Path $TempRoot 'mock-hits.txt'
$nodesFile = Join-Path $TempRoot 'nodes.txt'
$configPath = Join-Path $TempRoot 'easy_proxies-config.yaml'
$outLog = Join-Path $TempRoot 'easy_proxies.out.log'
$errLog = Join-Path $TempRoot 'easy_proxies.err.log'

[System.IO.File]::WriteAllText($nodesFile, "socks5://127.0.0.1:$mockPort#local-proxy`r`n")
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
  port: $proxyPort
multi_port:
  address: "127.0.0.1"
  base_port: $nodePort
pool:
  mode: latency
  failure_threshold: 3
  blacklist_duration: 24h
  retry_enabled: true
  retry_attempts: 3
management:
  enabled: true
  listen: "127.0.0.1:$mgmtPort"
  probe_target: "http://127.0.0.1:$mockPort/generate_204"
nodes_file: nodes.txt
"@
[System.IO.File]::WriteAllText($configPath, $config)

$mockProc = $null
$easyProc = $null
try {
    Remove-Item -LiteralPath $mockLog -Force -ErrorAction SilentlyContinue
    $mockProc = Start-Process -FilePath $Python -ArgumentList @(
        $MockScript, "$mockPort", $mockLog
    ) -WindowStyle Hidden -PassThru
    Wait-TcpPort $mockPort
    if ($mockProc.HasExited) {
        throw "mock proxy exited early; see $mockLog.err"
    }

    $easyProc = Start-Process -FilePath $EasyExe -ArgumentList '-config', $configPath -WorkingDirectory $TempRoot -RedirectStandardOutput $outLog -RedirectStandardError $errLog -WindowStyle Hidden -PassThru
    $ready = $false
    for ($i = 0; $i -lt 60; $i++) {
        if ($easyProc.HasExited) { break }
        try {
            $status = Invoke-RestMethod -Uri "http://127.0.0.1:$mgmtPort/api/nodes" -TimeoutSec 2
            if ($status.probe_sweep.available -gt 0) {
                $ready = $true
                break
            }
        } catch {}
        Start-Sleep -Milliseconds 500
    }
    if (-not $ready) { throw "easy_proxies did not become healthy; tail $outLog / $errLog" }

    $before = @(Get-Content -LiteralPath $mockLog).Count
    $oldErrorAction = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    $curlErr = Join-Path $TempRoot 'curl.err'
    $proxyBody = & curl.exe -sS -v --max-time 5 -x "http://127.0.0.1:$proxyPort" "http://127.0.0.1:$mockPort/check" 2> $curlErr
    $proxyExit = $LASTEXITCODE
    $ErrorActionPreference = $oldErrorAction
    $after = @(Get-Content -LiteralPath $mockLog).Count
    $curlErrText = Get-Content -Raw -LiteralPath $curlErr -ErrorAction SilentlyContinue
    if ($proxyExit -ne 0 -or $proxyBody -ne 'mock-ok' -or $after -le $before) {
        throw "proxy pool request failed exit=$proxyExit body=[$proxyBody] before=$before after=$after`n$curlErrText"
    }

    $nodeBody = & curl.exe -sS --max-time 5 -x "http://127.0.0.1:$nodePort" "http://127.0.0.1:$mockPort/check"
    if ($nodeBody -ne 'mock-ok') { throw "per-node proxy request failed body=[$nodeBody]" }

    $export = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$mgmtPort/api/export?target=9router" -TimeoutSec 5
    $exportLines = @($export.Content -split "`r?`n" | Where-Object { $_ })
    if ($exportLines.Count -ne 1 -or $exportLines[0] -ne "http://127.0.0.1:$nodePort") {
        throw "9router export mismatch: [$($exportLines -join ',')]"
    }
    $portMap = Join-Path $TempRoot 'node_ports.json'
    if (-not (Test-Path -LiteralPath $portMap -PathType Leaf)) { throw 'node_ports.json was not written' }

    Stop-Process -Id $easyProc.Id -Force
    $easyProc = $null
    Wait-PortDown $proxyPort

    $easyProc = Start-Process -FilePath $EasyExe -ArgumentList '-config', $configPath -WorkingDirectory $TempRoot -RedirectStandardOutput $outLog -RedirectStandardError $errLog -WindowStyle Hidden -PassThru
    $readyAfterRestart = $false
    for ($i = 0; $i -lt 60; $i++) {
        if ($easyProc.HasExited) { break }
        try {
            $status = Invoke-RestMethod -Uri "http://127.0.0.1:$mgmtPort/api/nodes" -TimeoutSec 2
            if ($status.probe_sweep.available -gt 0) {
                $readyAfterRestart = $true
                break
            }
        } catch {}
        Start-Sleep -Milliseconds 500
    }
    if (-not $readyAfterRestart) { throw "easy_proxies did not become healthy after restart; tail $outLog / $errLog" }

    $exportAfterRestart = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$mgmtPort/api/export?target=9router" -TimeoutSec 5
    $exportLinesAfterRestart = @($exportAfterRestart.Content -split "`r?`n" | Where-Object { $_ })
    if ($exportLinesAfterRestart.Count -ne 1 -or $exportLinesAfterRestart[0] -ne "http://127.0.0.1:$nodePort") {
        throw "9router export changed after restart: [$($exportLinesAfterRestart -join ',')]"
    }

    Stop-Process -Id $easyProc.Id -Force
    $easyProc = $null
    Wait-PortDown $proxyPort

    $strictBefore = @(Get-Content -LiteralPath $mockLog).Count
    $oldErrorAction = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    & curl.exe -sS --max-time 2 -x "http://127.0.0.1:$proxyPort" "http://127.0.0.1:$mockPort/check" 2>$null
    $strictExit = $LASTEXITCODE
    $ErrorActionPreference = $oldErrorAction
    $strictAfter = @(Get-Content -LiteralPath $mockLog).Count
    if ($strictExit -eq 0) { throw 'strict proxy request unexpectedly succeeded after pool stop' }
    if ($strictAfter -ne $strictBefore) { throw 'strict proxy client fell back to direct sentinel access' }

    Write-Output 'e2e hybrid, 9router export, port persistence, and strict no-fallback tests passed'
} finally {
    if ($easyProc -and -not $easyProc.HasExited) {
        Stop-Process -Id $easyProc.Id -Force -ErrorAction SilentlyContinue
    }
    if ($mockProc -and -not $mockProc.HasExited) {
        Stop-Process -Id $mockProc.Id -Force -ErrorAction SilentlyContinue
    }
    Remove-Item -LiteralPath $TempRoot -Recurse -Force -ErrorAction SilentlyContinue
}
