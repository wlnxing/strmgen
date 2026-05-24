package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"

	"strm/internal/models"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.pingAndPragma(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) pingAndPragma(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`)
	return err
}

func (s *Store) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS openlist_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			base_url TEXT NOT NULL DEFAULT '',
			username TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			cron TEXT NOT NULL,
			output_root TEXT NOT NULL,
			scan_dirs TEXT NOT NULL,
			sync_mode TEXT NOT NULL,
			strm_extensions TEXT NOT NULL,
			download_extensions TEXT NOT NULL,
			blacklist TEXT NOT NULL,
			encode_url INTEGER NOT NULL DEFAULT 1,
			download_concurrency INTEGER NOT NULL DEFAULT 2,
			download_timeout_seconds INTEGER NOT NULL DEFAULT 120,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id INTEGER NOT NULL,
			trigger TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			ended_at TEXT,
			stats TEXT NOT NULL,
			error TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS run_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL,
			level TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(run_id) REFERENCES runs(id) ON DELETE CASCADE
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO openlist_settings (id, updated_at) VALUES (1, ?)`, nowString())
	return err
}

func (s *Store) UpsertAdmin(ctx context.Context, username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	now := nowString()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(username) DO UPDATE SET password_hash = excluded.password_hash, updated_at = excluded.updated_at
	`, username, string(hash), now, now)
	return err
}

type User struct {
	ID           int64
	Username     string
	PasswordHash string
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT id, username, password_hash FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) GetOpenListSettings(ctx context.Context) (models.OpenListSettings, error) {
	var st models.OpenListSettings
	err := s.db.QueryRowContext(ctx, `SELECT base_url, username, password_hash FROM openlist_settings WHERE id = 1`).
		Scan(&st.BaseURL, &st.Username, &st.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	st.PasswordSet = st.PasswordHash != ""
	return st, err
}

func (s *Store) SaveOpenListSettings(ctx context.Context, st models.OpenListSettings) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO openlist_settings (id, base_url, username, password_hash, updated_at)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			base_url = excluded.base_url,
			username = excluded.username,
			password_hash = excluded.password_hash,
			updated_at = excluded.updated_at
	`, st.BaseURL, st.Username, st.PasswordHash, nowString())
	return err
}

func (s *Store) CreateTask(ctx context.Context, task models.Task) (models.Task, error) {
	now := time.Now().UTC()
	task.CreatedAt = now
	task.UpdatedAt = now
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (
			name, enabled, cron, output_root, scan_dirs, sync_mode, strm_extensions,
			download_extensions, blacklist, encode_url, download_concurrency,
			download_timeout_seconds, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.Name, boolInt(task.Enabled), task.Cron, task.OutputRoot, encodeJSON(task.ScanDirs), task.SyncMode,
		encodeJSON(task.StrmExtensions), encodeJSON(task.DownloadExtensions), encodeJSON(task.Blacklist),
		boolInt(task.EncodeURL), task.DownloadConcurrency, task.DownloadTimeoutSeconds,
		formatTime(task.CreatedAt), formatTime(task.UpdatedAt))
	if err != nil {
		return models.Task{}, err
	}
	task.ID, err = res.LastInsertId()
	return task, err
}

func (s *Store) UpdateTask(ctx context.Context, task models.Task) (models.Task, error) {
	task.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET
			name = ?, enabled = ?, cron = ?, output_root = ?, scan_dirs = ?,
			sync_mode = ?, strm_extensions = ?, download_extensions = ?,
			blacklist = ?, encode_url = ?, download_concurrency = ?,
			download_timeout_seconds = ?, updated_at = ?
		WHERE id = ?
	`, task.Name, boolInt(task.Enabled), task.Cron, task.OutputRoot, encodeJSON(task.ScanDirs),
		task.SyncMode, encodeJSON(task.StrmExtensions), encodeJSON(task.DownloadExtensions),
		encodeJSON(task.Blacklist), boolInt(task.EncodeURL), task.DownloadConcurrency,
		task.DownloadTimeoutSeconds, formatTime(task.UpdatedAt), task.ID)
	if err != nil {
		return models.Task{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return models.Task{}, err
	}
	if n == 0 {
		return models.Task{}, ErrNotFound
	}
	return s.GetTask(ctx, task.ID)
}

func (s *Store) DeleteTask(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetTask(ctx context.Context, id int64) (models.Task, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, enabled, cron, output_root, scan_dirs, sync_mode,
			strm_extensions, download_extensions, blacklist, encode_url,
			download_concurrency, download_timeout_seconds, created_at, updated_at
		FROM tasks WHERE id = ?
	`, id)
	task, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Task{}, ErrNotFound
	}
	return task, err
}

func (s *Store) ListTasks(ctx context.Context) ([]models.Task, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, enabled, cron, output_root, scan_dirs, sync_mode,
			strm_extensions, download_extensions, blacklist, encode_url,
			download_concurrency, download_timeout_seconds, created_at, updated_at
		FROM tasks ORDER BY id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := make([]models.Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *Store) CreateRun(ctx context.Context, taskID int64, trigger string) (models.Run, error) {
	run := models.Run{
		TaskID:    taskID,
		Trigger:   trigger,
		Status:    models.RunStatusRunning,
		Stats:     models.RunStats{},
		StartedAt: time.Now().UTC(),
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO runs (task_id, trigger, status, started_at, stats, error)
		VALUES (?, ?, ?, ?, ?, '')
	`, run.TaskID, run.Trigger, run.Status, formatTime(run.StartedAt), encodeJSON(run.Stats))
	if err != nil {
		return models.Run{}, err
	}
	run.ID, err = res.LastInsertId()
	return run, err
}

func (s *Store) FinishRun(ctx context.Context, runID int64, status string, stats models.RunStats, runErr string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE runs SET status = ?, ended_at = ?, stats = ?, error = ? WHERE id = ?
	`, status, formatTime(now), encodeJSON(stats), runErr, runID)
	return err
}

func (s *Store) AddRunEvent(ctx context.Context, runID int64, level, message string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO run_events (run_id, level, message, created_at) VALUES (?, ?, ?, ?)
	`, runID, level, message, nowString())
	return err
}

func (s *Store) ListRuns(ctx context.Context, limit int) ([]models.Run, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, r.task_id, COALESCE(t.name, ''), r.trigger, r.status, r.started_at, r.ended_at, r.stats, r.error
		FROM runs r
		LEFT JOIN tasks t ON t.id = r.task_id
		ORDER BY r.id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]models.Run, 0)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *Store) GetRun(ctx context.Context, id int64) (models.Run, []models.RunEvent, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT r.id, r.task_id, COALESCE(t.name, ''), r.trigger, r.status, r.started_at, r.ended_at, r.stats, r.error
		FROM runs r
		LEFT JOIN tasks t ON t.id = r.task_id
		WHERE r.id = ?
	`, id)
	run, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Run{}, nil, ErrNotFound
	}
	if err != nil {
		return models.Run{}, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, level, message, created_at FROM run_events WHERE run_id = ? ORDER BY id ASC
	`, id)
	if err != nil {
		return models.Run{}, nil, err
	}
	defer rows.Close()
	events := make([]models.RunEvent, 0)
	for rows.Next() {
		var ev models.RunEvent
		var created string
		if err := rows.Scan(&ev.ID, &ev.RunID, &ev.Level, &ev.Message, &created); err != nil {
			return models.Run{}, nil, err
		}
		ev.CreatedAt, _ = parseTime(created)
		events = append(events, ev)
	}
	return run, events, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTask(row rowScanner) (models.Task, error) {
	var task models.Task
	var enabled, encodeURL int
	var scanDirs, strmExts, downloadExts, blacklist string
	var created, updated string
	err := row.Scan(&task.ID, &task.Name, &enabled, &task.Cron, &task.OutputRoot, &scanDirs,
		&task.SyncMode, &strmExts, &downloadExts, &blacklist, &encodeURL,
		&task.DownloadConcurrency, &task.DownloadTimeoutSeconds, &created, &updated)
	if err != nil {
		return models.Task{}, err
	}
	task.Enabled = enabled != 0
	task.EncodeURL = encodeURL != 0
	task.ScanDirs = decodeStringSlice(scanDirs)
	task.StrmExtensions = decodeStringSlice(strmExts)
	task.DownloadExtensions = decodeStringSlice(downloadExts)
	task.Blacklist = decodeStringSlice(blacklist)
	task.CreatedAt, _ = parseTime(created)
	task.UpdatedAt, _ = parseTime(updated)
	return task, nil
}

func scanRun(row rowScanner) (models.Run, error) {
	var run models.Run
	var started string
	var ended sql.NullString
	var statsJSON string
	err := row.Scan(&run.ID, &run.TaskID, &run.TaskName, &run.Trigger, &run.Status, &started, &ended, &statsJSON, &run.Error)
	if err != nil {
		return models.Run{}, err
	}
	run.StartedAt, _ = parseTime(started)
	if ended.Valid && ended.String != "" {
		t, _ := parseTime(ended.String)
		run.EndedAt = &t
	}
	_ = json.Unmarshal([]byte(statsJSON), &run.Stats)
	return run, nil
}

func encodeJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

func decodeStringSlice(raw string) []string {
	if raw == "" {
		return nil
	}
	var v []string
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil
	}
	return v
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nowString() string {
	return formatTime(time.Now().UTC())
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("parse time %q: %w", s, err)
}
