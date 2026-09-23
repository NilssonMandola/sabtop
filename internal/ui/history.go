package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/NilssonMandola/sabtop/internal/sab"
)

const historyLimit = 200

type historyMsg struct {
	items []sab.HistoryItem
	err   error
}

type historyPane struct {
	client     *sab.Client
	items      []sab.HistoryItem
	failedOnly bool
	cur        cursor
	err        error
	detail     bool
}

func newHistoryPane(c *sab.Client) *historyPane { return &historyPane{client: c} }

func (p *historyPane) Title() string           { return "History" }
func (p *historyPane) Interval() time.Duration { return 15 * time.Second }
func (p *historyPane) Err() error              { return p.err }

func (p *historyPane) Load() tea.Cmd {
	c := p.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		items, err := c.History(ctx, historyLimit)
		return historyMsg{items: items, err: err}
	}
}

func (p *historyPane) visible() []sab.HistoryItem {
	if !p.failedOnly {
		return p.items
	}
	out := make([]sab.HistoryItem, 0, len(p.items))
	for _, h := range p.items {
		if h.Failed() {
			out = append(out, h)
		}
	}
	return out
}

func (p *historyPane) Summary() string {
	failed := 0
	for _, h := range p.items {
		if h.Failed() {
			failed++
		}
	}
	s := fmt.Sprintf("%d entries · %d failed", len(p.items), failed)
	if p.failedOnly {
		s = "failed only · " + s
	}
	return s
}

func (p *historyPane) Keys() []keyHint {
	if p.detail {
		return []keyHint{{"esc/enter", "back"}}
	}
	return []keyHint{
		{"enter", "details"},
		{"f", "failed only"},
		{"R", "retry"},
		{"x", "delete"},
	}
}

func (p *historyPane) selected() (sab.HistoryItem, bool) {
	v := p.visible()
	if p.cur.idx < 0 || p.cur.idx >= len(v) {
		return sab.HistoryItem{}, false
	}
	return v[p.cur.idx], true
}

func (p *historyPane) Handle(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case historyMsg:
		p.err = msg.err
		if msg.err == nil {
			p.items = msg.items
			p.cur.move(0, len(p.visible()))
		}
		return nil, true

	case tea.KeyMsg:
		c := p.client
		switch msg.String() {
		case "enter":
			if _, ok := p.selected(); ok {
				p.detail = !p.detail
			}
			return nil, true
		case "esc":
			if p.detail {
				p.detail = false
				return nil, true
			}
			if p.failedOnly {
				p.failedOnly = false
				p.cur.toTop()
				return nil, true
			}
		case "f":
			p.failedOnly = !p.failedOnly
			p.cur.toTop()
			return nil, true
		case "R":
			h, ok := p.selected()
			if !ok {
				return nil, true
			}
			if !h.Failed() {
				return flash("only failed jobs can be retried", true), true
			}
			return runAction("retrying "+shortName(h.Name), func(ctx context.Context) error {
				return c.RetryJob(ctx, h.NzoID)
			}, p.Load()), true
		case "x":
			h, ok := p.selected()
			if !ok {
				return nil, true
			}
			return confirm(fmt.Sprintf("Delete %s from history, files included?", shortName(h.Name)), true,
				func() tea.Cmd {
					return runAction("deleted "+shortName(h.Name), func(ctx context.Context) error {
						return c.DeleteHistory(ctx, h.NzoID)
					}, p.Load())
				}), true
		}
	}
	return nil, false
}

func (p *historyPane) MoveCursor(delta int) { p.cur.move(delta, len(p.visible())) }
func (p *historyPane) Home()                { p.cur.toTop() }
func (p *historyPane) End()                 { p.cur.toEnd(len(p.visible())) }

func (p *historyPane) View(w, h int) string {
	if p.detail {
		return fillHeight(p.viewDetail(w), h)
	}
	items := p.visible()
	if len(items) == 0 {
		if p.failedOnly {
			return emptyState("Nothing has failed — esc shows everything.", w, h)
		}
		return emptyState("History is empty.", w, h)
	}

	const (
		wMark   = 1
		wCat    = 8
		wSize   = 9
		wWhen   = 8
		wTiming = 16
	)
	wName := w - (wMark + wCat + wSize + wWhen + wTiming + 5)
	if wName < 20 {
		wName = 20
	}

	var b strings.Builder
	b.WriteString(renderRow(false,
		col("", wMark, styleHeaderRow), col("JOB", wName, styleHeaderRow),
		col("CATEGORY", wCat, styleHeaderRow), col("SIZE", wSize, styleHeaderRow),
		col("AGE", wWhen, styleHeaderRow), col("DL + PP", wTiming, styleHeaderRow),
	))
	b.WriteString("\n")

	lo, hi := p.cur.window(len(items), h-1)
	for i := lo; i < hi; i++ {
		it := items[i]

		mark, markStyle, nameStyle := "✓", styleOK, styleText
		switch {
		case it.Failed():
			mark, markStyle, nameStyle = "✗", styleErr, styleErr
		case it.Working():
			mark, markStyle = "⋯", styleWarn
		}

		timing := "—"
		if d, pp := it.DownloadTime.Int(), it.PostProcTime.Int(); d > 0 || pp > 0 {
			timing = fmt.Sprintf("%s + %s",
				hhmmss(time.Duration(d)*time.Second), hhmmss(time.Duration(pp)*time.Second))
		}

		b.WriteString(renderRow(i == p.cur.idx,
			col(mark, wMark, markStyle),
			col(shortName(it.Name), wName, nameStyle),
			col(it.Category, wCat, styleMuted),
			col(it.Size, wSize, styleMuted),
			col(relTime(it.CompletedAt()), wWhen, styleMuted),
			col(timing, wTiming, styleMuted),
		))
		if i < hi-1 {
			b.WriteString("\n")
		}
	}
	return fillHeight(b.String(), h)
}

func (p *historyPane) viewDetail(w int) string {
	it, ok := p.selected()
	if !ok {
		return ""
	}
	var b strings.Builder
	line := func(k, v string) {
		if v == "" {
			v = "—"
		}
		b.WriteString(styleMuted.Render(pad(k, 18)) + styleText.Render(fit(v, w-19)) + "\n")
	}

	b.WriteString(styleTitle.Render(shortName(it.Name)) + "\n\n")
	line("Status", it.Status)
	line("Category", it.Category)
	line("Size", it.Size)
	line("Completed", it.CompletedAt().Local().Format("2006-01-02 15:04:05")+"  ("+relTime(it.CompletedAt())+" ago)")
	line("Download time", hhmmss(time.Duration(it.DownloadTime.Int())*time.Second))
	line("Post-processing", hhmmss(time.Duration(it.PostProcTime.Int())*time.Second))
	line("Stored at", it.Storage)

	if it.FailMessage != "" {
		b.WriteString("\n" + styleErr.Render("Failure") + "\n\n")
		for _, l := range wrap(it.FailMessage, w-2) {
			b.WriteString(styleErr.Render(l) + "\n")
		}
	}
	return b.String()
}
