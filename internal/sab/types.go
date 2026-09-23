package sab

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SABnzbd returns numbers as strings more often than not, and occasionally as
// real JSON numbers for the same field. flexFloat accepts either.
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("parse %q as number: %w", s, err)
	}
	*f = flexFloat(v)
	return nil
}

func (f flexFloat) Float() float64 { return float64(f) }
func (f flexFloat) Int() int64     { return int64(f) }

// flexInt is flexFloat's integer sibling.
type flexInt int64

func (i *flexInt) UnmarshalJSON(b []byte) error {
	var f flexFloat
	if err := f.UnmarshalJSON(b); err != nil {
		return err
	}
	*i = flexInt(f)
	return nil
}

func (i flexInt) Int() int64 { return int64(i) }

// flexBool copes with true, "True", "1" and "".
type flexBool bool

func (v *flexBool) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	switch strings.ToLower(s) {
	case "true", "1", "yes":
		*v = true
	default:
		*v = false
	}
	return nil
}

func (v flexBool) Bool() bool { return bool(v) }

// Job is one item in the download queue.
type Job struct {
	Index      int       `json:"index"`
	NzoID      string    `json:"nzo_id"`
	Filename   string    `json:"filename"`
	Category   string    `json:"cat"`
	Script     string    `json:"script"`
	Priority   string    `json:"priority"`
	Status     string    `json:"status"`
	Size       string    `json:"size"`
	SizeLeft   string    `json:"sizeleft"`
	MB         flexFloat `json:"mb"`
	MBLeft     flexFloat `json:"mbleft"`
	MBMissing  flexFloat `json:"mbmissing"`
	Percentage flexInt   `json:"percentage"`
	TimeLeft   string    `json:"timeleft"`
	AvgAge     string    `json:"avg_age"`
	TimeAdded  int64     `json:"time_added"`
}

// Progress is the fraction of the job already downloaded, in [0,1].
func (j Job) Progress() float64 {
	p := float64(j.Percentage.Int()) / 100
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}

// Paused reports whether this individual job is held.
func (j Job) Paused() bool { return strings.EqualFold(j.Status, "Paused") }

// Missing reports whether articles are known to be missing, which usually
// means the job will fail on completion.
func (j Job) Missing() bool { return j.MBMissing.Float() > 0 }

// Added returns when the job entered the queue.
func (j Job) Added() time.Time {
	if j.TimeAdded == 0 {
		return time.Time{}
	}
	return time.Unix(j.TimeAdded, 0)
}

// Queue is the state of the download queue as a whole.
type Queue struct {
	Status          string    `json:"status"`
	Paused          flexBool  `json:"paused"`
	PausedAll       flexBool  `json:"paused_all"`
	PauseInt        string    `json:"pause_int"`
	Speed           string    `json:"speed"`
	KBPerSec        flexFloat `json:"kbpersec"`
	SpeedLimit      string    `json:"speedlimit"`
	SpeedLimitAbs   string    `json:"speedlimit_abs"`
	TimeLeft        string    `json:"timeleft"`
	MB              flexFloat `json:"mb"`
	MBLeft          flexFloat `json:"mbleft"`
	SizeLeft        string    `json:"sizeleft"`
	NoOfSlots       flexInt   `json:"noofslots_total"`
	HaveWarnings    flexInt   `json:"have_warnings"`
	Version         string    `json:"version"`
	CacheSize       string    `json:"cache_size"`
	CacheArt        flexInt   `json:"cache_art"`
	DiskSpace1      flexFloat `json:"diskspace1"`      // temp, GB free
	DiskSpace2      flexFloat `json:"diskspace2"`      // complete, GB free
	DiskSpaceTotal1 flexFloat `json:"diskspacetotal1"` // temp, GB total
	DiskSpaceTotal2 flexFloat `json:"diskspacetotal2"` // complete, GB total
	Slots           []Job     `json:"slots"`
}

// BytesPerSec is the current download rate.
func (q Queue) BytesPerSec() float64 { return q.KBPerSec.Float() * 1024 }

// DiskUsed returns the used fraction of a volume, in [0,1]. n selects the temp
// (1) or complete (2) volume.
func (q Queue) DiskUsed(n int) (float64, bool) {
	free, total := q.DiskSpace1.Float(), q.DiskSpaceTotal1.Float()
	if n == 2 {
		free, total = q.DiskSpace2.Float(), q.DiskSpaceTotal2.Float()
	}
	if total <= 0 {
		return 0, false
	}
	used := (total - free) / total
	if used < 0 {
		used = 0
	}
	if used > 1 {
		used = 1
	}
	return used, true
}

type queueResponse struct {
	Queue Queue `json:"queue"`
}

// HistoryItem is a finished job: completed, failed, or still post-processing.
type HistoryItem struct {
	NzoID        string  `json:"nzo_id"`
	Name         string  `json:"name"`
	Status       string  `json:"status"`
	Category     string  `json:"category"`
	Size         string  `json:"size"`
	Bytes        flexInt `json:"bytes"`
	FailMessage  string  `json:"fail_message"`
	Completed    int64   `json:"completed"`
	DownloadTime flexInt `json:"download_time"`
	PostProcTime flexInt `json:"postproc_time"`
	Storage      string  `json:"storage"`
}

// Failed reports whether the job ended badly.
func (h HistoryItem) Failed() bool {
	return strings.EqualFold(h.Status, "Failed") || h.FailMessage != ""
}

// Working reports whether the job is still being unpacked or verified.
func (h HistoryItem) Working() bool {
	switch strings.ToLower(h.Status) {
	case "completed", "failed":
		return false
	}
	return true
}

// CompletedAt returns when the job finished.
func (h HistoryItem) CompletedAt() time.Time {
	if h.Completed == 0 {
		return time.Time{}
	}
	return time.Unix(h.Completed, 0)
}

type historyResponse struct {
	History struct {
		Slots     []HistoryItem `json:"slots"`
		NoOfSlots flexInt       `json:"noofslots"`
		TotalSize string        `json:"total_size"`
		MonthSize string        `json:"month_size"`
		WeekSize  string        `json:"week_size"`
		DaySize   string        `json:"day_size"`
	} `json:"history"`
}

// Warning is one entry from SABnzbd's warning log.
type Warning struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Time int64  `json:"time"`
}

// At returns when the warning fired.
func (w Warning) At() time.Time {
	if w.Time == 0 {
		return time.Time{}
	}
	return time.Unix(w.Time, 0)
}

type warningsResponse struct {
	Warnings []Warning `json:"warnings"`
}

// Server is one configured news server and its usage counters.
//
// Note that articles_success is NOT the count of articles fetched
// successfully: SABnzbd only increments it when this server serves an article
// a previous server failed to provide. On a single-server setup it stays at
// zero, so no success rate is derived from it.
type Server struct {
	Name            string
	Total           int64
	ArticlesTried   int64
	ArticlesSuccess int64
	DayTotal        int64
	WeekTotal       int64
	MonthTotal      int64
}

// serverStats mirrors the server_stats response, whose per-server object keys
// are the server names themselves.
type serverStats struct {
	Total   flexInt `json:"total"`
	Servers map[string]struct {
		Total           flexInt            `json:"total"`
		Day             flexInt            `json:"day"`
		Week            flexInt            `json:"week"`
		Month           flexInt            `json:"month"`
		ArticlesTried   map[string]flexInt `json:"articles_tried"`
		ArticlesSuccess map[string]flexInt `json:"articles_success"`
	} `json:"servers"`
}

// Config holds the handful of SABnzbd settings sabtop surfaces, the ones that
// decide whether the queue can keep moving.
type Config struct {
	TopOnly               bool
	PauseOnPostProcessing bool
	FullDiskAutoResume    bool
	DownloadFree          string
	CompleteFree          string
	DownloadDir           string
	CompleteDir           string
}

type configResponse struct {
	Config struct {
		Misc struct {
			TopOnly               flexBool `json:"top_only"`
			PauseOnPostProcessing flexBool `json:"pause_on_post_processing"`
			FullDiskAutoResume    flexBool `json:"fulldisk_autoresume"`
			DownloadFree          string   `json:"download_free"`
			CompleteFree          string   `json:"complete_free"`
			DownloadDir           string   `json:"download_dir"`
			CompleteDir           string   `json:"complete_dir"`
		} `json:"misc"`
	} `json:"config"`
}

// apiError is the shape SABnzbd uses to report a refused call. It answers with
// HTTP 200 regardless, so the body has to be inspected.
type apiError struct {
	Status flexBool `json:"status"`
	Error  string   `json:"error"`
}

// checkError reports an API-level failure hiding inside a 200 response.
func checkError(body []byte) error {
	var e apiError
	if err := json.Unmarshal(body, &e); err != nil {
		return nil // not the error shape; the caller decodes it properly
	}
	if e.Error != "" {
		return fmt.Errorf("sabnzbd: %s", e.Error)
	}
	return nil
}
