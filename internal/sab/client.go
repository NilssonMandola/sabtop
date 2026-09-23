// Package sab is a small client for the SABnzbd JSON API, covering the subset
// sabtop renders and acts on.
package sab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Client talks to a SABnzbd instance.
type Client struct {
	baseURL string
	key     string
	http    *http.Client
}

// New returns a Client for the SABnzbd at baseURL.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     apiKey,
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

// call performs an API request. SABnzbd answers every call with HTTP 200, so
// failures arrive as {"status": false, "error": "..."} in the body.
func (c *Client) call(ctx context.Context, params url.Values, out any) error {
	params.Set("output", "json")
	params.Set("apikey", c.key)

	u := c.baseURL + "/api?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mode=%s: HTTP %d", params.Get("mode"), resp.StatusCode)
	}
	// An HTML body means the key was rejected before the API layer saw it.
	if bytes := strings.TrimSpace(string(body)); strings.HasPrefix(bytes, "<") {
		return fmt.Errorf("unexpected HTML response — check the API key and that %s is SABnzbd", c.baseURL)
	}
	if err := checkError(body); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

// Version returns the running SABnzbd version, and doubles as a reachability
// and credential check.
func (c *Client) Version(ctx context.Context) (string, error) {
	var v struct {
		Version string `json:"version"`
	}
	if err := c.call(ctx, url.Values{"mode": {"version"}}, &v); err != nil {
		return "", err
	}
	if v.Version == "" {
		return "", fmt.Errorf("no version in response — is %s really SABnzbd?", c.baseURL)
	}
	return v.Version, nil
}

// Queue returns the current download queue.
func (c *Client) Queue(ctx context.Context) (*Queue, error) {
	var r queueResponse
	if err := c.call(ctx, url.Values{"mode": {"queue"}, "limit": {"200"}}, &r); err != nil {
		return nil, err
	}
	return &r.Queue, nil
}

// History returns the most recent finished jobs, newest first.
func (c *Client) History(ctx context.Context, limit int) ([]HistoryItem, error) {
	var r historyResponse
	params := url.Values{"mode": {"history"}, "limit": {strconv.Itoa(limit)}}
	if err := c.call(ctx, params, &r); err != nil {
		return nil, err
	}
	return r.History.Slots, nil
}

// Warnings returns SABnzbd's warning log, newest first.
func (c *Client) Warnings(ctx context.Context) ([]Warning, error) {
	var r warningsResponse
	if err := c.call(ctx, url.Values{"mode": {"warnings"}}, &r); err != nil {
		return nil, err
	}
	// The API returns oldest first; the newest warning is the interesting one.
	for i, j := 0, len(r.Warnings)-1; i < j; i, j = i+1, j-1 {
		r.Warnings[i], r.Warnings[j] = r.Warnings[j], r.Warnings[i]
	}
	return r.Warnings, nil
}

// Servers returns per-news-server usage counters.
func (c *Client) Servers(ctx context.Context) ([]Server, error) {
	var s serverStats
	if err := c.call(ctx, url.Values{"mode": {"server_stats"}}, &s); err != nil {
		return nil, err
	}
	out := make([]Server, 0, len(s.Servers))
	for name, v := range s.Servers {
		srv := Server{
			Name:       name,
			Total:      v.Total.Int(),
			DayTotal:   v.Day.Int(),
			WeekTotal:  v.Week.Int(),
			MonthTotal: v.Month.Int(),
		}
		// articles_tried / articles_success are keyed by date; sum them.
		for _, n := range v.ArticlesTried {
			srv.ArticlesTried += n.Int()
		}
		for _, n := range v.ArticlesSuccess {
			srv.ArticlesSuccess += n.Int()
		}
		out = append(out, srv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Total > out[j].Total })
	return out, nil
}

// Config returns the settings that govern whether the queue can keep moving.
func (c *Client) Config(ctx context.Context) (*Config, error) {
	var r configResponse
	params := url.Values{"mode": {"get_config"}, "section": {"misc"}}
	if err := c.call(ctx, params, &r); err != nil {
		return nil, err
	}
	m := r.Config.Misc
	return &Config{
		TopOnly:               m.TopOnly.Bool(),
		PauseOnPostProcessing: m.PauseOnPostProcessing.Bool(),
		FullDiskAutoResume:    m.FullDiskAutoResume.Bool(),
		DownloadFree:          m.DownloadFree,
		CompleteFree:          m.CompleteFree,
		DownloadDir:           m.DownloadDir,
		CompleteDir:           m.CompleteDir,
	}, nil
}

// --- actions --------------------------------------------------------------

// PauseAll halts the whole queue.
func (c *Client) PauseAll(ctx context.Context) error {
	return c.call(ctx, url.Values{"mode": {"pause"}}, nil)
}

// ResumeAll restarts the queue.
func (c *Client) ResumeAll(ctx context.Context) error {
	return c.call(ctx, url.Values{"mode": {"resume"}}, nil)
}

// PauseJob holds a single job.
func (c *Client) PauseJob(ctx context.Context, nzoID string) error {
	return c.call(ctx, url.Values{"mode": {"queue"}, "name": {"pause"}, "value": {nzoID}}, nil)
}

// ResumeJob releases a single job.
func (c *Client) ResumeJob(ctx context.Context, nzoID string) error {
	return c.call(ctx, url.Values{"mode": {"queue"}, "name": {"resume"}, "value": {nzoID}}, nil)
}

// DeleteJob removes a job from the queue. Downloaded parts are deleted too.
func (c *Client) DeleteJob(ctx context.Context, nzoID string) error {
	params := url.Values{"mode": {"queue"}, "name": {"delete"}, "value": {nzoID}, "del_files": {"1"}}
	return c.call(ctx, params, nil)
}

// Priority levels accepted by SetPriority.
const (
	PriorityLow    = "-1"
	PriorityNormal = "0"
	PriorityHigh   = "1"
	PriorityForce  = "2"
)

// SetPriority changes a queued job's priority.
func (c *Client) SetPriority(ctx context.Context, nzoID, priority string) error {
	params := url.Values{
		"mode": {"queue"}, "name": {"priority"}, "value": {nzoID}, "value2": {priority},
	}
	return c.call(ctx, params, nil)
}

// SetSpeedLimit sets the download cap. The value is a percentage ("50") or an
// absolute rate ("2M", "500K"); an empty string or "100" removes the cap.
func (c *Client) SetSpeedLimit(ctx context.Context, limit string) error {
	params := url.Values{"mode": {"config"}, "name": {"speedlimit"}, "value": {limit}}
	return c.call(ctx, params, nil)
}

// RetryJob re-queues a failed history entry.
func (c *Client) RetryJob(ctx context.Context, nzoID string) error {
	return c.call(ctx, url.Values{"mode": {"retry"}, "value": {nzoID}}, nil)
}

// DeleteHistory removes a history entry, and its downloaded files with it.
func (c *Client) DeleteHistory(ctx context.Context, nzoID string) error {
	params := url.Values{"mode": {"history"}, "name": {"delete"}, "value": {nzoID}, "del_files": {"1"}}
	return c.call(ctx, params, nil)
}

// ClearWarnings empties the warning log.
func (c *Client) ClearWarnings(ctx context.Context) error {
	return c.call(ctx, url.Values{"mode": {"warnings"}, "name": {"clear"}}, nil)
}

// Category is one SABnzbd category and the folder its completed jobs land in.
type Category struct {
	Name string `json:"name"`
	Dir  string `json:"dir"`
}

// Absolute reports whether the category writes to a path of its own rather
// than a folder under complete_dir. Only absolute categories can sit on a
// different volume — and only those can fill up without SABnzbd noticing,
// because diskspace2 reports complete_dir alone.
func (c Category) Absolute() bool { return strings.HasPrefix(c.Dir, "/") }

// Categories returns the configured categories.
func (c *Client) Categories(ctx context.Context) ([]Category, error) {
	var r struct {
		Config struct {
			Categories []Category `json:"categories"`
		} `json:"config"`
	}
	params := url.Values{"mode": {"get_config"}, "section": {"categories"}}
	if err := c.call(ctx, params, &r); err != nil {
		return nil, err
	}
	return r.Config.Categories, nil
}
