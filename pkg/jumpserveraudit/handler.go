package jumpserveraudit

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const apiBasePath = "/v3/jumpserver-audit"

type Handler struct {
	cfg      Config
	commands CommandStore
	replays  ReplayStore
}

func NewHandlerFromEnv() http.Handler {
	cfg := NewConfigFromEnv()
	handler, err := NewHandler(cfg)
	if err != nil {
		logrus.Warnf("JumpServer audit integration is not fully configured: %v", err)
	}
	if handler != nil {
		return handler
	}
	return &Handler{cfg: cfg}
}

func NewHandler(cfg Config) (*Handler, error) {
	h := &Handler{cfg: cfg}
	var setupErr error

	switch cfg.CommandStorageType {
	case commandStorageES:
		cfg.CommandES.URL = normalizeESURL(cfg.CommandES.URL)
		store, err := NewESCommandStore(cfg.CommandES)
		if err != nil {
			setupErr = err
		} else {
			h.commands = store
		}
	case commandStorageFile:
		h.commands = NewFileCommandStore(cfg.CommandFilePath)
	case "", commandStorageDisabled:
	default:
		setupErr = fmt.Errorf("unsupported command storage type %q", cfg.CommandStorageType)
	}

	switch cfg.ReplayStorageType {
	case replayStorageS3:
		store, err := NewS3ReplayStore(cfg.ReplayS3)
		if err != nil {
			if setupErr == nil {
				setupErr = err
			}
		} else {
			h.replays = store
		}
	case "", replayStorageDisabled:
	default:
		if setupErr == nil {
			setupErr = fmt.Errorf("unsupported replay storage type %q", cfg.ReplayStorageType)
		}
	}

	return h, setupErr
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, apiBasePath)
	if path == "" {
		path = "/"
	}

	switch {
	case path == "/" || path == "/status":
		h.handleStatus(rw, req)
	case path == "/commands":
		h.handleCommands(rw, req)
	case strings.HasPrefix(path, "/replays/"):
		h.handleReplays(rw, req, strings.TrimPrefix(path, "/replays/"))
	default:
		writeError(rw, http.StatusNotFound, "not found")
	}
}

func (h *Handler) handleStatus(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	status := statusResponse{
		Enabled:              h.cfg.Enabled,
		CommandStorageType:   h.cfg.CommandStorageType,
		ReplayStorageType:    h.cfg.ReplayStorageType,
		ReplayPresignTTL:     h.cfg.ReplayPresignTTL.String(),
		MaxUploadBytes:       h.cfg.MaxUploadBytes,
		APIBasePath:          apiBasePath,
		ReplayObjectKeyStyle: "replay/{date}/{sessionID}/{filename}",
	}
	if h.commands != nil {
		status.CommandStorageReady = true
		if err := h.commands.Ping(); err != nil {
			status.CommandStorageReady = false
			status.CommandStorageError = err.Error()
		}
	}
	if h.replays != nil {
		status.ReplayStorageReady = true
	}
	writeJSON(rw, http.StatusOK, status)
}

func (h *Handler) handleCommands(rw http.ResponseWriter, req *http.Request) {
	if h.commands == nil {
		writeError(rw, http.StatusServiceUnavailable, "command audit storage is not configured")
		return
	}

	switch req.Method {
	case http.MethodPost:
		h.createCommands(rw, req)
	case http.MethodGet:
		h.queryCommands(rw, req)
	default:
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) createCommands(rw http.ResponseWriter, req *http.Request) {
	body, err := readLimitedBody(rw, req, 10<<20)
	if err != nil {
		writeError(rw, http.StatusBadRequest, err.Error())
		return
	}
	commands, err := decodeCommandRecords(body)
	if err != nil {
		writeError(rw, http.StatusBadRequest, err.Error())
		return
	}
	if len(commands) == 0 {
		writeError(rw, http.StatusBadRequest, "empty command records")
		return
	}
	for _, command := range commands {
		if command.Session == "" || command.Input == "" {
			writeError(rw, http.StatusBadRequest, "session and input are required for every command")
			return
		}
	}
	if err := h.commands.Save(commands); err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusCreated, map[string]interface{}{"saved": len(commands)})
}

func (h *Handler) queryCommands(rw http.ResponseWriter, req *http.Request) {
	query := commandQueryFromRequest(req)
	commands, err := h.commands.Query(query)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusOK, map[string]interface{}{
		"data":  commands,
		"count": len(commands),
	})
}

func (h *Handler) handleReplays(rw http.ResponseWriter, req *http.Request, subpath string) {
	if h.replays == nil {
		writeError(rw, http.StatusServiceUnavailable, "replay storage is not configured")
		return
	}

	parts := strings.Split(strings.Trim(subpath, "/"), "/")
	if len(parts) < 2 {
		writeError(rw, http.StatusBadRequest, "expected /replays/{sessionID}/{filename}")
		return
	}
	sessionID := parts[0]
	filename := parts[1]
	if sessionID == "" || filename == "" {
		writeError(rw, http.StatusBadRequest, "sessionID and filename are required")
		return
	}
	if sanitizePathSegment(sessionID) != sessionID || sanitizePathSegment(filename) != filename {
		writeError(rw, http.StatusBadRequest, "sessionID and filename must not contain path separators")
		return
	}

	key := replayObjectKey(h.cfg.ReplayS3.Prefix, req.URL.Query().Get("date"), sessionID, filename)
	if len(parts) == 3 && parts[2] == "url" {
		if req.Method != http.MethodGet {
			writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.presignReplay(rw, key)
		return
	}

	switch req.Method {
	case http.MethodPut, http.MethodPost:
		h.uploadReplay(rw, req, key)
	case http.MethodGet:
		h.downloadReplay(rw, key)
	default:
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) uploadReplay(rw http.ResponseWriter, req *http.Request, key string) {
	limit := h.cfg.MaxUploadBytes
	if limit <= 0 {
		limit = 1 << 30
	}
	reader := http.MaxBytesReader(rw, req.Body, limit)
	contentType := req.Header.Get("Content-Type")
	if err := h.replays.Upload(key, contentType, reader); err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusCreated, map[string]interface{}{"key": key})
}

func (h *Handler) downloadReplay(rw http.ResponseWriter, key string) {
	reader, contentType, err := h.replays.Download(key)
	if err != nil {
		writeError(rw, http.StatusNotFound, err.Error())
		return
	}
	defer reader.Close()
	if contentType != "" {
		rw.Header().Set("Content-Type", contentType)
	}
	rw.WriteHeader(http.StatusOK)
	if _, err := io.Copy(rw, reader); err != nil {
		logrus.Warnf("failed to stream replay object %s: %v", key, err)
	}
}

func (h *Handler) presignReplay(rw http.ResponseWriter, key string) {
	url, err := h.replays.Presign(key, h.cfg.ReplayPresignTTL)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusOK, map[string]interface{}{"key": key, "url": url})
}

func decodeCommandRecords(body []byte) ([]CommandRecord, error) {
	body = bytesTrimSpace(body)
	if len(body) == 0 {
		return nil, fmt.Errorf("empty body")
	}
	if body[0] == '[' {
		var commands []CommandRecord
		if err := json.Unmarshal(body, &commands); err != nil {
			return nil, err
		}
		return commands, nil
	}
	var command CommandRecord
	if err := json.Unmarshal(body, &command); err != nil {
		return nil, err
	}
	return []CommandRecord{command}, nil
}

func commandQueryFromRequest(req *http.Request) CommandQuery {
	q := req.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	return CommandQuery{
		Session: firstQuery(q.Get("sessionId"), q.Get("session")),
		User:    q.Get("user"),
		Asset:   q.Get("asset"),
		Account: q.Get("account"),
		Input:   q.Get("input"),
		From:    parseTimeQuery(q.Get("from")),
		To:      parseTimeQuery(q.Get("to")),
		Limit:   limit,
	}
}

func parseTimeQuery(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if unix, err := strconv.ParseInt(value, 10, 64); err == nil {
		return unix
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.Unix()
	}
	return 0
}

func firstQuery(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func readLimitedBody(rw http.ResponseWriter, req *http.Request, limit int64) ([]byte, error) {
	defer req.Body.Close()
	reader := http.MaxBytesReader(rw, req.Body, limit)
	return ioutil.ReadAll(reader)
}

func writeJSON(rw http.ResponseWriter, status int, value interface{}) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	if err := json.NewEncoder(rw).Encode(value); err != nil {
		logrus.Warnf("failed to write jumpserver audit response: %v", err)
	}
}

func writeError(rw http.ResponseWriter, status int, message string) {
	writeJSON(rw, status, map[string]interface{}{
		"error":   http.StatusText(status),
		"message": message,
	})
}

func bytesTrimSpace(body []byte) []byte {
	return []byte(strings.TrimSpace(string(body)))
}
