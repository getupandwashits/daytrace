package core

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type Journal struct{ root string }

func NewJournal(root string) *Journal { return &Journal{root: root} }
func (j *Journal) Root() string       { return j.root }

func (j *Journal) Append(sample ActivitySample) error {
	if err := os.MkdirAll(j.root, 0o700); err != nil {
		return err
	}
	return withFileLock(filepath.Join(j.root, ".journal.lock"), func() error {
		return appendRecord(filepath.Join(j.root, DayString(sample.Timestamp)+".jsonl"), sample, sampleTime)
	})
}

func (j *Journal) Files() ([]string, error) {
	entries, err := os.ReadDir(j.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type().IsRegular() && strings.HasSuffix(name, ".jsonl") {
			if _, err := time.Parse(time.DateOnly, dayOfLog(name)); err == nil {
				files = append(files, filepath.Join(j.root, name))
			}
		}
	}
	return files, nil
}

func dayOfLog(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}

func (j *Journal) SamplesOn(day time.Time) ([]ActivitySample, error) {
	path := filepath.Join(j.root, DayString(day)+".jsonl")
	samples, _, err := readLog[ActivitySample](path, logCursor{}, time.Time{}, sampleTime)
	sort.SliceStable(samples, func(a, b int) bool { return samples[a].Timestamp.Before(samples[b].Timestamp) })
	return samples, err
}

func sampleTime(sample ActivitySample) time.Time { return sample.Timestamp }

func (j *Journal) pending(delivered map[string]logCursor, cutoff time.Time) ([]ActivitySample, map[string]logCursor, error) {
	files, err := j.Files()
	if err != nil {
		return nil, nil, err
	}
	next := make(map[string]logCursor, len(files))
	var samples []ActivitySample
	for _, path := range files {
		day := dayOfLog(path)
		if day > DayString(cutoff) {
			if cursor := delivered[day]; cursor.Offset > 0 {
				next[day] = cursor
			}
			continue
		}
		records, cursor, err := readLog[ActivitySample](path, delivered[day], cutoff, sampleTime)
		if err != nil {
			return nil, nil, err
		}
		if cursor.Offset > 0 {
			next[day] = cursor
		}
		samples = append(samples, records...)
	}
	return samples, next, nil
}

func (j *Journal) pruneDelivered(delivered map[string]logCursor, now time.Time) error {
	expireBefore := DayString(now.In(Beijing).AddDate(0, 0, -7))
	for day, cursor := range delivered {
		if day >= expireBefore {
			continue
		}
		if _, err := time.Parse(time.DateOnly, day); err != nil {
			continue
		}
		err := withFileLock(filepath.Join(j.root, ".journal.lock"), func() error {
			path := filepath.Join(j.root, day+".jsonl")
			file, err := os.Open(path)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			defer file.Close()
			info, err := file.Stat()
			if err != nil || info.Size() != cursor.Offset {
				return err
			}
			matches, err := cursor.matches(file)
			if err != nil || !matches {
				return err
			}
			return os.Remove(path)
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

type SessionStore struct{ root string }

func NewSessionStore(root string) *SessionStore { return &SessionStore{root: root} }

func (s *SessionStore) path() string { return filepath.Join(s.root, "sessions.jsonl") }

func (s *SessionStore) Append(session FocusSession) error {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return err
	}
	return appendRecord(s.path(), session, sessionEnd)
}

func sessionEnd(session FocusSession) time.Time { return session.End }

func (s *SessionStore) pending(delivered logCursor, cutoff time.Time) ([]FocusSession, logCursor, error) {
	return readLog[FocusSession](s.path(), delivered, cutoff, sessionEnd)
}

func (s *SessionStore) LoadAll() ([]FocusSession, error) {
	sessions, _, err := s.pending(logCursor{}, time.Time{})
	sort.SliceStable(sessions, func(a, b int) bool { return sessions[a].Start.Before(sessions[b].Start) })
	return sessions, err
}

func SessionsOn(sessions []FocusSession, day time.Time) []FocusSession {
	start := StartOfDay(day)
	end := start.AddDate(0, 0, 1)
	var kept []FocusSession
	for _, session := range sessions {
		if session.Start.Before(end) && session.End.After(start) {
			kept = append(kept, session)
		}
	}
	return kept
}

func (s *SessionStore) SessionsOn(day time.Time) ([]FocusSession, error) {
	sessions, err := s.LoadAll()
	return SessionsOn(sessions, day), err
}

func appendRecord[T any](path string, value T, stamp func(T) time.Time) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	size, repair, err := logTail(file)
	if err != nil {
		return err
	}
	if repair {
		size, err = repairLogTail(file, size, stamp)
		if err != nil {
			return err
		}
	}
	if _, err = file.WriteAt(append(data, '\n'), size); err == nil {
		err = file.Sync()
	}
	if err == nil && size == 0 {
		err = syncDirectory(filepath.Dir(path))
	}
	if err != nil {
		return errors.Join(err, file.Truncate(size), file.Sync())
	}
	return nil
}

// logTail reports whether a nonempty log is missing its final line break.
func logTail(file *os.File) (size int64, repair bool, err error) {
	info, err := file.Stat()
	if err != nil || info.Size() == 0 {
		return 0, false, err
	}
	size = info.Size()
	var last [1]byte
	_, err = file.ReadAt(last[:], size-1)
	return size, last[0] != '\n', err
}

// repairLogTail runs with an exclusive file lock. A full record only needs
// its delimiter restored. A damaged fragment is backed up and synced before
// truncation, so an interruption cannot lose the bytes.
func repairLogTail[T any](file *os.File, size int64, stamp func(T) time.Time) (int64, error) {
	var block [4096]byte
	start := size
	for start > 0 {
		begin := max(0, start-int64(len(block)))
		chunk := block[:start-begin]
		if _, err := file.ReadAt(chunk, begin); err != nil {
			return 0, err
		}
		start = begin
		if newline := bytes.LastIndexByte(chunk, '\n'); newline >= 0 {
			start += int64(newline + 1)
			break
		}
	}
	tail, err := io.ReadAll(io.NewSectionReader(file, start, size-start))
	if err != nil {
		return 0, err
	}
	if _, err := decodeLogRecord(tail, stamp); err == nil {
		if _, err := file.WriteAt([]byte{'\n'}, size); err != nil {
			return 0, err
		}
		return size + 1, file.Sync()
	}

	digest := sha256.Sum256(tail)
	backup := fmt.Sprintf("%s.damaged-%d-%x", file.Name(), start, digest)
	if err := writeFileAtomic(backup, tail); err != nil {
		return 0, err
	}
	if err := syncDirectory(filepath.Dir(backup)); err != nil {
		return 0, err
	}
	if err := file.Truncate(start); err != nil {
		return 0, err
	}
	return start, file.Sync()
}

func decodeLogRecord[T any](line []byte, stamp func(T) time.Time) (T, error) {
	var record T
	if err := json.Unmarshal(line, &record); err != nil {
		return record, err
	}
	if stamp(record).IsZero() {
		return record, errors.New("缺少时间戳")
	}
	return record, nil
}

// Readers normally open the log read-only. If its last line was interrupted,
// release the shared lock before reopening for repair and check the tail
// again under the exclusive lock in case a writer already repaired it.
func openReadLog[T any](path string, stamp func(T) time.Time) (*os.File, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH); err != nil {
		_ = file.Close()
		return nil, err
	}
	_, repair, err := logTail(file)
	if err == nil && !repair {
		return file, nil
	}
	_ = file.Close()
	if err != nil {
		return nil, err
	}
	file, err = os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
	if err == nil {
		var size int64
		size, repair, err = logTail(file)
		if err == nil && repair {
			_, err = repairLogTail(file, size, stamp)
		}
	}
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("恢复日志 %s: %w", filepath.Base(path), err)
	}
	return file, nil
}

type logCursor struct {
	Offset    int64  `json:"offset"`
	LineBytes int64  `json:"lineBytes"`
	Anchor    string `json:"anchor"`
}

func (c logCursor) matches(file *os.File) (bool, error) {
	if c.Offset <= 0 || c.LineBytes <= 0 || c.LineBytes > c.Offset {
		return false, nil
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.NewSectionReader(file, c.Offset-c.LineBytes, c.LineBytes))
	if err != nil {
		return false, err
	}
	return n == c.LineBytes && hex.EncodeToString(hash.Sum(nil)) == c.Anchor, nil
}

// Each cursor covers a contiguous prefix of the append-only log. A record
// appended during an export remains beyond that prefix regardless of its timestamp.
func readLog[T any](path string, cursor logCursor, cutoff time.Time, stamp func(T) time.Time) ([]T, logCursor, error) {
	file, err := openReadLog(path, stamp)
	if file == nil {
		return nil, logCursor{}, err
	}
	defer file.Close()
	if cursor.Offset != 0 {
		matches, err := cursor.matches(file)
		if err != nil {
			return nil, logCursor{}, err
		}
		if !matches {
			cursor = logCursor{}
		}
	}
	if _, err := file.Seek(cursor.Offset, io.SeekStart); err != nil {
		return nil, logCursor{}, err
	}
	reader := bufio.NewReaderSize(file, 64*1024)
	var records []T
	var lastLine []byte
	done := func() ([]T, logCursor, error) {
		if lastLine != nil {
			cursor.LineBytes = int64(len(lastLine))
			sum := sha256.Sum256(lastLine)
			cursor.Anchor = hex.EncodeToString(sum[:])
		}
		return records, cursor, nil
	}
	for {
		line, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) && len(line) == 0 {
			return done()
		}
		if err != nil {
			return nil, logCursor{}, fmt.Errorf("%s:%d: %w", filepath.Base(path), cursor.Offset, err)
		}
		record, err := decodeLogRecord(line, stamp)
		if err != nil {
			return nil, logCursor{}, fmt.Errorf("%s:%d: %w", filepath.Base(path), cursor.Offset, err)
		}
		if !cutoff.IsZero() && stamp(record).After(cutoff) {
			return done()
		}
		cursor.Offset += int64(len(line))
		lastLine = line
		records = append(records, record)
	}
}

func withFileLock(path string, action func() error) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	return action()
}
