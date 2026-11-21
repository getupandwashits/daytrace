package core

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Journal struct {
	mu   sync.Mutex
	root string
}

func NewJournal(root string) *Journal {
	return &Journal{root: root}
}

func (j *Journal) Root() string { return j.root }

func (j *Journal) Append(sample ActivitySample) error {
	if err := os.MkdirAll(j.root, 0o700); err != nil {
		return err
	}
	path := filepath.Join(j.root, DayString(sample.Timestamp)+".jsonl")
	data, err := json.Marshal(sample)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	j.mu.Lock()
	defer j.mu.Unlock()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	if _, err := file.Write(data); err != nil {
		return err
	}
	return file.Sync()
}

func (j *Journal) Files() ([]string, error) {
	entries, err := os.ReadDir(j.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		files = append(files, filepath.Join(j.root, entry.Name()))
	}
	sort.Strings(files)
	return files, nil
}

func (j *Journal) ReadRange(since, until *time.Time) ([]ActivitySample, error) {
	files, err := j.Files()
	if err != nil {
		return nil, err
	}
	var samples []ActivitySample
	for _, path := range files {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		for scanner.Scan() {
			var sample ActivitySample
			if json.Unmarshal(scanner.Bytes(), &sample) != nil {
				continue
			}
			if since != nil && sample.Timestamp.Before(*since) {
				continue
			}
			if until != nil && sample.Timestamp.After(*until) {
				continue
			}
			samples = append(samples, sample)
		}
		scanErr := scanner.Err()
		_ = file.Close()
		if scanErr != nil {
			return nil, scanErr
		}
	}
	sort.Slice(samples, func(a, b int) bool { return samples[a].Timestamp.Before(samples[b].Timestamp) })
	return samples, nil
}

func (j *Journal) SamplesOn(day time.Time) ([]ActivitySample, error) {
	start := StartOfDay(day)
	end := start.AddDate(0, 0, 1).Add(-time.Millisecond)
	return j.ReadRange(&start, &end)
}

func (j *Journal) Pending(since *time.Time, until time.Time) ([]ActivitySample, error) {
	samples, err := j.ReadRange(since, &until)
	if err != nil {
		return nil, err
	}
	if since == nil {
		return samples, nil
	}
	kept := samples[:0]
	for _, sample := range samples {
		if sample.Timestamp.After(*since) {
			kept = append(kept, sample)
		}
	}
	return kept, nil
}

type SessionStore struct {
	mu   sync.Mutex
	root string
}

func NewSessionStore(root string) *SessionStore {
	return &SessionStore{root: root}
}

func (s *SessionStore) path() string {
	return filepath.Join(s.root, "sessions.jsonl")
}

func (s *SessionStore) Append(session FocusSession) error {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.OpenFile(s.path(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	if _, err := file.Write(data); err != nil {
		return err
	}
	return file.Sync()
}

func (s *SessionStore) LoadAll() ([]FocusSession, error) {
	file, err := os.Open(s.path())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()
	var sessions []FocusSession
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var session FocusSession
		if json.Unmarshal(scanner.Bytes(), &session) == nil {
			sessions = append(sessions, session)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.Slice(sessions, func(a, b int) bool { return sessions[a].Start.Before(sessions[b].Start) })
	return sessions, nil
}

func (s *SessionStore) SessionsOn(day time.Time) ([]FocusSession, error) {
	start := StartOfDay(day)
	end := start.AddDate(0, 0, 1)
	sessions, err := s.LoadAll()
	if err != nil {
		return nil, err
	}
	var kept []FocusSession
	for _, session := range sessions {
		if session.Start.Before(end) && session.End.After(start) {
			kept = append(kept, session)
		}
	}
	return kept, nil
}

func SanitizeFilename(value string) string {
	replacer := strings.NewReplacer("/", "-", ":", "-", "\x00", "")
	return replacer.Replace(value)
}
