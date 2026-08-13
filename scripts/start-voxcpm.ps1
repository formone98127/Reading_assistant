# Start VoxCPM sidecar on port 8808 (Reading Assistant TTS).
param(
    [int]$Port = 8808,
    [string]$BindAddress = "127.0.0.1"
)

$ErrorActionPreference = "Stop"
$script = Join-Path $PSScriptRoot "voxcpm_server.py"

if (-not (Get-Command python -ErrorAction SilentlyContinue)) {
    throw "Python not found. Install Python 3.10+ and: pip install voxcpm soundfile"
}

# Kill stale listeners on this port (multiple instances cause empty HTTP replies).
$existing = netstat -ano | Select-String ":${Port}\s+.*LISTENING"
foreach ($line in $existing) {
    if ($line -match '\s(\d+)\s*$') {
        $procId = [int]$matches[1]
        if ($procId -gt 0) {
            Write-Host "Stopping stale process on port ${Port} (PID $procId)"
            Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue
        }
    }
}
Start-Sleep -Milliseconds 400

Write-Host "Starting VoxCPM sidecar on http://${BindAddress}:${Port}"
Write-Host "First run downloads VoxCPM2 weights (may take a while)."
Write-Host "In the app: Upload file tab -> Voice (VoxCPM) -> Generate voice on upload."
Write-Host "Press Ctrl+C to stop."
python $script --host $BindAddress --port $Port
