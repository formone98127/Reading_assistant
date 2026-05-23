# Reading Assistant

Read English sentence-by-sentence; press **↓** for Gemma-powered simplification (local Ollama).

## Desktop GUI (default)

1. Install [Ollama](https://ollama.com) and pull a model, e.g. `ollama pull gemma4:latest`
2. Install MinGW for Fyne: `winget install BrechtSanders.WinLibs.POSIX.UCRT`
3. `go run .` or `.\build.ps1` then `.\reading-assistant.exe`

## Web app (optional)

```powershell
go run ./cmd/web
# open http://localhost:8080
```

```powershell
go build -o reading-assistant-web.exe ./cmd/web
```

## Environment

| Variable | Default |
|----------|---------|
| `PORT` / `READING_ASSISTANT_PORT` | `8080` (web only) |
| `OLLAMA_URL` | `http://127.0.0.1:11434` |
| `OLLAMA_MODEL` | `gemma4:latest` |
| `SAVE_DIR` | `~/reading-assistant/saves` |
