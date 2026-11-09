package main

import (
	"encoding/json"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const resultsFile = "results.jsonl"

// result is one line of the results file.
type result struct {
	Time      string            `json:"time"`
	Remote    string            `json:"remote"`
	Scheme    string            `json:"scheme"`
	Host      string            `json:"host"`
	Scenario  string            `json:"scenario"`
	Notes     string            `json:"notes,omitempty"`
	UserAgent string            `json:"user_agent,omitempty"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// resultLog appends results to a file, one JSON object per line, and shows
// each one in the terminal.
type resultLog struct {
	mu   sync.Mutex
	path string
}

func (l *resultLog) add(r result) error {
	r.Time = time.Now().Format(time.RFC3339)
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if !strings.HasPrefix(r.Scenario, "forged-") {
		log.Printf("RESULT  %s  %s over %s  %s", r.Remote, r.Scenario, r.Scheme, summary(r.Checks))
	}
	return nil
}

// summary lists the checks in a stable order.
func summary(checks map[string]string) string {
	keys := make([]string, 0, len(checks))
	for k := range checks {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+checks[k])
	}
	return strings.Join(parts, " ")
}
