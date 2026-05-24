package scanner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"strm/internal/db"
	"strm/internal/models"
	"strm/internal/openlist"
)

type Service struct {
	Store      *db.Store
	HTTPClient *http.Client
}

type downloadJob struct {
	remotePath    string
	localPath     string
	url           string
	size          int64
	remoteTime    time.Time
	hasRemoteTime bool
}

func (s *Service) Run(ctx context.Context, runID int64, task models.Task) (models.RunStats, error) {
	var stats models.RunStats
	task, err := NormalizeTask(task)
	if err != nil {
		return stats, err
	}
	settings, err := s.Store.GetOpenListSettings(ctx)
	if err != nil {
		return stats, err
	}
	if settings.BaseURL == "" || settings.Username == "" || settings.PasswordHash == "" {
		return stats, errors.New("openlist settings are incomplete")
	}

	client := openlist.NewClient(settings.BaseURL, s.HTTPClient)
	if err := s.event(ctx, runID, "info", "logging in to OpenList"); err != nil {
		return stats, err
	}
	if err := client.LoginHash(ctx, settings.Username, settings.PasswordHash); err != nil {
		return stats, err
	}

	seen := map[string]struct{}{}
	var jobs []downloadJob
	for _, scanDir := range task.ScanDirs {
		if err := s.event(ctx, runID, "info", "scanning "+scanDir); err != nil {
			return stats, err
		}
		if err := s.walk(ctx, client, runID, task, scanDir, &stats, seen, &jobs); err != nil {
			return stats, err
		}
	}

	if err := s.runDownloads(ctx, task, jobs, &stats); err != nil {
		stats.Errors++
		_ = s.event(context.Background(), runID, "error", err.Error())
		return stats, err
	}

	if task.SyncMode == models.SyncModeStrict {
		if err := s.strictSync(ctx, runID, task, seen, &stats); err != nil {
			stats.Errors++
			return stats, err
		}
	}
	_ = s.event(ctx, runID, "info", "scan finished")
	return stats, nil
}

func (s *Service) walk(ctx context.Context, client *openlist.Client, runID int64, task models.Task, dir string, stats *models.RunStats, seen map[string]struct{}, jobs *[]downloadJob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if MatchBlacklist(task.Blacklist, dir) {
		stats.Skipped++
		return nil
	}
	entries, err := client.List(ctx, dir)
	if err != nil {
		stats.Errors++
		_ = s.event(context.Background(), runID, "error", err.Error())
		return err
	}
	stats.Dirs++
	for _, entry := range entries {
		remotePath := openlist.JoinPath(dir, entry.Name)
		if MatchBlacklist(task.Blacklist, remotePath) {
			stats.Skipped++
			continue
		}
		if entry.IsDir {
			if err := s.walk(ctx, client, runID, task, remotePath, stats, seen, jobs); err != nil {
				return err
			}
			continue
		}
		stats.Files++
		switch {
		case HasConfiguredSuffix(entry.Name, task.DownloadExtensions):
			if err := s.prepareDownload(ctx, client, task, remotePath, entry, stats, seen, jobs); err != nil {
				stats.Errors++
				_ = s.event(context.Background(), runID, "error", err.Error())
				return err
			}
		case HasConfiguredSuffix(entry.Name, task.StrmExtensions):
			if err := s.writeSTRM(ctx, client, task, remotePath, entry, stats, seen); err != nil {
				stats.Errors++
				_ = s.event(context.Background(), runID, "error", err.Error())
				return err
			}
		default:
			stats.Skipped++
		}
	}
	return nil
}

func (s *Service) writeSTRM(ctx context.Context, client *openlist.Client, task models.Task, remotePath string, entry openlist.Entry, stats *models.RunStats, seen map[string]struct{}) error {
	localPath, err := OutputPath(task.OutputRoot, remotePath, true)
	if err != nil {
		return err
	}
	seen[filepath.Clean(localPath)] = struct{}{}
	sign, err := s.resolveSign(ctx, client, remotePath, entry)
	if err != nil {
		return err
	}
	downloadURL, err := openlist.BuildDURL(s.mustBaseURL(ctx), remotePath, sign, task.EncodeURL)
	if err != nil {
		return err
	}
	content := []byte(downloadURL + "\n")
	old, err := os.ReadFile(localPath)
	if err == nil && string(old) == string(content) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return err
	}
	tmp := localPath + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, localPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	stats.StrmWritten++
	return nil
}

func (s *Service) prepareDownload(ctx context.Context, client *openlist.Client, task models.Task, remotePath string, entry openlist.Entry, stats *models.RunStats, seen map[string]struct{}, jobs *[]downloadJob) error {
	localPath, err := OutputPath(task.OutputRoot, remotePath, false)
	if err != nil {
		return err
	}
	seen[filepath.Clean(localPath)] = struct{}{}
	remoteTime, hasRemoteTime := LatestEntryTime(entry)
	if !NeedsDownload(localPath, entry.Size, remoteTime, hasRemoteTime) {
		return nil
	}
	sign, err := s.resolveSign(ctx, client, remotePath, entry)
	if err != nil {
		return err
	}
	downloadURL, err := openlist.BuildDURL(s.mustBaseURL(ctx), remotePath, sign, task.EncodeURL)
	if err != nil {
		return err
	}
	*jobs = append(*jobs, downloadJob{
		remotePath:    remotePath,
		localPath:     localPath,
		url:           downloadURL,
		size:          entry.Size,
		remoteTime:    remoteTime,
		hasRemoteTime: hasRemoteTime,
	})
	return nil
}

func (s *Service) resolveSign(ctx context.Context, client *openlist.Client, remotePath string, entry openlist.Entry) (string, error) {
	if entry.Sign != "" {
		return entry.Sign, nil
	}
	full, err := client.Get(ctx, remotePath)
	if err != nil {
		return "", err
	}
	return full.Sign, nil
}

func (s *Service) mustBaseURL(ctx context.Context) string {
	settings, err := s.Store.GetOpenListSettings(ctx)
	if err != nil {
		return ""
	}
	return settings.BaseURL
}

func (s *Service) runDownloads(ctx context.Context, task models.Task, jobs []downloadJob, stats *models.RunStats) error {
	if len(jobs) == 0 {
		return nil
	}
	workers := task.DownloadConcurrency
	if workers <= 0 {
		workers = 1
	}
	queue := make(chan downloadJob)
	errCh := make(chan error, len(jobs))
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range queue {
				if err := s.downloadOne(ctx, task, job); err != nil {
					errCh <- err
					continue
				}
				mu.Lock()
				stats.Downloads++
				mu.Unlock()
			}
		}()
	}
	for _, job := range jobs {
		select {
		case <-ctx.Done():
			close(queue)
			wg.Wait()
			return ctx.Err()
		case queue <- job:
		}
	}
	close(queue)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) downloadOne(ctx context.Context, task models.Task, job downloadJob) error {
	timeout := time.Duration(task.DownloadTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	jobCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(jobCtx, http.MethodGet, job.url, nil)
	if err != nil {
		return err
	}
	httpClient := s.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("download %s failed with http %d", job.remotePath, res.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(job.localPath), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp-%d", job.localPath, time.Now().UnixNano())
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, res.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, job.localPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if job.hasRemoteTime {
		_ = os.Chtimes(job.localPath, job.remoteTime, job.remoteTime)
	}
	return nil
}

func (s *Service) strictSync(ctx context.Context, runID int64, task models.Task, seen map[string]struct{}, stats *models.RunStats) error {
	for _, scanDir := range task.ScanDirs {
		localRoot, err := OutputPath(task.OutputRoot, scanDir, false)
		if err != nil {
			return err
		}
		if _, err := os.Stat(localRoot); os.IsNotExist(err) {
			continue
		}
		var dirs []string
		err = filepath.WalkDir(localRoot, func(localPath string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			remotePath := localRemotePath(scanDir, localRoot, localPath, !d.IsDir())
			if MatchBlacklist(task.Blacklist, remotePath) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				if localPath != localRoot {
					dirs = append(dirs, localPath)
				}
				return nil
			}
			if _, ok := seen[filepath.Clean(localPath)]; ok {
				return nil
			}
			if err := os.Remove(localPath); err != nil {
				return err
			}
			stats.Deleted++
			_ = s.event(context.Background(), runID, "info", "deleted "+localPath)
			return nil
		})
		if err != nil {
			return err
		}
		sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
		for _, dir := range dirs {
			_ = os.Remove(dir)
		}
	}
	return nil
}

func (s *Service) event(ctx context.Context, runID int64, level, message string) error {
	if s.Store == nil || runID == 0 {
		return nil
	}
	return s.Store.AddRunEvent(ctx, runID, level, message)
}
