package terminalaudit

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	commandStorageDisabled = "disabled"
	commandStorageFile     = "file"
	commandStorageES       = "elasticsearch"
	replayStorageDisabled  = "disabled"
	replayStorageS3        = "s3"
)

func NewConfigFromEnv() Config {
	enabled := envBool("CATTLE_TERMINAL_AUDIT_ENABLED", false)
	commandStorage := strings.ToLower(envString("CATTLE_TERMINAL_COMMAND_STORAGE", ""))
	replayStorage := strings.ToLower(envString("CATTLE_TERMINAL_REPLAY_STORAGE", ""))
	if commandStorage == "" {
		if enabled {
			commandStorage = commandStorageFile
		} else {
			commandStorage = commandStorageDisabled
		}
	}
	if replayStorage == "" {
		if enabled {
			replayStorage = replayStorageS3
		} else {
			replayStorage = replayStorageDisabled
		}
	}

	cfg := Config{
		Enabled:            enabled,
		CommandStorageType: commandStorage,
		ReplayStorageType:  replayStorage,
		ReplayPresignTTL:   envDurationSeconds("CATTLE_TERMINAL_REPLAY_PRESIGN_TTL_SECONDS", time.Hour),
		MaxUploadBytes:     envInt64("CATTLE_TERMINAL_MAX_UPLOAD_BYTES", 1<<30),
	}

	cfg.CommandFilePath = envString("CATTLE_TERMINAL_COMMAND_FILE_PATH", "/var/lib/rancher/terminal-audit/commands.jsonl")
	cfg.CommandES = ESConfig{
		URL:                strings.TrimRight(envString("CATTLE_TERMINAL_COMMAND_ES_URL", ""), "/"),
		Username:           envString("CATTLE_TERMINAL_COMMAND_ES_USERNAME", ""),
		Password:           envString("CATTLE_TERMINAL_COMMAND_ES_PASSWORD", ""),
		Index:              envString("CATTLE_TERMINAL_COMMAND_ES_INDEX", "terminal-command"),
		IndexByDate:        envBool("CATTLE_TERMINAL_COMMAND_ES_INDEX_BY_DATE", true),
		InsecureSkipVerify: envBool("CATTLE_TERMINAL_COMMAND_ES_INSECURE_SKIP_VERIFY", false),
	}

	prefix := strings.Trim(envString("CATTLE_TERMINAL_REPLAY_S3_PREFIX", defaultReplayPrefix), "/")
	cfg.ReplayS3 = S3Config{
		Bucket:         envString("CATTLE_TERMINAL_REPLAY_S3_BUCKET", ""),
		Region:         envString("CATTLE_TERMINAL_REPLAY_S3_REGION", ""),
		Endpoint:       envString("CATTLE_TERMINAL_REPLAY_S3_ENDPOINT", ""),
		AccessKey:      envString("CATTLE_TERMINAL_REPLAY_S3_ACCESS_KEY", ""),
		SecretKey:      envString("CATTLE_TERMINAL_REPLAY_S3_SECRET_KEY", ""),
		ForcePathStyle: envBool("CATTLE_TERMINAL_REPLAY_S3_FORCE_PATH_STYLE", false),
		Prefix:         prefix,
		ServerSideEnc:  envString("CATTLE_TERMINAL_REPLAY_S3_SSE", ""),
	}

	return cfg
}

func envString(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt64(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envDurationSeconds(name string, fallback time.Duration) time.Duration {
	seconds := envInt64(name, int64(fallback/time.Second))
	return time.Duration(seconds) * time.Second
}
