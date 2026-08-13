# Start Freebuff2API on port 8081 (no Docker). Reading Assistant expects http://127.0.0.1:8081/v1
param(
    [string]$Token = $env:AUTH_TOKENS,
    [int]$Port = 8081
)

$ErrorActionPreference = "Stop"
$bin = Join-Path $env:USERPROFILE "go\bin\Freebuff2API.exe"
if (-not (Test-Path $bin)) {
    Write-Host "Installing Freebuff2API (one-time)..."
    go install github.com/Quorinex/Freebuff2API@latest
}
if (-not $Token) {
    $cfgPath = Join-Path $PSScriptRoot "freebuff2api\config.json"
    if (Test-Path $cfgPath) {
        try {
            $cfg = Get-Content $cfgPath -Raw | ConvertFrom-Json
            if ($cfg.AUTH_TOKENS -and $cfg.AUTH_TOKENS.Count -gt 0) {
                $Token = [string]$cfg.AUTH_TOKENS[0]
                Write-Host "Using token from scripts\freebuff2api\config.json"
            }
        } catch {
            Write-Host "Could not read $cfgPath : $_"
        }
    }
}
if (-not $Token) {
    Write-Host ""
    Write-Host "Need a FreeBuff auth token."
    Write-Host "  1) Open https://freebuff.llm.pm and copy your token"
    Write-Host "  2) Or: npm i -g freebuff  ->  run freebuff  ->  token in"
    Write-Host "     %USERPROFILE%\.config\manicode\credentials.json  (authToken field)"
    Write-Host ""
    $Token = Read-Host "Paste AUTH_TOKEN"
}
if (-not $Token) { throw "No token provided" }

$dir = Join-Path $PSScriptRoot "freebuff2api"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
# Use env vars (avoids PowerShell UTF-8 BOM breaking config.json parsing)
$env:LISTEN_ADDR = ":$Port"
$env:AUTH_TOKENS = $Token.Trim()
$env:ROTATION_INTERVAL = "6h"
$env:REQUEST_TIMEOUT = "15m"

Write-Host "Starting Freebuff2API on http://127.0.0.1:${Port}/v1"
Write-Host "In Reading Assistant: Rewrite LLM -> FreeBuff, pick minimax/kimi/deepseek from dropdown (not Gemini)."
Write-Host "Press Ctrl+C to stop."
& $bin
