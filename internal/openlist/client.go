package openlist

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const hashSuffix = "-https://github.com/alist-org/alist"

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type Entry struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	IsDir    bool   `json:"is_dir"`
	Modified string `json:"modified"`
	Created  string `json:"created"`
	Sign     string `json:"sign"`
	RawURL   string `json:"raw_url"`
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    httpClient,
	}
}

func HashPassword(password string) string {
	sum := sha256.Sum256([]byte(password + hashSuffix))
	return hex.EncodeToString(sum[:])
}

func (c *Client) LoginHash(ctx context.Context, username, passwordHash string) error {
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := c.post(ctx, "/api/auth/login/hash", "", map[string]string{
		"username": username,
		"password": passwordHash,
	}, &resp); err != nil {
		return err
	}
	if resp.Code != 200 {
		return fmt.Errorf("openlist login failed: %s", resp.Message)
	}
	if resp.Data.Token == "" {
		return errors.New("openlist login returned empty token")
	}
	c.token = resp.Data.Token
	return nil
}

func (c *Client) List(ctx context.Context, dir string) ([]Entry, error) {
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Content []Entry `json:"content"`
		} `json:"data"`
	}
	body := map[string]any{
		"path":     NormalizePath(dir),
		"password": "",
		"page":     1,
		"per_page": 0,
		"refresh":  false,
	}
	if err := c.post(ctx, "/api/fs/list", c.token, body, &resp); err != nil {
		return nil, err
	}
	if resp.Code != 200 {
		return nil, fmt.Errorf("openlist list %s failed: %s", dir, resp.Message)
	}
	return resp.Data.Content, nil
}

func (c *Client) Get(ctx context.Context, filePath string) (Entry, error) {
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    Entry  `json:"data"`
	}
	body := map[string]any{
		"path":     NormalizePath(filePath),
		"password": "",
		"page":     1,
		"per_page": 0,
		"refresh":  false,
	}
	if err := c.post(ctx, "/api/fs/get", c.token, body, &resp); err != nil {
		return Entry{}, err
	}
	if resp.Code != 200 {
		return Entry{}, fmt.Errorf("openlist get %s failed: %s", filePath, resp.Message)
	}
	return resp.Data, nil
}

func (c *Client) post(ctx context.Context, apiPath, token string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+apiPath, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("openlist http %d: %s", res.StatusCode, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return err
	}
	return nil
}

func NormalizePath(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	clean := path.Clean(p)
	if clean == "." {
		return "/"
	}
	return clean
}

func JoinPath(dir, name string) string {
	return NormalizePath(path.Join(NormalizePath(dir), name))
}

func BuildDURL(baseURL, filePath, sign string, encodePath bool) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return "", errors.New("base url is required")
	}
	p := NormalizePath(filePath)
	if encodePath {
		p = EncodePath(p)
	}
	raw := base + "/d" + p
	if sign == "" {
		return raw, nil
	}
	sep := "?"
	if strings.Contains(raw, "?") {
		sep = "&"
	}
	return raw + sep + "sign=" + url.QueryEscape(sign), nil
}

func EncodePath(p string) string {
	p = NormalizePath(p)
	if p == "/" {
		return "/"
	}
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return "/" + strings.Join(parts, "/")
}

func ParseEntryTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"}
	for _, layout := range layouts {
		t, err := time.Parse(layout, raw)
		if err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
