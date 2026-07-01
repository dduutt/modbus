param(
    [switch]$Race
)

$ErrorActionPreference = "Stop"

function Run-Step {
    param(
        [string]$Name,
        [scriptblock]$Command
    )

    Write-Host ""
    Write-Host "==> $Name"
    & $Command
}

Run-Step "go test" {
    go test -count=1 ./...
}

if ($Race) {
    Run-Step "go test -race" {
        go test -race -count=1 ./...
    }
}

Run-Step "go vet" {
    go vet ./...
}

Run-Step "tcp slave loopback" {
    .\scripts\test_tcp_slave.ps1
}

Run-Step "advanced tcp slave loopback" {
    .\scripts\test_tcp_slave_advanced.ps1
}

Run-Step "rtu over tcp loopback" {
    go run ./examples/rtu_over_tcp_loopback
}

Run-Step "tags loopback" {
    go run ./examples/tags_loopback
}

Run-Step "advanced loopback" {
    go run ./examples/advanced_loopback
}

Write-Host ""
Write-Host "all local tests passed"
