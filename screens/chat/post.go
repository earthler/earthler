package chat

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/page"
	"github.com/earthler/earthler/core/scribe"
)

// post writes a message.
func (s *Screen) post(w http.ResponseWriter, r *http.Request) {
	if !page.SameOrigin(r) {
		http.Error(w, s.pages.T("error.origin"), http.StatusForbidden)
		return
	}
	sess := accounts.From(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	text := strings.TrimSpace(strings.ReplaceAll(r.FormValue("text"), "\r\n", "\n"))
	switch {
	case text == "":
		s.show(w, r, http.StatusBadRequest, "chat.error.empty", "")
		return
	case len([]rune(text)) > MaxText:
		s.show(w, r, http.StatusBadRequest, "chat.error.long", text)
		return
	}
	channel, _, err := s.channel(r.Context())
	if err != nil {
		log.Printf("chat: %v", err)
		http.Error(w, s.pages.T("error.internal"), http.StatusInternalServerError)
		return
	}
	content, err := event.New(event.MessageCreated, sess.Account, sess.Device, time.Now(),
		event.MessageCreatedData{Channel: channel, Text: text})
	if err == nil {
		_, err = s.scribe.Write(r.Context(), content)
	}
	s.answer(w, r, err, text)
}

// delete takes a message out of the channel. Only its author can, for now:
// the roles that let someone else remove content come with the permission
// table.
func (s *Screen) delete(w http.ResponseWriter, r *http.Request) {
	if !page.SameOrigin(r) {
		http.Error(w, s.pages.T("error.origin"), http.StatusForbidden)
		return
	}
	sess := accounts.From(r.Context())
	number, err := strconv.ParseUint(r.FormValue("number"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var account, channel, deleted uint64
	err = s.index.DB().QueryRowContext(r.Context(), `SELECT account, channel, deleted FROM messages WHERE number = ?`, number).
		Scan(&account, &channel, &deleted)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		http.NotFound(w, r)
		return
	case err != nil:
		log.Printf("chat: %v", err)
		http.Error(w, s.pages.T("error.internal"), http.StatusInternalServerError)
		return
	case account != sess.Account:
		http.Error(w, s.pages.T("chat.error.notyours"), http.StatusForbidden)
		return
	case deleted != 0:
		http.Redirect(w, r, "/chat", http.StatusSeeOther)
		return
	}
	content, err := event.New(event.MessageDeleted, sess.Account, sess.Device, time.Now(),
		event.MessageDeletedData{Channel: channel, Message: number})
	if err == nil {
		_, err = s.scribe.Write(r.Context(), content)
	}
	s.answer(w, r, err, "")
}

// answer ends a write: back to the conversation, or the page again with
// what went wrong and the draft kept.
func (s *Screen) answer(w http.ResponseWriter, r *http.Request, err error, draft string) {
	switch {
	case errors.Is(err, scribe.ErrStopped):
		s.show(w, r, http.StatusServiceUnavailable, "error.stopped", draft)
	case err != nil:
		log.Printf("chat: %v", err)
		s.show(w, r, http.StatusInternalServerError, "error.internal", draft)
	default:
		http.Redirect(w, r, "/chat#end", http.StatusSeeOther)
	}
}
