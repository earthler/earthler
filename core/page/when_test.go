package page

import (
	"testing"
	"time"
)

// TestWhenSaysItAsPeopleDo checks the times the search and the portal show,
// such as "hoje às 8h12" in a result of the search.
func TestWhenSaysItAsPeopleDo(t *testing.T) {
	r, err := New("pt-BR", "Vila")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 16, 15, 30, 0, 0, time.Local)
	for _, c := range []struct {
		at   time.Time
		want string
	}{
		{now.Add(-20 * time.Second), "agora"},
		{now.Add(-90 * time.Second), "há 1 minuto"},
		{now.Add(-42 * time.Minute), "há 42 minutos"},
		{time.Date(2026, 11, 16, 8, 12, 0, 0, time.Local), "hoje às 8h12"},
		{time.Date(2026, 11, 15, 9, 0, 0, 0, time.Local), "ontem às 9h"},
		{time.Date(2026, 11, 3, 21, 5, 0, 0, time.Local), "3 de nov."},
		{time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local), "1 de maio"},
		{time.Date(2025, 12, 31, 23, 59, 0, 0, time.Local), "31 de dez. de 2025"},
	} {
		if got := r.When(c.at, now); got != c.want {
			t.Errorf("%v: %q, want %q", c.at, got, c.want)
		}
	}
}
