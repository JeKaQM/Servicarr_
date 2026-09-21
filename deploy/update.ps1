[CmdletBinding()]
param(
    [switch]$SkipGitPull
)

# Rebuild and replace the Servicarr container without removing its data volume.
# Run with -SkipGitPull when updating from changes that are already present locally.

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repositoryRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
$composeFile = Join-Path $PSScriptRoot "docker-compose.yml"
$environmentFile = Join-Path $repositoryRoot ".env"

function Invoke-CheckedCommand {
    param(
        [Parameter(Mandatory = $true)]
        [string]$FilePath,

        [Parameter(Mandatory = $true)]
        [string[]]$CommandArguments,

        [Parameter(Mandatory = $true)]
        [string]$Description
    )

    Write-Host "`n$Description..." -ForegroundColor Cyan
    & $FilePath @CommandArguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Description failed with exit code $LASTEXITCODE."
    }
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "Docker was not found on PATH. Install Docker with the Compose v2 plugin before updating Servicarr."
}

if (-not (Test-Path -LiteralPath $composeFile -PathType Leaf)) {
    throw "Compose file not found: $composeFile"
}

if (-not (Test-Path -LiteralPath $environmentFile -PathType Leaf)) {
    throw "Environment file not found: $environmentFile. Copy .env.example to .env and review its values first."
}

Push-Location -LiteralPath $repositoryRoot
try {
    Invoke-CheckedCommand -FilePath "docker" -CommandArguments @("compose", "version") -Description "Checking Docker Compose"

    if (-not $SkipGitPull) {
        if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
            throw "Git was not found on PATH. Install Git or run this script with -SkipGitPull."
        }

        if (-not (Test-Path -LiteralPath (Join-Path $repositoryRoot ".git"))) {
            throw "This installation is not a Git checkout. Run this script with -SkipGitPull after updating the source manually."
        }

        $trackedChanges = & git status --porcelain --untracked-files=no
        if ($LASTEXITCODE -ne 0) {
            throw "Unable to inspect the Git working tree."
        }
        if ($trackedChanges) {
            throw "Tracked local changes were found. Commit or stash them, or use -SkipGitPull to build the current source without pulling."
        }

        Invoke-CheckedCommand -FilePath "git" -CommandArguments @("pull", "--ff-only") -Description "Updating the Servicarr source"
    }

    Invoke-CheckedCommand -FilePath "docker" -CommandArguments @(
        "compose", "-f", $composeFile, "config", "--quiet"
    ) -Description "Validating the Compose configuration"

    Invoke-CheckedCommand -FilePath "docker" -CommandArguments @(
        "compose", "-f", $composeFile, "up", "-d", "--build", "--remove-orphans"
    ) -Description "Building and starting Servicarr"

    Invoke-CheckedCommand -FilePath "docker" -CommandArguments @(
        "compose", "-f", $composeFile, "ps"
    ) -Description "Checking the Servicarr container"
}
finally {
    Pop-Location
}

Write-Host "`nUpdate complete. Data remains in the 'servicarr_data' volume." -ForegroundColor Green
Write-Host "Open Servicarr at: http://localhost:4555" -ForegroundColor Yellow
