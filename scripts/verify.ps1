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

	New-Item -ItemType Directory -Force .cache/verify/coverage | Out-Null
	$coverageFailures = @()
	go list ./internal/... | ForEach-Object {
		$package = $_
		$profileName = $package -replace '[^A-Za-z0-9_-]', '_'
		$profile = ".cache/verify/coverage/$profileName.out"
		$coverageArguments = @("test", "-count=1", "-covermode=atomic", "-coverprofile=$profile")
		if ($Race) {
			$coverageArguments += "-race"
		}
		$coverageArguments += $package
		& go @coverageArguments | Out-Host
		if ($LASTEXITCODE -ne 0) {
			throw "coverage test failed for $package"
		}
		$total = (& go tool cover "-func=$profile" | Select-String '^total:') -split '\s+'
		$percentage = [double]$total[-1].TrimEnd('%')
		Write-Host ("{0,-70} {1,6:N1}%" -f $package, $percentage)
		if ($percentage -le 75) {
			$coverageFailures += "$package coverage $percentage% must be greater than 75%"
		}
	}
	if ($coverageFailures.Count -gt 0) {
		throw ($coverageFailures -join "`n")
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
    go build -o .cache/verify/identity.exe ./examples/identity
    if ($LASTEXITCODE -ne 0) {
        throw "identity-service build failed"
    }
	go build -o .cache/verify/grpc-health.exe ./examples/grpc-health
	if ($LASTEXITCODE -ne 0) {
		throw "gRPC health service build failed"
	}

    Write-Host "Verification completed successfully."
} finally {
    Pop-Location
}
