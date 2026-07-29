[CmdletBinding()]
param(
    [switch]$Race
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot

try {
    $goFiles = Get-ChildItem cmd, examples, internal, test -Recurse -Filter *.go
    $unformatted = @(
        $goFiles | ForEach-Object {
            $current = [System.IO.File]::ReadAllText($_.FullName).Replace("`r`n", "`n")
            $formatted = ((& gofmt $_.FullName) -join "`n") + "`n"
            if ($current -ne $formatted) {
                $_.FullName
            }
        }
    )
    if ($unformatted.Count -gt 0) {
        throw "The following Go files require gofmt:`n$($unformatted -join "`n")"
    }

    go vet ./...
    if ($LASTEXITCODE -ne 0) {
        throw "go vet failed"
    }

    $testArguments = @("test", "-count=1")
    if ($Race) {
        $testArguments += "-race"
    }
    $testArguments += "./..."
    & go @testArguments
    if ($LASTEXITCODE -ne 0) {
        throw "go test failed"
    }

    New-Item -ItemType Directory -Force .cache/verify | Out-Null
    go build -o .cache/verify/gateway.exe ./cmd/gateway
    if ($LASTEXITCODE -ne 0) {
        throw "gateway build failed"
    }
    go build -o .cache/verify/users.exe ./examples/backend
    if ($LASTEXITCODE -ne 0) {
        throw "users-service build failed"
    }
    go build -o .cache/verify/billing.exe ./examples/billing
    if ($LASTEXITCODE -ne 0) {
        throw "billing-service build failed"
    }

    Write-Host "Verification completed successfully."
} finally {
    Pop-Location
}
