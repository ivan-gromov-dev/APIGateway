[CmdletBinding()]
param([string]$BaseURL = "http://localhost:8080", [string]$AdminURL = "http://localhost:9090")

$ErrorActionPreference = "Stop"
function Assert-Status([string]$URL, [int]$Expected = 200) {
    $response = Invoke-WebRequest -UseBasicParsing -Uri $URL
    if ($response.StatusCode -ne $Expected) { throw "$URL returned $($response.StatusCode), expected $Expected" }
    return $response
}

Assert-Status "$AdminURL/healthz" | Out-Null
Assert-Status "$AdminURL/readyz" | Out-Null
$first = Assert-Status "$BaseURL/api/billing/invoices"
$second = Assert-Status "$BaseURL/api/billing/invoices"
if ($first.Headers["X-Cache"] -ne "MISS" -and $first.Headers["X-Cache"] -ne "HIT") { throw "billing cache was not active" }
if ($second.Headers["X-Cache"] -ne "HIT") { throw "billing cache did not produce a hit" }

$basic = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes("gateway-demo:gateway-demo-secret"))
$token = (Invoke-RestMethod -Method Post -Uri "http://localhost:8084/token" -Headers @{ Authorization = "Basic $basic" } -Body @{ grant_type = "client_credentials"; scope = "users.read" }).access_token
$users = Invoke-WebRequest -UseBasicParsing -Uri "$BaseURL/api/users" -Headers @{ Authorization = "Bearer $token"; "X-User-ID" = "smoke-user" }
if ($users.StatusCode -ne 200) { throw "authenticated users request failed" }

$metrics = (Assert-Status "$AdminURL/metrics").Content
foreach ($metric in @("gateway_http_requests_total", "gateway_feature_decisions_total", "gateway_discovery_refresh_total", "gateway_discovery_targets")) {
    if (-not $metrics.Contains($metric)) { throw "metric $metric was not exported" }
}
go run ./examples/grpc-health -check localhost:8080
if ($LASTEXITCODE -ne 0) { throw "gRPC proxy smoke check failed" }
Write-Host "Docker smoke checks passed."
