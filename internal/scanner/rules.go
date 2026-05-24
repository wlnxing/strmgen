package scanner

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"strm/internal/models"
	"strm/internal/openlist"
)

var errUnsafeOutput = errors.New("unsafe output path")

func DefaultTask() models.Task {
	return models.Task{
		Name:                   "默认任务",
		Enabled:                true,
		Cron:                   "0 3 * * *",
		OutputRoot:             "/media",
		ScanDirs:               []string{"/"},
		SyncMode:               models.SyncModeLoose,
		StrmExtensions:         []string{".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".m4v", ".ts", ".m2ts", ".webm", ".iso"},
		DownloadExtensions:     []string{".srt", ".ass", ".ssa", ".sub", ".idx", ".nfo", ".jpg", ".jpeg", ".png"},
		Blacklist:              []string{},
		EncodeURL:              true,
		DownloadConcurrency:    2,
		DownloadTimeoutSeconds: 120,
	}
}

func NormalizeTask(task models.Task) (models.Task, error) {
	task.Name = strings.TrimSpace(task.Name)
	if task.Name == "" {
		return task, errors.New("task name is required")
	}
	task.Cron = strings.TrimSpace(task.Cron)
	if _, err := cron.ParseStandard(task.Cron); err != nil {
		return task, fmt.Errorf("invalid cron: %w", err)
	}
	root, err := ValidateOutputRoot(task.OutputRoot)
	if err != nil {
		return task, err
	}
	task.OutputRoot = root
	if len(task.ScanDirs) == 0 {
		task.ScanDirs = []string{"/"}
	}
	for i := range task.ScanDirs {
		task.ScanDirs[i] = openlist.NormalizePath(task.ScanDirs[i])
	}
	task.SyncMode = strings.TrimSpace(task.SyncMode)
	if task.SyncMode == "" {
		task.SyncMode = models.SyncModeLoose
	}
	if task.SyncMode != models.SyncModeLoose && task.SyncMode != models.SyncModeStrict {
		return task, errors.New("sync_mode must be loose or strict")
	}
	task.StrmExtensions = NormalizeExtensions(task.StrmExtensions)
	task.DownloadExtensions = NormalizeExtensions(task.DownloadExtensions)
	task.Blacklist = normalizeList(task.Blacklist)
	if task.DownloadConcurrency <= 0 {
		task.DownloadConcurrency = 1
	}
	if task.DownloadConcurrency > 16 {
		task.DownloadConcurrency = 16
	}
	if task.DownloadTimeoutSeconds <= 0 {
		task.DownloadTimeoutSeconds = 120
	}
	return task, nil
}

func ValidateOutputRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("output root is required")
	}
	if !filepath.IsAbs(root) {
		return "", errors.New("output root must be absolute")
	}
	clean := filepath.Clean(root)
	if clean == string(filepath.Separator) {
		return "", errors.New("output root cannot be /")
	}
	return clean, nil
}

func NormalizeExtensions(exts []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, ext := range exts {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		if _, ok := seen[ext]; ok {
			continue
		}
		seen[ext] = struct{}{}
		out = append(out, ext)
	}
	sort.Strings(out)
	return out
}

func HasConfiguredSuffix(name string, exts []string) bool {
	lower := strings.ToLower(name)
	for _, ext := range exts {
		if strings.HasSuffix(lower, strings.ToLower(ext)) {
			return true
		}
	}
	return false
}

func OutputPath(root, remotePath string, strm bool) (string, error) {
	root, err := ValidateOutputRoot(root)
	if err != nil {
		return "", err
	}
	cleanRemote := openlist.NormalizePath(remotePath)
	rel := strings.TrimPrefix(cleanRemote, "/")
	local := root
	if rel != "" {
		local = filepath.Join(root, filepath.FromSlash(rel))
	}
	if strm {
		local += ".strm"
	}
	if !within(root, local) {
		return "", errUnsafeOutput
	}
	return local, nil
}

func NeedsDownload(localPath string, remoteSize int64, remoteTime time.Time, hasRemoteTime bool) bool {
	info, err := os.Stat(localPath)
	if err != nil {
		return true
	}
	if remoteSize >= 0 && info.Size() != remoteSize {
		return true
	}
	if hasRemoteTime && remoteTime.After(info.ModTime()) {
		return true
	}
	return false
}

func LatestEntryTime(entry openlist.Entry) (time.Time, bool) {
	modified, hasModified := openlist.ParseEntryTime(entry.Modified)
	created, hasCreated := openlist.ParseEntryTime(entry.Created)
	switch {
	case hasModified && hasCreated:
		if modified.After(created) {
			return modified, true
		}
		return created, true
	case hasModified:
		return modified, true
	case hasCreated:
		return created, true
	default:
		return time.Time{}, false
	}
}

func MatchBlacklist(patterns []string, remotePath string) bool {
	remotePath = openlist.NormalizePath(remotePath)
	withoutSlash := strings.TrimPrefix(remotePath, "/")
	base := path.Base(remotePath)
	for _, pattern := range normalizeList(patterns) {
		p := strings.ReplaceAll(pattern, "\\", "/")
		if !strings.Contains(p, "/") {
			if ok, _ := path.Match(p, base); ok {
				return true
			}
		}
		if globMatch(p, remotePath) || globMatch(p, withoutSlash) {
			return true
		}
	}
	return false
}

func localRemotePath(scanDir, localRoot, localPath string, trimSTRM bool) string {
	rel, err := filepath.Rel(localRoot, localPath)
	if err != nil || rel == "." {
		return openlist.NormalizePath(scanDir)
	}
	rel = filepath.ToSlash(rel)
	if trimSTRM && strings.HasSuffix(rel, ".strm") {
		rel = strings.TrimSuffix(rel, ".strm")
	}
	return openlist.JoinPath(scanDir, rel)
}

func within(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func normalizeList(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func globMatch(pattern, value string) bool {
	re, err := regexp.Compile(globToRegex(pattern))
	if err != nil {
		return false
	}
	return re.MatchString(value)
}

func globToRegex(pattern string) string {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					b.WriteString("(?:.*/)?")
					i += 3
					continue
				}
				b.WriteString(".*")
				i += 2
				continue
			}
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
		i++
	}
	b.WriteString("$")
	return b.String()
}
