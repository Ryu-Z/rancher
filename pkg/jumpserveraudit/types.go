package jumpserveraudit

import (
	"encoding/json"
	"io"
	"time"
)

const defaultReplayPrefix = "replay"

type Config struct {
	Enabled bool `json:"enabled"`

	CommandStorageType string   `json:"commandStorageType"`
	CommandFilePath    string   `json:"commandFilePath,omitempty"`
	CommandES          ESConfig `json:"commandElasticsearch,omitempty"`

	ReplayStorageType string        `json:"replayStorageType"`
	ReplayS3          S3Config      `json:"replayS3,omitempty"`
	ReplayPresignTTL  time.Duration `json:"-"`
	MaxUploadBytes    int64         `json:"maxUploadBytes"`
}

type ESConfig struct {
	URL                string `json:"url,omitempty"`
	Username           string `json:"username,omitempty"`
	Password           string `json:"-"`
	Index              string `json:"index,omitempty"`
	IndexByDate        bool   `json:"indexByDate"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
}

type S3Config struct {
	Bucket         string `json:"bucket,omitempty"`
	Region         string `json:"region,omitempty"`
	Endpoint       string `json:"endpoint,omitempty"`
	AccessKey      string `json:"accessKey,omitempty"`
	SecretKey      string `json:"-"`
	ForcePathStyle bool   `json:"forcePathStyle"`
	Prefix         string `json:"prefix,omitempty"`
	ServerSideEnc  string `json:"serverSideEncryption,omitempty"`
}

type CommandRecord struct {
	ID         string `json:"id,omitempty"`
	User       string `json:"user,omitempty"`
	Asset      string `json:"asset,omitempty"`
	Account    string `json:"account,omitempty"`
	Session    string `json:"session,omitempty"`
	Input      string `json:"input,omitempty"`
	Output     string `json:"output,omitempty"`
	RiskLevel  int    `json:"riskLevel,omitempty"`
	OrgID      string `json:"orgId,omitempty"`
	RemoteAddr string `json:"remoteAddr,omitempty"`
	Timestamp  int64  `json:"timestamp,omitempty"`
}

func (c *CommandRecord) UnmarshalJSON(data []byte) error {
	type commandRecord CommandRecord
	aux := struct {
		commandRecord
		RiskLevelSnake  *int   `json:"risk_level"`
		OrgIDSnake      string `json:"org_id"`
		RemoteAddrSnake string `json:"remote_addr"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*c = CommandRecord(aux.commandRecord)
	if aux.RiskLevelSnake != nil {
		c.RiskLevel = *aux.RiskLevelSnake
	}
	if aux.OrgIDSnake != "" {
		c.OrgID = aux.OrgIDSnake
	}
	if aux.RemoteAddrSnake != "" {
		c.RemoteAddr = aux.RemoteAddrSnake
	}
	return nil
}

type CommandQuery struct {
	Session string
	User    string
	Asset   string
	Account string
	Input   string
	From    int64
	To      int64
	Limit   int
}

type CommandStore interface {
	Save(commands []CommandRecord) error
	Query(query CommandQuery) ([]CommandRecord, error)
	Ping() error
	Type() string
}

type ReplayStore interface {
	Upload(key string, contentType string, body io.Reader) error
	Download(key string) (io.ReadCloser, string, error)
	Presign(key string, ttl time.Duration) (string, error)
	Exists(key string) (bool, error)
	Type() string
}

type statusResponse struct {
	Enabled              bool   `json:"enabled"`
	CommandStorageType   string `json:"commandStorageType"`
	CommandStorageReady  bool   `json:"commandStorageReady"`
	CommandStorageError  string `json:"commandStorageError,omitempty"`
	ReplayStorageType    string `json:"replayStorageType"`
	ReplayStorageReady   bool   `json:"replayStorageReady"`
	ReplayStorageError   string `json:"replayStorageError,omitempty"`
	ReplayPresignTTL     string `json:"replayPresignTTL"`
	MaxUploadBytes       int64  `json:"maxUploadBytes"`
	APIBasePath          string `json:"apiBasePath"`
	ReplayObjectKeyStyle string `json:"replayObjectKeyStyle"`
}
