package terminalaudit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type fileCommandStore struct {
	path string
	mu   sync.Mutex
}

func NewFileCommandStore(path string) CommandStore {
	return &fileCommandStore{path: path}
}

func (s *fileCommandStore) Save(commands []CommandRecord) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, command := range commands {
		if command.Timestamp == 0 {
			command.Timestamp = time.Now().Unix()
		}
		if err := enc.Encode(command); err != nil {
			return err
		}
	}
	return nil
}

func (s *fileCommandStore) Query(query CommandQuery) ([]CommandRecord, error) {
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return []CommandRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if query.Limit <= 0 {
		query.Limit = 100
	}

	var result []CommandRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var command CommandRecord
		if err := json.Unmarshal(scanner.Bytes(), &command); err != nil {
			continue
		}
		if !commandMatches(command, query) {
			continue
		}
		result = append(result, command)
		if len(result) >= query.Limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *fileCommandStore) Ping() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	return f.Close()
}

func (s *fileCommandStore) Type() string {
	return commandStorageFile
}

func commandMatches(command CommandRecord, query CommandQuery) bool {
	if query.Session != "" && command.Session != query.Session {
		return false
	}
	if query.User != "" && command.User != query.User {
		return false
	}
	if query.Asset != "" && command.Asset != query.Asset {
		return false
	}
	if query.Account != "" && command.Account != query.Account {
		return false
	}
	if query.Input != "" && !strings.Contains(command.Input, query.Input) {
		return false
	}
	if query.From > 0 && command.Timestamp < query.From {
		return false
	}
	if query.To > 0 && command.Timestamp > query.To {
		return false
	}
	return true
}
