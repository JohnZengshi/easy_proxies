$ErrorActionPreference = 'Stop'

$TempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('proxy-chain-test-' + [System.Guid]::NewGuid().ToString('N'))
$Runtime = Join-Path $TempRoot 'runtime'
New-Item -ItemType Directory -Path $Runtime -Force | Out-Null

function Get-FreePort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    $port = ([System.Net.IPEndPoint]$listener.LocalEndpoint).Port
    $listener.Stop()
    return $port
}

$ScriptCopy = Join-Path $Runtime 'proxy-chain.ps1'
Copy-Item -LiteralPath (Resolve-Path (Join-Path $PSScriptRoot '..\proxy-chain.ps1')) -Destination $ScriptCopy

$pwsh = (Get-Command powershell).Source
$webPort = Get-FreePort
$fakeEasyScript = Join-Path $Runtime 'fake-easy-proxies.ps1'
$fakeEasyLines = @(
    'param([string]$config, [Parameter(ValueFromRemainingArguments=$true)][string[]]$Remaining)',
    '$logPath = Join-Path $PSScriptRoot "fake-easy-proxies-args.txt"',
    '[System.IO.File]::WriteAllLines($logPath, @("-config", $config))',
    'if ($env:FAKE_EASY_ARGS) { [System.IO.File]::WriteAllLines($env:FAKE_EASY_ARGS, @("-config", $config)) }',
    '$server = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, [int]$env:FAKE_EASY_PORT)',
    '$server.Start()',
    'while ($true) {',
    '  $client = $server.AcceptTcpClient()',
    '  try {',
    '    $stream = $client.GetStream()',
    '    $buf = New-Object byte[] 1024',
    '    $null = $stream.Read($buf, 0, $buf.Length)',
    '    $resp = "HTTP/1.1 200 OK`r`nContent-Length: 2`r`nConnection: close`r`n`r`nok"',
    '    $bytes = [System.Text.Encoding]::ASCII.GetBytes($resp)',
    '    $stream.Write($bytes, 0, $bytes.Length)',
    '  } finally { $client.Close() }',
    '}'
)
Set-Content -LiteralPath $fakeEasyScript -Value $fakeEasyLines -Encoding utf8

$fakeUrl = 'https://example.test/subscribe?token=fixture-secret'
$statePath = Join-Path $TempRoot 'app_state.json'
Set-Content -LiteralPath $statePath -Value (@{
    xboard_subscription = @{ subscribe_url = $fakeUrl }
} | ConvertTo-Json -Depth 5) -Encoding utf8

$fakeResolver = Join-Path $TempRoot 'fake-resolver.ps1'
$fakeResolverLines = @(
    'param([string]$StatePath, [string]$OutputPath)',
    'if (-not (Test-Path -LiteralPath $StatePath -PathType Leaf)) { exit 1 }',
    '$content = @"',
    'mode: hybrid',
    'listener:',
    '  address: "127.0.0.1"',
    '  port: 2323',
    'multi_port:',
    '  address: "127.0.0.1"',
    '  base_port: 24000',
    'skip_cert_verify: false',
    'nodes_file: nodes.txt',
    'subscriptions:',
    ('  - "' + $fakeUrl + '"'),
    '"@',
    'Set-Content -LiteralPath $OutputPath -Value $content -Encoding utf8',
    'exit 0'
)
Set-Content -LiteralPath $fakeResolver -Value $fakeResolverLines -Encoding utf8

$failResolver = Join-Path $TempRoot 'fail-resolver.ps1'
Set-Content -LiteralPath $failResolver -Value @(
    'param([string]$StatePath, [string]$OutputPath)',
    'exit 1'
) -Encoding utf8

$easyArgsPath = Join-Path $Runtime 'fake-easy-proxies-args.txt'

function Invoke-ProxyChain([string[]]$Arguments, [string]$Name) {
    $out = Join-Path $TempRoot ($Name + '.out.log')
    $err = Join-Path $TempRoot ($Name + '.err.log')
    $allArgs = @('-NoProfile', '-File', $ScriptCopy)
    $allArgs += $Arguments
    $oldErrorAction = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    & $pwsh @allArgs 1> $out 2> $err
    $exit = $LASTEXITCODE
    $ErrorActionPreference = $oldErrorAction
    return [pscustomobject]@{
        ExitCode = if ($null -eq $exit) { 0 } else { $exit }
        Out = Get-Content -Raw -LiteralPath $out
        Err = Get-Content -Raw -LiteralPath $err
    }
}

function Wait-ForProcessGone([int]$ProcessId) {
    for ($i = 0; $i -lt 20; $i++) {
        if (-not (Get-Process -Id $ProcessId -ErrorAction SilentlyContinue)) {
            return
        }
        Start-Sleep -Milliseconds 250
    }
    throw "process $ProcessId still running after stop"
}

function Wait-ForFile([string]$Path) {
    for ($i = 0; $i -lt 50; $i++) {
        if (Test-Path -LiteralPath $Path -PathType Leaf) {
            return
        }
        Start-Sleep -Milliseconds 100
    }
    throw "file was not created: $Path"
}

$oldEasyArgs = $env:FAKE_EASY_ARGS
$oldEasyPort = $env:FAKE_EASY_PORT
$env:FAKE_EASY_ARGS = $easyArgsPath
$env:FAKE_EASY_PORT = [string]$webPort

try {
    $commonArgs = @(
        '-EasyProxiesBin', $pwsh,
        '-EasyProxiesArgumentPrefix', ('-NoProfile -ExecutionPolicy Bypass -File ' + $fakeEasyScript),
        '-WebUiPort', "$webPort",
        '-SkipLogRedirect',
        '-ResolverScript', $fakeResolver,
        '-ResolverStatePath', $statePath
    )

    $start = Invoke-ProxyChain (@('-Action', 'start') + $commonArgs) 'start'
    if ($start.ExitCode -ne 0) {
        Write-Host "START OUT:`n$($start.Out)"
        Write-Host "START ERR:`n$($start.Err)"
        throw "start failed exit=$($start.ExitCode): $($start.Out) $($start.Err)"
    }
    if ($start.Out -notmatch 'direct mode: proxypool stopped') { throw 'start did not report direct mode' }
    if ($start.Out -notmatch '9router export http://127.0.0.1:[0-9]+/api/export\?target=9router') { throw 'start did not report 9router export URL' }
    Wait-ForFile $easyArgsPath
    if ($start.Out -match 'fixture-secret') { throw 'start output leaked subscription secret' }

    $argsText = Get-Content -Raw -LiteralPath $easyArgsPath
    if ($argsText -notmatch 'easy_proxies-config.yaml') { throw "easy_proxies did not receive direct config: $argsText" }

    $generatedConfig = Join-Path $Runtime 'easy_proxies-config.yaml'
    if (-not (Test-Path -LiteralPath $generatedConfig -PathType Leaf)) { throw 'direct config was not generated' }
    $configText = Get-Content -Raw -LiteralPath $generatedConfig
    if ($configText -notmatch 'mode: hybrid') { throw 'direct config must use hybrid mode' }
    if ($configText -notmatch 'multi_port:') { throw 'direct config must configure multi-port listeners' }
    if ($configText -notmatch 'base_port: 24000') { throw 'direct config must start per-node ports at 24000' }
    if ($configText -notmatch 'skip_cert_verify: false') { throw 'direct config disabled certificate verification' }
    if ($configText -match 'username:|password:') { throw 'direct config added proxy authentication' }

    $pidFile = Join-Path $Runtime 'easy_proxies.pid'
    if (-not (Test-Path -LiteralPath $pidFile -PathType Leaf)) { throw 'easy_proxies pid file was not written' }
    $runningPid = [int]((Get-Content -Raw -LiteralPath $pidFile).Trim())

    $status = Invoke-ProxyChain (@('-Action', 'status') + $commonArgs) 'status'
    if ($status.ExitCode -ne 0) { throw "status failed: $($status.Out) $($status.Err)" }
    if ($status.Out -notmatch 'running \(easy_proxies_pid=') { throw "status did not report direct running: $($status.Out)" }

    $stop = Invoke-ProxyChain (@('-Action', 'stop') + $commonArgs) 'stop'
    if ($stop.ExitCode -ne 0) { throw "stop failed: $($stop.Out) $($stop.Err)" }
    Wait-ForProcessGone $runningPid

    $statusAfterStop = Invoke-ProxyChain (@('-Action', 'status') + $commonArgs) 'status_after_stop'
    if ($statusAfterStop.Out -notmatch 'stopped') { throw "status after stop did not report stopped: $($statusAfterStop.Out)" }

    Remove-Item -LiteralPath $easyArgsPath -Force -ErrorAction SilentlyContinue
    $failStart = Invoke-ProxyChain @(
        '-Action', 'start',
        '-ResolverScript', $failResolver,
        '-ResolverStatePath', $statePath,
        '-EasyProxiesBin', $pwsh,
        '-EasyProxiesArgumentPrefix', ('-NoProfile -ExecutionPolicy Bypass -File ' + $fakeEasyScript),
        '-WebUiPort', '0',
        '-SkipLogRedirect'
    ) 'fail_start'
    if ($failStart.ExitCode -eq 0) { throw 'start with failing resolver unexpectedly succeeded' }
    if (Test-Path -LiteralPath $easyArgsPath -PathType Leaf) { throw 'easy_proxies was started after resolver failure' }
    if (Test-Path -LiteralPath $generatedConfig -PathType Leaf) { throw 'stale direct config remained after resolver failure' }
    if ($failStart.Out -match 'fixture-secret' -or $failStart.Err -match 'fixture-secret') { throw 'failure output leaked subscription secret' }

    Write-Output 'proxy-chain direct tests passed'
} finally {
    $env:FAKE_EASY_ARGS = $oldEasyArgs
    $env:FAKE_EASY_PORT = $oldEasyPort

    $pidFile = Join-Path $Runtime 'easy_proxies.pid'
    if (Test-Path -LiteralPath $pidFile -PathType Leaf) {
        $pidValue = (Get-Content -Raw -LiteralPath $pidFile).Trim()
        if ($pidValue -and (Get-Process -Id ([int]$pidValue) -ErrorAction SilentlyContinue)) {
            Stop-Process -Id ([int]$pidValue) -Force -ErrorAction SilentlyContinue
        }
    }
    Remove-Item -LiteralPath $TempRoot -Recurse -Force -ErrorAction SilentlyContinue
}
