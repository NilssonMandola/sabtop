package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/NilssonMandola/sabtop/internal/sab"
)

type queueMsg struct {
	queue *sab.Queue
	err   error
}

type queuePane struct {
	client *sab.Client
	queue  *sab.Queue
	cur    cursor
	err    error
}

func newQueuePane(c *sab.Client) *queuePane { return &queuePane{client: c} }

func (p *queuePane) Title() string { return "Queue" }

// Idle queues need no fast polling; a downloading one does.
func (p *queuePane) Interval() time.Duration {
	if p.queue != nil && !p.queue.Paused.Bool() && p.queue.BytesPerSec() > 0 {
		return 2 * time.Second
	}
	return 5 * time.Second
}

func (p *queuePane) Err() error { return p.err }

func (p *queuePane) Load() tea.Cmd {
	c := p.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		q, err := c.Queue(ctx)
		return queueMsg{queue: q, err: err}
	}
}

func (p *queuePane) jobs() []sab.Job {
	if p.queue == nil {
		return nil
	}
	return p.queue.Slots
}

func (p *queuePane) Summary() string {
	if p.queue == nil {
		return ""
	}
	q := p.queue
	state := "downloading"
	if q.Paused.Bool() {
		state = "PAUSED"
	}
	s := fmt.Sprintf("%s · %d jobs · %s left", state, q.NoOfSlots.Int(), sizeGB(q.MBLeft.Float()))
	if !q.Paused.Bool() {
		s += " · " + rate(q.BytesPerSec())
		if q.TimeLeft != "" && q.TimeLeft != "0:00:00" {
			s += " · eta " + q.TimeLeft
		}
	}
	if lim := q.SpeedLimitAbs; lim != "" && lim != "0" {
		s += " · capped " + rate(parseFloat(lim)*1024)
	}
	if w := q.HaveWarnings.Int(); w > 0 {
		s += fmt.Sprintf(" · %d warnings", w)
	}
	return s
}

func (p *queuePane) Keys() []keyHint {
	return []keyHint{
		{"space", "pause/resume job"},
		{"P", "pause/resume all"},
		{"+/-", "priority"},
		{"L", "speed limit"},
		{"x", "delete"},
	}
}

func (p *queuePane) selected() (sab.Job, bool) {
	j := p.jobs()
	if p.cur.idx < 0 || p.cur.idx >= len(j) {
		return sab.Job{}, false
	}
	return j[p.cur.idx], true
}

func (p *queuePane) Handle(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case queueMsg:
		p.err = msg.err
		if msg.err == nil {
			p.queue = msg.queue
			p.cur.move(0, len(p.jobs()))
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

		case " ":
			j, ok := p.selected()
			if !ok {
				return nil, true
			}
			if j.Paused() {
				return runAction("resumed "+shortName(j.Filename), func(ctx context.Context) error {
					return c.ResumeJob(ctx, j.NzoID)
				}, p.Load()), true
			}
			return runAction("paused "+shortName(j.Filename), func(ctx context.Context) error {
				return c.PauseJob(ctx, j.NzoID)
			}, p.Load()), true

		case "x":
			j, ok := p.selected()
			if !ok {
				return nil, true
			}
			return confirm(fmt.Sprintf("Delete %s and its downloaded parts?", shortName(j.Filename)), true,
				func() tea.Cmd {
					return runAction("deleted "+shortName(j.Filename), func(ctx context.Context) error {
						return c.DeleteJob(ctx, j.NzoID)
					}, p.Load())
				}), true

		case "+", "=":
			j, ok := p.selected()
			if !ok {
				return nil, true
			}
			return runAction(shortName(j.Filename)+" → high priority", func(ctx context.Context) error {
				return c.SetPriority(ctx, j.NzoID, sab.PriorityHigh)
			}, p.Load()), true

		case "-", "_":
			j, ok := p.selected()
			if !ok {
				return nil, true
			}
			return runAction(shortName(j.Filename)+" → low priority", func(ctx context.Context) error {
				return c.SetPriority(ctx, j.NzoID, sab.PriorityLow)
			}, p.Load()), true

		case "L":
			return prompt("Speed limit", `percent ("50"), rate ("2M"), or blank for none`,
				func(text string) tea.Cmd {
					limit := strings.TrimSpace(text)
					label := "speed limit removed"
					if limit != "" {
						label = "speed limit set to " + limit
					}
					return runAction(label, func(ctx context.Context) error {
						return c.SetSpeedLimit(ctx, limit)
					}, p.Load())
				}), true
		}
	}
	return nil, false
}

func (p *queuePane) MoveCursor(delta int) { p.cur.move(delta, len(p.jobs())) }
func (p *queuePane) Home()                { p.cur.toTop() }
func (p *queuePane) End()                 { p.cur.toEnd(len(p.jobs())) }

func (p *queuePane) View(w, h int) string {
	jobs := p.jobs()
	if len(jobs) == 0 {
		if p.queue == nil {
			return emptyState("Loading…", w, h)
		}
		return emptyState("Queue is empty.", w, h)
	}

	const (
		wMark = 1
		wCat  = 8
		wSize = 9
		wLeft = 9
		wBar  = 14
		wPct  = 4
		wEta  = 9
	)
	wName := w - (wMark + wCat + wSize + wLeft + wBar + wPct + wEta + 7)
	if wName < 20 {
		wName = 20
	}

	var b strings.Builder
	b.WriteString(renderRow(false,
		col("", wMark, styleHeaderRow), col("JOB", wName, styleHeaderRow),
		col("CATEGORY", wCat, styleHeaderRow), col("SIZE", wSize, styleHeaderRow),
		col("LEFT", wLeft, styleHeaderRow), col("", wBar, styleHeaderRow),
		col("", wPct, styleHeaderRow), col("ETA", wEta, styleHeaderRow),
	))
	b.WriteString("\n")

	lo, hi := p.cur.window(len(jobs), h-1)
	for i := lo; i < hi; i++ {
		j := jobs[i]

		// SABnzbd labels every job in the queue "Downloading", including the
		// 159 that have not started, so its status field says nothing about
		// activity. Progress is the honest signal: a job with bytes on disk
		// has been started, one at 0% is still waiting.
		mark, markStyle := "·", styleFaint
		barStyle := styleFaint
		switch {
		case j.Missing():
			mark, markStyle, barStyle = "!", styleErr, styleErr
		case j.Paused():
			mark, markStyle, barStyle = "⏸", styleWarn, styleWarn
		case p.queue.Paused.Bool():
			mark, markStyle, barStyle = "⏸", styleWarn, styleWarn
		case j.Progress() > 0:
			mark, markStyle, barStyle = "▼", styleOK, styleOK
		}

		plain, styled := progressBar(j.Progress(), wBar, barStyle)
		eta := j.TimeLeft
		if eta == "" || eta == "0:00:00" {
			eta = "—"
		}

		b.WriteString(renderRow(i == p.cur.idx,
			col(mark, wMark, markStyle),
			col(shortName(j.Filename), wName, styleText),
			col(j.Category, wCat, styleMuted),
			col(j.Size, wSize, styleMuted),
			col(j.SizeLeft, wLeft, styleMuted),
			cell{text: plain, w: wBar, style: barStyle, raw: styled},
			col(fmt.Sprintf("%d%%", j.Percentage.Int()), wPct, styleMuted),
			col(eta, wEta, styleMuted),
		))
		if i < hi-1 {
			b.WriteString("\n")
		}
	}
	return fillHeight(b.String(), h)
}

// shortName trims the release-name noise that makes queue rows unreadable,
// keeping the title and year where they can be found.
func shortName(s string) string {
	s = strings.TrimSuffix(s, ".nzb")
	if i := strings.Index(s, "password="); i > 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
