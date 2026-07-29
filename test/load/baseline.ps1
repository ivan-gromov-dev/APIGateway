[CmdletBinding()]
param(
    [string]$Duration = "60s",
    [ValidateRange(1, 10000)]
    [int]$Concurrency = 100,
    [ValidateRange(1, 1000000)]
    [int]$WarmupRequests = 1000,
    [ValidateRange(1, 10000)]
    [int]$WarmupConcurrency = 20,
    [string]$GatewayURL = "http://localhost:8080",
    [string]$AdminURL = "http://localhost:9090"
)

$ErrorActionPreference = "Stop"

function Resolve-Hey {
    $command = Get-Command "hey" -ErrorAction SilentlyContinue
    if ($null -ne $command) {
        return $command.Source
    }

    $goHey = Join-Path $env:USERPROFILE "go\bin\hey.exe"
    if (Test-Path -LiteralPath $goHey) {
        return $goHey
    }

    throw @"
hey was not found.
Install it with:
  go install github.com/rakyll/hey@latest
"@
}

function Invoke-HeyScenario {
    param(
        [Parameter(Mandatory)]
        [string]$Name,
        [Parameter(Mandatory)]
        [string[]]$Arguments,
        [Parameter(Mandatory)]
        [string]$OutputFile
    )

    $separator = "=" * 80
    @(
        ""
        $separator
        "Scenario: $Name"
        "Started:  $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss K')"
        "Command:  hey $($Arguments -join ' ')"
        $separator
    ) | Tee-Object -FilePath $OutputFile -Append

    & $script:HeyPath @Arguments 2>&1 |
        Tee-Object -FilePath $OutputFile -Append

    if ($LASTEXITCODE -ne 0) {
        throw "hey failed for scenario '$Name' with exit code $LASTEXITCODE"
    }
}

$script:HeyPath = Resolve-Hey
$readyURL = "$($AdminURL.TrimEnd('/'))/readyz"

try {
    $ready = Invoke-RestMethod -Method Get -Uri $readyURL -TimeoutSec 5
} catch {
    throw "Gateway is not ready at $readyURL. Start it with 'docker compose up --build -d'. $($_.Exception.Message)"
}

if ($ready.status -ne "ready") {
    throw "Gateway returned an unexpected readiness response: $($ready | ConvertTo-Json -Compress)"
}

$resultsDirectory = Join-Path $PSScriptRoot "results"
New-Item -ItemType Directory -Force -Path $resultsDirectory | Out-Null

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$resultFile = Join-Path $resultsDirectory "baseline-$timestamp.txt"
$gatewayBaseURL = $GatewayURL.TrimEnd("/")
$writeConcurrency = [Math]::Max(1, [Math]::Floor($Concurrency / 2))

@(
    "API Gateway performance baseline"
    "Created:             $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss K')"
    "Gateway:             $gatewayBaseURL"
    "Duration:            $Duration"
    "Read concurrency:    $Concurrency"
    "Write concurrency:   $writeConcurrency"
    "Warm-up requests:    $WarmupRequests"
    "Warm-up concurrency: $WarmupConcurrency"
    "hey executable:      $script:HeyPath"
) | Set-Content -LiteralPath $resultFile -Encoding utf8

Write-Host "Warming up the gateway..."
& $script:HeyPath -n $WarmupRequests -c $WarmupConcurrency "$gatewayBaseURL/api/users" |
    Out-Null
if ($LASTEXITCODE -ne 0) {
    throw "Gateway warm-up failed with exit code $LASTEXITCODE"
}

Invoke-HeyScenario `
    -Name "Users GET" `
    -Arguments @("-z", $Duration, "-c", "$Concurrency", "$gatewayBaseURL/api/users") `
    -OutputFile $resultFile

Invoke-HeyScenario `
    -Name "Billing GET" `
    -Arguments @("-z", $Duration, "-c", "$Concurrency", "$gatewayBaseURL/api/billing/invoices") `
    -OutputFile $resultFile

Invoke-HeyScenario `
    -Name "Billing POST" `
    -Arguments @(
        "-z", $Duration,
        "-c", "$writeConcurrency",
        "-m", "POST",
        "-H", "Content-Type: application/json",
        "-d", "{}",
        "$gatewayBaseURL/api/billing/payments"
    ) `
    -OutputFile $resultFile

Write-Host ""
Write-Host "Baseline completed."
Write-Host "Results: $resultFile"
