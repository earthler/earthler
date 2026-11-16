package chat

import (
	"log"
	"net/http"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/page"
)

// line is one channel on Talk's home: its name and the last thing said in it.
type line struct {
	Number uint64
	Name   string
	Author string
	Last   string
}

// showHome is Talk's home: the open channels of the community, each with
// the last message, as the drawn screen has them.
func (s *Screen) showHome(w http.ResponseWriter, r *http.Request) {
	sess := accounts.From(r.Context())
	rows, err := s.index.DB().QueryContext(r.Context(), `
		SELECT c.number, c.name, coalesce(a.name, ''), coalesce(m.text, '') FROM channels c
		LEFT JOIN messages m ON m.number = (SELECT number FROM messages
			WHERE channel = c.number AND deleted = 0 ORDER BY place DESC, number DESC LIMIT 1)
		LEFT JOIN authors a ON a.account = m.account
		ORDER BY c.number`)
	if err != nil {
		log.Printf("chat: %v", err)
		http.Error(w, s.pages.T("error.internal"), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.Number, &l.Name, &l.Author, &l.Last); err != nil {
			log.Printf("chat: %v", err)
			continue
		}
		lines = append(lines, l)
	}
	s.pages.Render(w, http.StatusOK, s.home, page.Data{Title: s.pages.T("app.talk.tab.conversations"), App: "talk", Tab: "conversations",
		User: sess.Name, Role: string(sess.Role), Body: lines})
}
