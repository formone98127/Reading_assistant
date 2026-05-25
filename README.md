# Group → Bot forward userbot

Forwards every new message from supergroup `-5226298353` to bot `8653334006` using your Telegram account (Telethon userbot).

## Prerequisites

1. [my.telegram.org](https://my.telegram.org/apps) — create an app, copy `api_id` and `api_hash`.
2. Your account must be a member of the source group.
3. Start a chat with the target bot at least once (open the bot in Telegram and press **Start**).

## Setup

```powershell
cd c:\Users\Administrator\Reading_assistant
python -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -r requirements.txt
copy .env.example .env
# Edit .env with TELEGRAM_API_ID and TELEGRAM_API_HASH
```

## First run (login)

```powershell
python forward_userbot.py
```

Telethon will prompt for phone number, login code, and 2FA password if enabled. This creates `userbot.session` — keep it private.

## Run in background (Windows)

```powershell
Start-Process -NoNewWindow python -ArgumentList "forward_userbot.py" -WorkingDirectory (Get-Location)
```

Or use Task Scheduler / `nssm` for a persistent service.

## Config

| Variable | Default | Description |
|----------|---------|-------------|
| `SOURCE_CHAT_ID` | `-5226298353` | Group to watch |
| `TARGET_BOT_ID` | `8653334006` | Bot user id to receive forwards |
| `TELEGRAM_SESSION` | `userbot` | Session filename |

Service messages (join/leave/pin) are skipped.
