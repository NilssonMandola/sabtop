package sab

import (
	"encoding/json"
	"testing"
)

// SABnzbd is inconsistent about whether a number is a JSON number or a string,
// and the same field can change shape between versions. These tests pin the
// tolerance down.
func TestFlexTypesAcceptBothShapes(t *testing.T) {
	var q Queue
	body := `{
		"paused": "True",
		"kbpersec": "56430.12",
		"mbleft": 4626013.73,
		"noofslots_total": "160",
		"have_warnings": 20,
		"diskspace1": "190.5",
		"diskspacetotal1": 238.28
	}`
	if err := json.Unmarshal([]byte(body), &q); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !q.Paused.Bool() {
		t.Error(`"True" should parse as true`)
	}
	if got := q.KBPerSec.Float(); got != 56430.12 {
		t.Errorf("kbpersec = %v", got)
	}
	if got := q.MBLeft.Float(); got != 4626013.73 {
		t.Errorf("mbleft as a bare number = %v", got)
	}
	if got := q.NoOfSlots.Int(); got != 160 {
		t.Errorf("noofslots_total = %d", got)
	}
	if got := q.HaveWarnings.Int(); got != 20 {
		t.Errorf("have_warnings as a bare number = %d", got)
	}
}

func TestFlexTypesTolerateEmptyAndNull(t *testing.T) {
	var q Queue
	if err := json.Unmarshal([]byte(`{"kbpersec":"","mbleft":null,"paused":""}`), &q); err != nil {
		t.Fatalf("empty values should not be an error: %v", err)
	}
	if q.KBPerSec.Float() != 0 || q.MBLeft.Float() != 0 || q.Paused.Bool() {
		t.Error("empty values should read as zero/false")
	}
}

func TestFlexFloatRejectsGarbage(t *testing.T) {
	var f flexFloat
	if err := f.UnmarshalJSON([]byte(`"not a number"`)); err == nil {
		t.Error("expected an error for unparseable input")
	}
}

func TestBytesPerSec(t *testing.T) {
	q := Queue{KBPerSec: 1000}
	if got := q.BytesPerSec(); got != 1024000 {
		t.Errorf("BytesPerSec = %v, want 1024000 (SAB reports KiB/s)", got)
	}
}

func TestJobProgressClamps(t *testing.T) {
	for _, tt := range []struct {
		pct  flexInt
		want float64
	}{{0, 0}, {44, 0.44}, {100, 1}, {150, 1}, {-5, 0}} {
		if got := (Job{Percentage: tt.pct}).Progress(); got != tt.want {
			t.Errorf("Progress(%d%%) = %v, want %v", tt.pct, got, tt.want)
		}
	}
}

func TestJobMissingAndPaused(t *testing.T) {
	if !(Job{MBMissing: 12}).Missing() {
		t.Error("a job with missing articles should report Missing")
	}
	if (Job{}).Missing() {
		t.Error("a clean job should not report Missing")
	}
	if !(Job{Status: "paused"}).Paused() {
		t.Error("status matching should be case-insensitive")
	}
}

func TestHistoryFailed(t *testing.T) {
	if !(HistoryItem{Status: "Failed"}).Failed() {
		t.Error("Failed status should count as failed")
	}
	// SABnzbd sometimes leaves the status at "Completed" but fills in a
	// failure message; treat that as failed too.
	if !(HistoryItem{Status: "Completed", FailMessage: "unpack error"}).Failed() {
		t.Error("a fail_message should count as failed regardless of status")
	}
	if (HistoryItem{Status: "Completed"}).Failed() {
		t.Error("a clean completion is not a failure")
	}
}

func TestHistoryWorking(t *testing.T) {
	if !(HistoryItem{Status: "Extracting"}).Working() {
		t.Error("Extracting is still in progress")
	}
	if (HistoryItem{Status: "Completed"}).Working() {
		t.Error("Completed is not in progress")
	}
}

func TestQueueDiskUsed(t *testing.T) {
	q := Queue{DiskSpace1: 50, DiskSpaceTotal1: 200, DiskSpace2: 0, DiskSpaceTotal2: 0}
	used, ok := q.DiskUsed(1)
	if !ok || used != 0.75 {
		t.Errorf("DiskUsed(1) = %v, %v; want 0.75, true", used, ok)
	}
	if _, ok := q.DiskUsed(2); ok {
		t.Error("a zero-size volume should report no usage rather than divide by zero")
	}
}

func TestCategoryAbsolute(t *testing.T) {
	if !(Category{Dir: "/Volumes/nas/movies"}).Absolute() {
		t.Error("a rooted path is absolute")
	}
	// A relative dir lives under complete_dir, which SABnzbd already measures.
	if (Category{Dir: "tv"}).Absolute() {
		t.Error("a relative dir is not absolute")
	}
}

func TestCheckErrorFindsAPIFailure(t *testing.T) {
	if err := checkError([]byte(`{"status":false,"error":"API Key Required"}`)); err == nil {
		t.Fatal("expected an error")
	}
	if err := checkError([]byte(`{"queue":{"paused":false}}`)); err != nil {
		t.Errorf("a normal response should not look like an error: %v", err)
	}
}
