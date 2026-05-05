# JumpServer Audit Integration

This package ports the server-side parts of JumpServer-style command audit and replay storage into Rancher.

The module is mounted behind Rancher's authenticated router at:

```text
/v3/jumpserver-audit
```

## Endpoints

```text
GET  /v3/jumpserver-audit/status
POST /v3/jumpserver-audit/commands
GET  /v3/jumpserver-audit/commands?sessionId=<session>
PUT  /v3/jumpserver-audit/replays/{sessionID}/{filename}?date=YYYY-MM-DD
GET  /v3/jumpserver-audit/replays/{sessionID}/{filename}?date=YYYY-MM-DD
GET  /v3/jumpserver-audit/replays/{sessionID}/{filename}/url?date=YYYY-MM-DD
```

Replay objects use the JumpServer-compatible key layout:

```text
replay/{date}/{sessionID}/{filename}
```

## Environment

```bash
CATTLE_JUMPSERVER_AUDIT_ENABLED=true

# Command audit backend: elasticsearch, file, disabled
CATTLE_JUMPSERVER_COMMAND_STORAGE=elasticsearch
CATTLE_JUMPSERVER_COMMAND_ES_URL=https://elasticsearch.example.com:9200
CATTLE_JUMPSERVER_COMMAND_ES_USERNAME=jumpserver
CATTLE_JUMPSERVER_COMMAND_ES_PASSWORD=<password>
CATTLE_JUMPSERVER_COMMAND_ES_INDEX=jumpserver-command
CATTLE_JUMPSERVER_COMMAND_ES_INDEX_BY_DATE=true

# Replay backend: s3, disabled
CATTLE_JUMPSERVER_REPLAY_STORAGE=s3
CATTLE_JUMPSERVER_REPLAY_S3_BUCKET=jumpserver-replay-prod
CATTLE_JUMPSERVER_REPLAY_S3_REGION=us-east-1
CATTLE_JUMPSERVER_REPLAY_S3_ENDPOINT=https://s3.us-east-1.amazonaws.com
CATTLE_JUMPSERVER_REPLAY_S3_ACCESS_KEY=<access-key>
CATTLE_JUMPSERVER_REPLAY_S3_SECRET_KEY=<secret-key>
CATTLE_JUMPSERVER_REPLAY_S3_PREFIX=replay
CATTLE_JUMPSERVER_REPLAY_PRESIGN_TTL_SECONDS=3600
CATTLE_JUMPSERVER_MAX_UPLOAD_BYTES=1073741824
```

For production, prefer Elasticsearch for command audit and S3 lifecycle policies for replay retention.
