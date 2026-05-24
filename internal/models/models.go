package models

import "time"

const (
	SyncModeStrict = "strict"
	SyncModeLoose  = "loose"

	RunStatusRunning  = "running"
	RunStatusSuccess  = "success"
	RunStatusFailed   = "failed"
	RunStatusCanceled = "canceled"
)

type OpenListSettings struct {
	BaseURL         string `json:"base_url"`
	DownloadBaseURL string `json:"download_base_url"`
	Username        string `json:"username"`
	PasswordHash    string `json:"-"`
	PasswordSet     bool   `json:"password_set"`
}

type Task struct {
	ID                     int64     `json:"id"`
	Name                   string    `json:"name"`
	Enabled                bool      `json:"enabled"`
	Cron                   string    `json:"cron"`
	OutputRoot             string    `json:"output_root"`
	ScanDirs               []string  `json:"scan_dirs"`
	SyncMode               string    `json:"sync_mode"`
	StrmExtensions         []string  `json:"strm_extensions"`
	DownloadExtensions     []string  `json:"download_extensions"`
	Blacklist              []string  `json:"blacklist"`
	EncodeURL              bool      `json:"encode_url"`
	DownloadConcurrency    int       `json:"download_concurrency"`
	DownloadTimeoutSeconds int       `json:"download_timeout_seconds"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type RunStats struct {
	Dirs        int64 `json:"dirs"`
	Files       int64 `json:"files"`
	StrmWritten int64 `json:"strm_written"`
	Downloads   int64 `json:"downloads"`
	Deleted     int64 `json:"deleted"`
	Skipped     int64 `json:"skipped"`
	Errors      int64 `json:"errors"`
}

type Run struct {
	ID        int64      `json:"id"`
	TaskID    int64      `json:"task_id"`
	TaskName  string     `json:"task_name,omitempty"`
	Trigger   string     `json:"trigger"`
	Status    string     `json:"status"`
	Stats     RunStats   `json:"stats"`
	Error     string     `json:"error,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

type RunEvent struct {
	ID        int64     `json:"id"`
	RunID     int64     `json:"run_id"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type ActiveRun struct {
	RunID     int64     `json:"run_id"`
	TaskID    int64     `json:"task_id"`
	TaskName  string    `json:"task_name"`
	Trigger   string    `json:"trigger"`
	StartedAt time.Time `json:"started_at"`
}
