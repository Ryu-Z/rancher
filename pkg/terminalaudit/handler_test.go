package terminalaudit

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDecodeCommandRecordsAcceptsTerminalSnakeCase(t *testing.T) {
	body := []byte(`[
		{
			"user": "admin",
			"asset": "node-1",
			"account": "root",
			"session": "s-1",
			"input": "whoami",
			"output": "cm9vdAo=",
			"risk_level": 4,
			"org_id": "00000000-0000-0000-0000-000000000000",
			"remote_addr": "10.0.0.10",
			"timestamp": 1710000000
		}
	]`)

	records, err := decodeCommandRecords(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one record, got %d", len(records))
	}
	record := records[0]
	if record.RiskLevel != 4 {
		t.Fatalf("expected snake_case risk level to be decoded, got %d", record.RiskLevel)
	}
	if record.OrgID == "" || record.RemoteAddr == "" {
		t.Fatalf("expected org_id and remote_addr to be decoded: %#v", record)
	}
}

func TestConfigDefaultsWhenFeatureEnabled(t *testing.T) {
	oldEnabled := os.Getenv("CATTLE_TERMINAL_AUDIT_ENABLED")
	oldCommandStorage := os.Getenv("CATTLE_TERMINAL_COMMAND_STORAGE")
	oldReplayStorage := os.Getenv("CATTLE_TERMINAL_REPLAY_STORAGE")
	defer func() {
		_ = os.Setenv("CATTLE_TERMINAL_AUDIT_ENABLED", oldEnabled)
		_ = os.Setenv("CATTLE_TERMINAL_COMMAND_STORAGE", oldCommandStorage)
		_ = os.Setenv("CATTLE_TERMINAL_REPLAY_STORAGE", oldReplayStorage)
	}()

	_ = os.Setenv("CATTLE_TERMINAL_AUDIT_ENABLED", "true")
	_ = os.Unsetenv("CATTLE_TERMINAL_COMMAND_STORAGE")
	_ = os.Unsetenv("CATTLE_TERMINAL_REPLAY_STORAGE")

	cfg := NewConfigFromEnv()
	if cfg.CommandStorageType != commandStorageFile {
		t.Fatalf("expected default command file storage, got %s", cfg.CommandStorageType)
	}
	if cfg.ReplayStorageType != replayStorageS3 {
		t.Fatalf("expected default replay s3 storage, got %s", cfg.ReplayStorageType)
	}
}

func TestFileCommandStoreSaveAndQuery(t *testing.T) {
	store := NewFileCommandStore(filepath.Join(tempDir(t), "commands.jsonl"))
	err := store.Save([]CommandRecord{
		{Session: "s-1", User: "alice", Asset: "node-1", Account: "root", Input: "whoami", Timestamp: 1710000000},
		{Session: "s-2", User: "bob", Asset: "node-2", Account: "root", Input: "id", Timestamp: 1710000100},
	})
	if err != nil {
		t.Fatal(err)
	}

	records, err := store.Query(CommandQuery{Session: "s-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one record, got %d", len(records))
	}
	if records[0].Input != "whoami" {
		t.Fatalf("unexpected command: %#v", records[0])
	}
}

func TestHandlerCommandEndpoints(t *testing.T) {
	cfg := Config{
		CommandStorageType: commandStorageFile,
		CommandFilePath:    filepath.Join(tempDir(t), "commands.jsonl"),
		ReplayStorageType:  replayStorageDisabled,
		ReplayPresignTTL:   time.Hour,
		MaxUploadBytes:     1024,
	}
	handler, err := NewHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, apiBasePath+"/commands", bytes.NewBufferString(`{"session":"s-1","input":"whoami","timestamp":1710000000}`))
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	if rw.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rw.Code, rw.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, apiBasePath+"/commands?sessionId=s-1", nil)
	rw = httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rw.Code, rw.Body.String())
	}

	var response struct {
		Data []CommandRecord `json:"data"`
	}
	if err := json.Unmarshal(rw.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data) != 1 || response.Data[0].Input != "whoami" {
		t.Fatalf("unexpected response: %s", rw.Body.String())
	}
}

func TestHandlerReplayUploadBuildsTerminalStyleKey(t *testing.T) {
	replay := &fakeReplayStore{}
	handler := &Handler{
		cfg: Config{
			ReplayStorageType: replayStorageS3,
			ReplayS3:          S3Config{Prefix: "replay"},
			ReplayPresignTTL:  time.Hour,
			MaxUploadBytes:    1024,
		},
		replays: replay,
	}

	req := httptest.NewRequest(http.MethodPut, apiBasePath+"/replays/s-1/s-1.cast.gz?date=2026-05-06", bytes.NewBufferString("cast-data"))
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	if rw.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rw.Code, rw.Body.String())
	}
	if replay.key != "replay/2026-05-06/s-1/s-1.cast.gz" {
		t.Fatalf("unexpected replay key: %s", replay.key)
	}
	if replay.body != "cast-data" {
		t.Fatalf("unexpected replay body: %s", replay.body)
	}
}

func tempDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "terminalaudit-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

type fakeReplayStore struct {
	key  string
	body string
}

func (f *fakeReplayStore) Upload(key string, contentType string, body io.Reader) error {
	data, err := ioutil.ReadAll(body)
	if err != nil {
		return err
	}
	f.key = key
	f.body = string(data)
	return nil
}

func (f *fakeReplayStore) Download(key string) (io.ReadCloser, string, error) {
	return ioutil.NopCloser(bytes.NewBufferString(f.body)), "application/octet-stream", nil
}

func (f *fakeReplayStore) Presign(key string, ttl time.Duration) (string, error) {
	return "https://example.invalid/" + key, nil
}

func (f *fakeReplayStore) Exists(key string) (bool, error) {
	return true, nil
}

func (f *fakeReplayStore) Type() string {
	return replayStorageS3
}
