$ErrorActionPreference = 'Stop'

$ScriptPath = Resolve-Path (Join-Path $PSScriptRoot '..\sync-vpncheap-subscription.ps1')
$TempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('vpncheap-resolver-test-' + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $TempRoot -Force | Out-Null

function Write-FakeState([string]$Path, [string]$Raw) {
    New-Item -ItemType Directory -Path (Split-Path -Parent $Path) -Force | Out-Null
    Set-Content -LiteralPath $Path -Value $Raw -Encoding utf8
}

function Invoke-Resolver([string]$State, [string]$Output) {
    $log = Join-Path $TempRoot ([System.Guid]::NewGuid().ToString('N') + '.log')
    $oldErrorAction = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    & pwsh -NoProfile -File $ScriptPath -StatePath $State -OutputPath $Output *> $log
    $exit = $LASTEXITCODE
    $ErrorActionPreference = $oldErrorAction
    return [pscustomobject]@{
        ExitCode = $exit
        Log = Get-Content -Raw -LiteralPath $log
    }
}

try {
    $fakeUrl = 'https://example.test/subscribe?token=fixture-secret'
    $statePath = Join-Path $TempRoot 'happy\app_state.json'
    $outputPath = Join-Path $TempRoot 'happy\easy_proxies-config.yaml'
    Write-FakeState $statePath (@{
        xboard_subscription = @{ subscribe_url = $fakeUrl }
    } | ConvertTo-Json -Depth 5)

    $result = Invoke-Resolver $statePath $outputPath
    if ($result.ExitCode -ne 0) { throw "happy resolver exited $($result.ExitCode): $($result.Log)" }
    if ($result.Log -match 'fixture-secret') { throw 'resolver output leaked subscription secret' }
    if (-not (Test-Path -LiteralPath $outputPath -PathType Leaf)) { throw 'happy resolver did not create output' }

    $config = Get-Content -Raw -LiteralPath $outputPath
    if ($config -notmatch 'mode: hybrid') { throw 'generated config must use hybrid mode' }
    if ($config -notmatch 'skip_cert_verify: false') { throw 'generated config must keep certificate verification enabled' }
    if ($config -notmatch 'address: "127.0.0.1"') { throw 'generated config must bind proxy listener to loopback' }
    if ($config -notmatch 'port: 2323') { throw 'generated config must use pool port 2323' }
    if ($config -notmatch 'multi_port:') { throw 'generated config must configure multi-port listeners' }
    if ($config -notmatch 'base_port: 24000') { throw 'generated config must start per-node ports at 24000' }
    if ($config -match 'username:|password:') { throw 'generated config must not add proxy authentication' }
    if ($config -notmatch [regex]::Escape($fakeUrl)) { throw 'generated config is missing subscription URL' }

    $acl = Get-Acl -LiteralPath $outputPath
    foreach ($rule in $acl.Access) {
        $identity = $rule.IdentityReference.Value
        if ($identity -match 'Everyone|Users|Authenticated Users') {
            throw "generated config ACL exposes $identity"
        }
    }

    $failureCases = @(
        @{ Name = 'missing_state'; Write = { param($p) }; Expect = 'state file is not available' },
        @{
            Name = 'malformed_json'
            Write = { param($p) Set-Content -LiteralPath $p -Value '{ bad' -Encoding utf8 }
            Expect = 'not valid JSON'
        },
        @{
            Name = 'missing_subscription'
            Write = { param($p) Set-Content -LiteralPath $p -Value (@{ other = 'x' } | ConvertTo-Json) -Encoding utf8 }
            Expect = 'subscription data is missing'
        },
        @{
            Name = 'non_string_url'
            Write = { param($p) Set-Content -LiteralPath $p -Value (@{ xboard_subscription = @{ subscribe_url = 123 } } | ConvertTo-Json -Depth 5) -Encoding utf8 }
            Expect = 'missing or not a string'
        },
        @{
            Name = 'http_url'
            Write = { param($p) Set-Content -LiteralPath $p -Value (@{ xboard_subscription = @{ subscribe_url = 'http://example.test/sub' } } | ConvertTo-Json -Depth 5) -Encoding utf8 }
            Expect = 'must use https'
        }
    )

    foreach ($case in $failureCases) {
        $caseState = Join-Path $TempRoot ($case.Name + '\app_state.json')
        $caseOutput = Join-Path $TempRoot ($case.Name + '\easy_proxies-config.yaml')
        New-Item -ItemType Directory -Path (Split-Path -Parent $caseState) -Force | Out-Null
        & $case.Write $caseState
        Set-Content -LiteralPath $caseOutput -Value 'stale' -Encoding utf8
        $failed = Invoke-Resolver $caseState $caseOutput
        if ($failed.ExitCode -eq 0) { throw "$($case.Name) unexpectedly succeeded" }
        if ($failed.Log -notmatch [regex]::Escape($case.Expect)) { throw "$($case.Name) did not report expected error: $($failed.Log)" }
        if ($failed.Log -match 'fixture-secret|example.test') { throw "$($case.Name) leaked URL or secret" }
        if (Test-Path -LiteralPath $caseOutput -PathType Leaf) { throw "$($case.Name) left stale output config" }
    }

    $dirOutput = Join-Path $TempRoot 'write_failure\easy_proxies-config.yaml'
    New-Item -ItemType Directory -Path $dirOutput -Force | Out-Null
    $dirState = Join-Path $TempRoot 'write_failure\app_state.json'
    Write-FakeState $dirState (@{
        xboard_subscription = @{ subscribe_url = $fakeUrl }
    } | ConvertTo-Json -Depth 5)
    $writeFailure = Invoke-Resolver $dirState $dirOutput
    if ($writeFailure.ExitCode -eq 0) { throw 'write failure unexpectedly succeeded' }
    if ($writeFailure.Log -match 'fixture-secret') { throw 'write failure output leaked subscription secret' }

    Write-Output 'sync-vpncheap-subscription tests passed'
} finally {
    Remove-Item -LiteralPath $TempRoot -Recurse -Force -ErrorAction SilentlyContinue
}
