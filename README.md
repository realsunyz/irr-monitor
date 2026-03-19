# IRR Monitor

A Telegram bot that monitors new ASN allocations from RIPE, ARIN, and APNIC.

## Features

- **RIPE**: Real-time monitoring via NRTM protocol (polling every 60s)
- **ARIN**: Real-time monitoring via NRTM protocol (polling every 60s)
- **APNIC**: Daily comparison of delegated files (UTC 16:00)
- Sends notifications to Telegram channels with links to RIPE DB / ARIN RDAP / APNIC DB and BGP.TOOLS

## Quick Start

```bash
docker run -d \
  -e TELEGRAM_BOT_TOKEN=your_token \
  -e TELEGRAM_CHANNELS=@your_channel \
  -v irr-monitor-data:/data \
  ghcr.io/realsunyz/irr-monitor:latest
```

## Terminal NRTM Test

Run a direct connectivity test for ARIN and RIPE without configuring Telegram:

```bash
go run ./cmd/irr-monitor test-nrtm --source all
```

You can also test a single source:

```bash
go run ./cmd/irr-monitor test-nrtm arin
go run ./cmd/irr-monitor test-nrtm ripe
```

## Environment Variables

| Variable             | Required | Default            | Description                                         |
| -------------------- | -------- | ------------------ | --------------------------------------------------- |
| `TELEGRAM_BOT_TOKEN` | Yes      | -                  | Telegram Bot API token                              |
| `TELEGRAM_CHANNELS`  | Yes      | -                  | Comma-separated channels (`@channel` or numeric ID) |
| `POLL_INTERVAL`      | No       | `60`               | RIPE polling interval in seconds                    |
| `STATE_FILE`         | No       | `/data/state.json` | Path to state file                                  |

## Contributing

Issues and Pull Requests are definitely welcome!

Please make sure you have tested your code locally before submitting a PR.

## License

Source code is released under the [MIT License](https://github.com/realSunyz/irr-monitor/blob/main/LICENSE).
