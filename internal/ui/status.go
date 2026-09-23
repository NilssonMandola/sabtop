package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/NilssonMandola/sabtop/internal/sab"
)

// diskWarnAt is the fill fraction at which a volume is called out even though
// SABnzbd is still happy with it. Ten percent headroom is the usual advice for
// a volume that unpacks archives onto itself.
const diskWarnAt = 0.90

type statusMsg struct {
	queue      *sab.Queue
	config     *sab.Config
	warnings   []sab.Warning
	categories []sab.Category
	err        error
}

type statusPane struct {
	client     *sab.Client
	queue      *sab.Queue
	config     *sab.Config
	warnings   []sab.Warning
	categories []sab.Category
	cur        cursor
	err        error
}

func newStatusPane(c *sab.Client) *statusPane { return &statusPane{client: c} }

func (p *statusPane) Title() string           { return "Status" }
func (p *statusPane) Interval() time.Duration { return 10 * time.Second }
func (p *statusPane) Err() error              { return p.err }

func (p *statusPane) Load() tea.Cmd {
	c := p.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		q, err := c.Queue(ctx)
		if err != nil {
			return statusMsg{err: err}
		}
		cfg, err := c.Config(ctx)
		if err != nil {
			return statusMsg{err: err}
		}
		warnings, err := c.Warnings(ctx)
		if err != nil {
			return statusMsg{err: err}
		}
		cats, err := c.Categories(ctx)
		if err != nil {
			return statusMsg{err: err}
		}
		return statusMsg{queue: q, config: cfg, warnings: warnings, categories: cats}
	}
}

func (p *statusPane) Summary() string {
	if p.queue == nil {
		return ""
	}
	problems := p.problems()
	if len(problems) == 0 {
		return "no problems detected"
	}
	return strings.Join(problems, " · ")
}

// problems reports the conditions that stop, or are about to stop, the queue.
// This is the whole point of the pane: SABnzbd's own warning says only "too
// little diskspace", never which volume.
func (p *statusPane) problems() []string {
	var out []string
	if p.queue == nil {
		return out
	}
	if p.queue.Paused.Bool() {
		out = append(out, "QUEUE PAUSED")
	}
	for _, d := range p.disks() {
		if d.belowFloor {
			out = append(out, fmt.Sprintf("%s below SAB's %s floor", d.label, d.floor))
		} else if d.used >= diskWarnAt {
			out = append(out, fmt.Sprintf("%s %.0f%% full", d.label, d.used*100))
		}
	}
	if p.config != nil && !p.config.TopOnly {
		out = append(out, "top_only off (parallel downloads can fill the temp volume)")
	}
	return out
}

// disk is one volume's worth of the status view.
type disk struct {
	// unreadable marks a category path this machine cannot see, e.g. when
	// sabtop runs somewhere that does not share SABnzbd's mounts.
	unreadable bool
	label      string
	path       string
	freeGB     float64
	totalGB    float64
	used       float64
	floor      string
	belowFloor bool
}

func (p *statusPane) disks() []disk {
	if p.queue == nil {
		return nil
	}
	mk := func(label, path string, free, total float64, floor string) disk {
		d := disk{label: label, path: path, freeGB: free, totalGB: total, floor: floor}
		if total > 0 {
			d.used = (total - free) / total
		}
		if f := parseSize(floor); f > 0 && free*1e9 < f {
			d.belowFloor = true
		}
		return d
	}
	var tempPath, completePath, tempFloor, completeFloor string
	if p.config != nil {
		tempPath, completePath = p.config.DownloadDir, p.config.CompleteDir
		tempFloor, completeFloor = p.config.DownloadFree, p.config.CompleteFree
	}
	out := []disk{
		mk("temp", tempPath, p.queue.DiskSpace1.Float(), p.queue.DiskSpaceTotal1.Float(), tempFloor),
		mk("complete", completePath, p.queue.DiskSpace2.Float(), p.queue.DiskSpaceTotal2.Float(), completeFloor),
	}

	// Categories with an absolute dir can sit on a volume SABnzbd never
	// measures. Those are the ones that fill up silently, so stat them here.
	seen := map[string]bool{tempPath: true, completePath: true}
	for _, cat := range p.categories {
		if !cat.Absolute() || seen[cat.Dir] {
			continue
		}
		seen[cat.Dir] = true
		free, total, ok := volumeUsage(cat.Dir)
		if !ok {
			out = append(out, disk{label: "cat:" + cat.Name, path: cat.Dir, unreadable: true})
			continue
		}
		out = append(out, mk("cat:"+cat.Name, cat.Dir,
			float64(free)/1e9, float64(total)/1e9, completeFloor))
	}
	return out
}

func (p *statusPane) Keys() []keyHint {
	return []keyHint{
		{"P", "pause/resume all"},
		{"C", "clear warnings"},
		{"r", "refresh"},
	}
}

func (p *statusPane) Handle(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case statusMsg:
		p.err = msg.err
		if msg.err == nil {
			p.queue, p.config, p.warnings = msg.queue, msg.config, msg.warnings
			p.categories = msg.categories
			p.cur.move(0, len(p.warnings))
		}
		return nil, true

	case tea.KeyMsg:
		c := p.client
		switch msg.String() {
		case "P":
			if p.queue == nil {
				return nil, true
			}
			if p.queue.Paused.Bool() {
				return runAction("queue resumed", func(ctx context.Context) error {
					return c.ResumeAll(ctx)
				}, p.Load()), true
			}
			return confirm("Pause the whole queue?", false, func() tea.Cmd {
				return runAction("queue paused", func(ctx context.Context) error {
					return c.PauseAll(ctx)
				}, p.Load())
			}), true
		case "C":
			if len(p.warnings) == 0 {
				return nil, true
			}
			return confirm(fmt.Sprintf("Clear all %d warnings?", len(p.warnings)), false, func() tea.Cmd {
				return runAction("warnings cleared", func(ctx context.Context) error {
					return c.ClearWarnings(ctx)
				}, p.Load())
			}), true
		}
	}
	return nil, false
}

func (p *statusPane) MoveCursor(delta int) { p.cur.move(delta, len(p.warnings)) }
func (p *statusPane) Home()                { p.cur.toTop() }
func (p *statusPane) End()                 { p.cur.toEnd(len(p.warnings)) }

func (p *statusPane) View(w, h int) string {
	if p.queue == nil {
		return emptyState("Loading…", w, h)
	}

	var b strings.Builder
	line := func(k, v string, style lipgloss.Style) {
		b.WriteString(styleMuted.Render(pad(k, 24)) + style.Render(fit(v, w-25)) + "\n")
	}

	// --- disks, the thing that actually breaks ---
	b.WriteString(styleTitle.Render("Disks") + "\n")
	for _, d := range p.disks() {
		if d.unreadable {
			b.WriteString("  " + styleFaint.Render(pad(d.label, 12)+"not visible from this machine — "+d.path) + "\n")
			continue
		}
		barStyle := styleOK
		note := ""
		switch {
		case d.belowFloor:
			barStyle = styleErr
			note = "  ← below SAB's " + d.floor + " floor, downloads will pause"
		case d.used >= diskWarnAt:
			barStyle = styleWarn
			note = fmt.Sprintf("  ← under %.0f%% headroom", (1-diskWarnAt)*100)
		}
		_, bar := progressBar(d.used, 24, barStyle)
		label := fmt.Sprintf("%-12s %s %5.1f%%  %s free of %s",
			d.label, bar, d.used*100, tb(d.freeGB), tb(d.totalGB))
		b.WriteString("  " + label + barStyle.Render(note) + "\n")
		if d.path != "" {
			b.WriteString("  " + styleFaint.Render(fit(strings.Repeat(" ", 13)+d.path, w-4)) + "\n")
		}
	}

	// --- queue state ---
	b.WriteString("\n" + styleTitle.Render("Queue") + "\n")
	state, stateStyle := "downloading", styleOK
	if p.queue.Paused.Bool() {
		state, stateStyle = "PAUSED", styleErr
	}
	line("State", state, stateStyle)
	line("Speed", rate(p.queue.BytesPerSec()), styleText)
	limit := "none"
	if l := p.queue.SpeedLimitAbs; l != "" && l != "0" {
		limit = rate(parseFloat(l) * 1024)
	}
	line("Speed limit", limit, styleText)
	line("Queued", fmt.Sprintf("%d jobs · %s remaining", p.queue.NoOfSlots.Int(), sizeGB(p.queue.MBLeft.Float())), styleText)

	// --- the switches that prevent a stuck queue ---
	if c := p.config; c != nil {
		b.WriteString("\n" + styleTitle.Render("Safety switches") + "\n")
		sw := func(name string, on bool, why string) {
			v, style := "off", styleWarn
			if on {
				v, style = "on", styleOK
			}
			b.WriteString(styleMuted.Render(pad(name, 26)) + style.Render(pad(v, 5)) +
				styleFaint.Render(fit(why, w-32)) + "\n")
		}
		sw("top_only", c.TopOnly, "one job at a time, so temp holds a single download")
		sw("pause_on_post_processing", c.PauseOnPostProcessing, "stop fetching while unpacking")
		sw("fulldisk_autoresume", c.FullDiskAutoResume, "resume once space returns")
		line("download_free", c.DownloadFree, styleText)
		line("complete_free", c.CompleteFree, styleText)
	}

	// --- warnings ---
	b.WriteString("\n" + styleTitle.Render(fmt.Sprintf("Warnings (%d)", len(p.warnings))) + "\n")
	if len(p.warnings) == 0 {
		b.WriteString("  " + styleFaint.Render("none") + "\n")
	}
	for i, wn := range p.warnings {
		if i >= 6 {
			b.WriteString("  " + styleFaint.Render(fmt.Sprintf("… and %d older", len(p.warnings)-6)) + "\n")
			break
		}
		style := styleWarn
		if strings.EqualFold(wn.Type, "ERROR") {
			style = styleErr
		}
		b.WriteString("  " + styleFaint.Render(pad(relTime(wn.At()), 6)) + " " +
			style.Render(fit(strings.ReplaceAll(wn.Text, "\n", " "), w-10)) + "\n")
	}

	return fillHeight(b.String(), h)
}

// tb renders a gigabyte count as GB or TB.
func tb(gb float64) string {
	if gb >= 1000 {
		return fmt.Sprintf("%.2f TB", gb/1000)
	}
	return fmt.Sprintf("%.0f GB", gb)
}

// parseSize reads SABnzbd's size settings ("50G", "2G", "500M") as bytes.
func parseSize(s string) float64 {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0
	}
	mult := 1.0
	switch s[len(s)-1] {
	case 'K':
		mult, s = 1e3, s[:len(s)-1]
	case 'M':
		mult, s = 1e6, s[:len(s)-1]
	case 'G':
		mult, s = 1e9, s[:len(s)-1]
	case 'T':
		mult, s = 1e12, s[:len(s)-1]
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v * mult
}
