# Terminal Audit Integration

This package ports the server-side parts of terminal session command audit and replay storage into Rancher.

The module is mounted behind Rancher's authenticated router at:

```text
/v3/terminal-audit
```

## Endpoints

```text
GET  /v3/terminal-audit/status
POST /v3/terminal-audit/commands
GET  /v3/terminal-audit/commands?sessionId=<session>
PUT  /v3/terminal-audit/replays/{sessionID}/{filename}?date=YYYY-MM-DD
GET  /v3/terminal-audit/replays/{sessionID}/{filename}?date=YYYY-MM-DD
GET  /v3/terminal-audit/replays/{sessionID}/{filename}/url?date=YYYY-MM-DD
```

Replay objects use the terminal replay key layout:

```text
replay/{date}/{sessionID}/{filename}
```

## Environment

```bash
CATTLE_TERMINAL_AUDIT_ENABLED=true

# Command audit backend: elasticsearch, file, disabled
CATTLE_TERMINAL_COMMAND_STORAGE=elasticsearch
CATTLE_TERMINAL_COMMAND_ES_URL=https://elasticsearch.example.com:9200
CATTLE_TERMINAL_COMMAND_ES_USERNAME=terminal
CATTLE_TERMINAL_COMMAND_ES_PASSWORD=<password>
CATTLE_TERMINAL_COMMAND_ES_INDEX=terminal-command
CATTLE_TERMINAL_COMMAND_ES_INDEX_BY_DATE=true

# Replay backend: s3, disabled
CATTLE_TERMINAL_REPLAY_STORAGE=s3
CATTLE_TERMINAL_REPLAY_S3_BUCKET=terminal-replay-prod
CATTLE_TERMINAL_REPLAY_S3_REGION=us-east-1
CATTLE_TERMINAL_REPLAY_S3_ENDPOINT=https://s3.us-east-1.amazonaws.com
CATTLE_TERMINAL_REPLAY_S3_ACCESS_KEY=<access-key>
CATTLE_TERMINAL_REPLAY_S3_SECRET_KEY=<secret-key>
CATTLE_TERMINAL_REPLAY_S3_PREFIX=replay
CATTLE_TERMINAL_REPLAY_PRESIGN_TTL_SECONDS=3600
CATTLE_TERMINAL_MAX_UPLOAD_BYTES=1073741824
```

For production, prefer Elasticsearch for command audit and S3 lifecycle policies for replay retention.
