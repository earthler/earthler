package page

import (
	"strconv"
	"strings"
	"time"
)

// When writes a moment the way people say it, from now: "agora", "há 5
// minutos", "hoje às 8h12", "ontem às 9h", "12 de nov." and, in another
// year, "12 de nov. de 2025". The words are in the locales.
func (r *Renderer) When(t, now time.Time) string {
	t, now = t.In(time.Local), now.In(time.Local)
	fill := func(key string, pairs ...string) string {
		return strings.NewReplacer(pairs...).Replace(r.T(key))
	}
	since := now.Sub(t)
	day := func(x time.Time) time.Time { return time.Date(x.Year(), x.Month(), x.Day(), 0, 0, 0, 0, time.Local) }
	switch today := day(now); {
	case since < time.Minute:
		return r.T("when.now")
	case since < 2*time.Minute:
		return r.T("when.minute")
	case since < time.Hour:
		return fill("when.minutes", "{n}", strconv.Itoa(int(since/time.Minute)))
	case !t.Before(today):
		return fill("when.today", "{time}", r.clock(t))
	case !t.Before(today.AddDate(0, 0, -1)):
		return fill("when.yesterday", "{time}", r.clock(t))
	case t.Year() == now.Year():
		return fill("when.date", "{day}", strconv.Itoa(t.Day()), "{month}", r.T("when.month."+strconv.Itoa(int(t.Month()))))
	default:
		return fill("when.year", "{day}", strconv.Itoa(t.Day()), "{month}", r.T("when.month."+strconv.Itoa(int(t.Month()))), "{year}", strconv.Itoa(t.Year()))
	}
}

// clock writes an hour as people write it: "8h12", or "9h" on the hour.
func (r *Renderer) clock(t time.Time) string {
	key := "when.clock"
	if t.Minute() == 0 {
		key = "when.clock.hour"
	}
	m := strconv.Itoa(t.Minute())
	if len(m) == 1 {
		m = "0" + m
	}
	return strings.NewReplacer("{h}", strconv.Itoa(t.Hour()), "{m}", m).Replace(r.T(key))
}
