package terminalaudit

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

type esCommandStore struct {
	cfg    ESConfig
	client *http.Client
}

func NewESCommandStore(cfg ESConfig) (CommandStore, error) {
	if cfg.URL == "" {
		return nil, errors.New("elasticsearch url is required")
	}
	if cfg.Index == "" {
		cfg.Index = "terminal-command"
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // nolint:gosec
	}

	return &esCommandStore{
		cfg:    cfg,
		client: &http.Client{Transport: transport, Timeout: 30 * time.Second},
	}, nil
}

func (s *esCommandStore) Save(commands []CommandRecord) error {
	if len(commands) == 0 {
		return nil
	}

	groups := map[string][]CommandRecord{}
	for _, command := range commands {
		if command.Timestamp == 0 {
			command.Timestamp = time.Now().Unix()
		}
		index := s.indexFor(command.Timestamp)
		groups[index] = append(groups[index], command)
	}

	for index, records := range groups {
		var body bytes.Buffer
		enc := json.NewEncoder(&body)
		for _, command := range records {
			if err := enc.Encode(map[string]interface{}{"index": map[string]interface{}{}}); err != nil {
				return err
			}
			if err := enc.Encode(commandToESDocument(command)); err != nil {
				return err
			}
		}
		if err := s.do(http.MethodPost, "/"+index+"/_bulk", &body, nil); err != nil {
			return err
		}
	}
	return nil
}

func (s *esCommandStore) Query(query CommandQuery) ([]CommandRecord, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	if query.Limit > 1000 {
		query.Limit = 1000
	}

	body, err := json.Marshal(esSearchBody(query))
	if err != nil {
		return nil, err
	}

	var response esSearchResponse
	if err := s.do(http.MethodPost, "/"+s.searchIndex()+"/_search", bytes.NewReader(body), &response); err != nil {
		return nil, err
	}

	result := make([]CommandRecord, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		result = append(result, hit.Source.toCommandRecord())
	}
	return result, nil
}

func (s *esCommandStore) Ping() error {
	return s.do(http.MethodGet, "/", nil, nil)
}

func (s *esCommandStore) Type() string {
	return commandStorageES
}

func (s *esCommandStore) indexFor(timestamp int64) string {
	if !s.cfg.IndexByDate {
		return s.cfg.Index
	}
	return fmt.Sprintf("%s-%s", s.cfg.Index, time.Unix(timestamp, 0).UTC().Format("2006-01-02"))
}

func (s *esCommandStore) searchIndex() string {
	if s.cfg.IndexByDate {
		return s.cfg.Index + "-*"
	}
	return s.cfg.Index
}

func (s *esCommandStore) do(method, path string, body io.Reader, out interface{}) error {
	req, err := http.NewRequest(method, s.cfg.URL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.cfg.Username != "" || s.cfg.Password != "" {
		req.SetBasicAuth(s.cfg.Username, s.cfg.Password)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("elasticsearch request failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	if out != nil && len(respBody) > 0 {
		return json.Unmarshal(respBody, out)
	}
	return nil
}

func commandToESDocument(command CommandRecord) map[string]interface{} {
	return map[string]interface{}{
		"id":         command.ID,
		"user":       command.User,
		"asset":      command.Asset,
		"account":    command.Account,
		"session":    command.Session,
		"input":      command.Input,
		"output":     command.Output,
		"riskLevel":  command.RiskLevel,
		"orgId":      command.OrgID,
		"remoteAddr": command.RemoteAddr,
		"timestamp":  command.Timestamp,
		"@timestamp": time.Unix(command.Timestamp, 0).UTC().Format(time.RFC3339),
	}
}

func esSearchBody(query CommandQuery) map[string]interface{} {
	filter := make([]map[string]interface{}, 0, 8)
	must := make([]map[string]interface{}, 0, 1)

	addTerm := func(field, value string) {
		if value == "" {
			return
		}
		filter = append(filter, map[string]interface{}{
			"term": map[string]interface{}{field: value},
		})
	}
	addTerm("session", query.Session)
	addTerm("user", query.User)
	addTerm("asset", query.Asset)
	addTerm("account", query.Account)

	if query.Input != "" {
		must = append(must, map[string]interface{}{
			"match": map[string]interface{}{"input": query.Input},
		})
	}
	if query.From > 0 || query.To > 0 {
		rng := map[string]interface{}{}
		if query.From > 0 {
			rng["gte"] = query.From
		}
		if query.To > 0 {
			rng["lte"] = query.To
		}
		filter = append(filter, map[string]interface{}{
			"range": map[string]interface{}{"timestamp": rng},
		})
	}

	boolQuery := map[string]interface{}{}
	if len(filter) > 0 {
		boolQuery["filter"] = filter
	}
	if len(must) > 0 {
		boolQuery["must"] = must
	}
	if len(boolQuery) == 0 {
		boolQuery["must"] = []map[string]interface{}{{"match_all": map[string]interface{}{}}}
	}

	return map[string]interface{}{
		"size": query.Limit,
		"sort": []map[string]interface{}{
			{"timestamp": map[string]interface{}{"order": "desc"}},
		},
		"query": map[string]interface{}{"bool": boolQuery},
	}
}

type esSearchResponse struct {
	Hits struct {
		Hits []struct {
			Source esCommandDocument `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

type esCommandDocument struct {
	ID         string `json:"id"`
	User       string `json:"user"`
	Asset      string `json:"asset"`
	Account    string `json:"account"`
	Session    string `json:"session"`
	Input      string `json:"input"`
	Output     string `json:"output"`
	RiskLevel  int    `json:"riskLevel"`
	OrgID      string `json:"orgId"`
	RemoteAddr string `json:"remoteAddr"`
	Timestamp  int64  `json:"timestamp"`
}

func (d esCommandDocument) toCommandRecord() CommandRecord {
	return CommandRecord{
		ID:         d.ID,
		User:       d.User,
		Asset:      d.Asset,
		Account:    d.Account,
		Session:    d.Session,
		Input:      d.Input,
		Output:     d.Output,
		RiskLevel:  d.RiskLevel,
		OrgID:      d.OrgID,
		RemoteAddr: d.RemoteAddr,
		Timestamp:  d.Timestamp,
	}
}

func normalizeESURL(url string) string {
	return strings.TrimRight(strings.TrimSpace(url), "/")
}
