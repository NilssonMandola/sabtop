package ui

import "testing"

func TestRate(t *testing.T) {
	for in, want := range map[float64]string{
		0:          "0 B/s",
		-1:         "0 B/s",
		56_430:     "56 kB/s",
		56_430_000: "56.4 MB/s",
		2.5e9:      "2.50 GB/s",
	} {
		if got := rate(in); got != want {
			t.Errorf("rate(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestSizeGB(t *testing.T) {
	// SABnzbd reports sizes in megabytes throughout.
	for in, want := range map[float64]string{
		0:           "0 MB",
		917.91:      "918 MB",
		51_293.65:   "51.3 GB",
		4_626_013.7: "4.63 TB",
	} {
		if got := sizeGB(in); got != want {
			t.Errorf("sizeGB(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSize(t *testing.T) {
	for in, want := range map[string]float64{
		"50G": 50e9, "2G": 2e9, "500M": 500e6, "1T": 1e12, "100K": 100e3,
		"": 0, "garbage": 0, "  25G ": 25e9,
	} {
		if got := parseSize(in); got != want {
			t.Errorf("parseSize(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestBytes(t *testing.T) {
	for in, want := range map[int64]string{
		0: "—", 500_000: "500 kB", 5_000_000: "5.0 MB",
		5_000_000_000: "5.0 GB", 110_855_641_001_910: "110.86 TB",
	} {
		if got := bytes(in); got != want {
			t.Errorf("bytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestShortName(t *testing.T) {
	// Release names carry an archive password for some indexers; it has no
	// business being rendered in a queue row.
	in := "Pet.Sematary.1989.UHD.BluRay.REMUX-FraMeSToRpassword=7258M2L6n36g47U96n83U17G29G11235"
	if got := shortName(in); got != "Pet.Sematary.1989.UHD.BluRay.REMUX-FraMeSToR" {
		t.Errorf("shortName kept the password: %q", got)
	}
	if got := shortName("Some.Movie.2001.nzb"); got != "Some.Movie.2001" {
		t.Errorf("shortName(.nzb) = %q", got)
	}
}

func TestVolumeUsage(t *testing.T) {
	free, total, ok := volumeUsage("/")
	if !ok || total == 0 || free > total {
		t.Errorf("volumeUsage(/) = %d, %d, %v — expected a sane reading", free, total, ok)
	}
	if _, _, ok := volumeUsage("/definitely/not/a/path/here"); ok {
		t.Error("a missing path should report not-ok rather than zeroes")
	}
}
