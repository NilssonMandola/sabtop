package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/NilssonMandola/sabtop/internal/sab"
)

type serversMsg struct {
	items []sab.Server
	err   error
}

type serversPane struct {
	client *sab.Client
	items  []sab.Server
	cur    cursor
	err    error
}

func newServersPane(c *sab.Client) *serversPane { return &serversPane{client: c} }

func (p *serversPane) Title() string           { return "Servers" }
func (p *serversPane) Interval() time.Duration { return 60 * time.Second }
func (p *serversPane) Err() error              { return p.err }

func (p *serversPane) Load() tea.Cmd {
	c := p.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		items, err := c.Servers(ctx)
		return serversMsg{items: items, err: err}
	}
}

func (p *serversPane) Summary() string {
	var total int64
	for _, s := range p.items {
		total += s.Total
	}
	noun := "servers"
	if len(p.items) == 1 {
		noun = "server"
	}
	return fmt.Sprintf("%d %s · %s downloaded all time", len(p.items), noun, bytes(total))
}

func (p *serversPane) Keys() []keyHint { return []keyHint{{"r", "refresh"}} }

func (p *serversPane) Handle(msg tea.Msg) (tea.Cmd, bool) {
	if msg, ok := msg.(serversMsg); ok {
		p.err = msg.err
		if msg.err == nil {
			p.items = msg.items
			p.cur.move(0, len(p.items))
		}
		return nil, true
	}
	return nil, false
}

func (p *serversPane) MoveCursor(delta int) { p.cur.move(delta, len(p.items)) }
func (p *serversPane) Home()                { p.cur.toTop() }
func (p *serversPane) End()                 { p.cur.toEnd(len(p.items)) }

func (p *serversPane) View(w, h int) string {
	if len(p.items) == 0 {
		return emptyState("No news servers configured.", w, h)
	}

	const (
		wDay   = 11
		wWeek  = 11
		wMonth = 11
		wTotal = 12
		wArts  = 12
	)
	wName := w - (wDay + wWeek + wMonth + wTotal + wArts + 5)
	if wName < 16 {
		wName = 16
	}

	var b strings.Builder
	b.WriteString(renderRow(false,
		col("SERVER", wName, styleHeaderRow), col("TODAY", wDay, styleHeaderRow),
		col("WEEK", wWeek, styleHeaderRow), col("MONTH", wMonth, styleHeaderRow),
		col("ALL TIME", wTotal, styleHeaderRow), col("ARTICLES", wArts, styleHeaderRow),
	))
	b.WriteString("\n")

	lo, hi := p.cur.window(len(p.items), h-1)
	for i := lo; i < hi; i++ {
		s := p.items[i]

		articles := "—"
		if s.ArticlesTried > 0 {
			articles = count(s.ArticlesTried)
		}

		b.WriteString(renderRow(i == p.cur.idx,
			col(s.Name, wName, styleText),
			col(bytes(s.DayTotal), wDay, styleMuted),
			col(bytes(s.WeekTotal), wWeek, styleMuted),
			col(bytes(s.MonthTotal), wMonth, styleMuted),
			col(bytes(s.Total), wTotal, styleText),
			col(articles, wArts, styleMuted),
		))
		if i < hi-1 {
			b.WriteString("\n")
		}
	}
	return fillHeight(b.String(), h)
}

// bytes renders a byte count in decimal units.
func bytes(n int64) string {
	f := float64(n)
	switch {
	case n <= 0:
		return "—"
	case f < 1e6:
		return fmt.Sprintf("%.0f kB", f/1e3)
	case f < 1e9:
		return fmt.Sprintf("%.1f MB", f/1e6)
	case f < 1e12:
		return fmt.Sprintf("%.1f GB", f/1e9)
	default:
		return fmt.Sprintf("%.2f TB", f/1e12)
	}
}

// count renders a large tally compactly: 1.2M rather than 1203481.
func count(n int64) string {
	f := float64(n)
	switch {
	case n < 1_000:
		return fmt.Sprintf("%d", n)
	case n < 1_000_000:
		return fmt.Sprintf("%.1fk", f/1e3)
	default:
		return fmt.Sprintf("%.1fM", f/1e6)
	}
}
