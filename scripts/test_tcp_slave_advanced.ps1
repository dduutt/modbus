param(
    [string]$Address = "127.0.0.1",
    [int]$Port = 1503,
    [int]$StartupTimeoutSeconds = 5
)

$ErrorActionPreference = "Stop"

if ($Address -ne "127.0.0.1" -or $Port -ne 1503) {
    throw "examples/tcp_slave_advanced and examples/tcp_client_advanced currently use fixed address 127.0.0.1:1503"
}

$binDir = Join-Path (Get-Location) "bin"
$exePath = Join-Path $binDir "tcp_slave_advanced.exe"
$stdoutPath = Join-Path $binDir "tcp_slave_advanced_test.log"
$stderrPath = Join-Path $binDir "tcp_slave_advanced_test.err.log"
$startedProcess = $null

if (!(Test-Path $binDir)) {
    New-Item -ItemType Directory -Path $binDir | Out-Null
}

function Get-Listener {
    Get-NetTCPConnection -LocalAddress $Address -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
}

try {
    $listener = Get-Listener
    if ($listener) {
        Write-Host "using existing advanced tcp slave on ${Address}:${Port} pid=$($listener.OwningProcess)"
    } else {
        Write-Host "building advanced tcp slave"
        go build -o $exePath ./examples/tcp_slave_advanced

        Write-Host "starting advanced tcp slave on ${Address}:${Port}"
        $startedProcess = Start-Process `
            -FilePath $exePath `
            -WorkingDirectory (Get-Location) `
            -RedirectStandardOutput $stdoutPath `
            -RedirectStandardError $stderrPath `
            -WindowStyle Hidden `
            -PassThru

        $deadline = (Get-Date).AddSeconds($StartupTimeoutSeconds)
        do {
            Start-Sleep -Milliseconds 100
            $listener = Get-Listener
            if ($listener) {
                break
            }
            if ($startedProcess.HasExited) {
                $stderr = ""
                if (Test-Path $stderrPath) {
                    $stderr = Get-Content $stderrPath -Raw
                }
                throw "advanced tcp slave exited before listening. $stderr"
            }
        } while ((Get-Date) -lt $deadline)

        if (!$listener) {
            throw "advanced tcp slave did not start listening within ${StartupTimeoutSeconds}s"
        }
    }

    Write-Host "running advanced tcp client test"
    go run ./examples/tcp_client_advanced
    Write-Host "advanced tcp slave loopback test passed"
} finally {
    if ($startedProcess -and !$startedProcess.HasExited) {
        Stop-Process -Id $startedProcess.Id -Force
        Write-Host "stopped advanced tcp slave pid=$($startedProcess.Id)"
    }
}
