# Desktop GUI (default) — requires gcc: winget install BrechtSanders.WinLibs.POSIX.UCRT
$env:CGO_ENABLED = "1"
go build -o reading-assistant.exe .
if ($LASTEXITCODE -eq 0) { Write-Host "Built reading-assistant.exe (desktop GUI)" }

# Web app (no CGO):
# go build -o reading-assistant-web.exe ./cmd/web
