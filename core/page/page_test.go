package page

import (
	"crypto/tls"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSameOriginComparesSchemeAndHost(t *testing.T) {
	cases := []struct {
		name   string
		secure bool
		host   string
		origin string
		want   bool
	}{
		{"the secure site itself", true, "vila.internal", "https://vila.internal", true},
		{"plain http on the same host, in front of the secure site", true, "vila.internal", "http://vila.internal", false},
		{"https where the server speaks plain http", false, "127.0.0.1:8090", "https://127.0.0.1:8090", false},
		{"development without https", false, "127.0.0.1:8090", "http://127.0.0.1:8090", true},
		{"another host", true, "vila.internal", "https://example.org", false},
		{"another port", true, "vila.internal", "https://vila.internal:8443", false},
		{"no origin", true, "vila.internal", "", false},
		{"the null origin", true, "vila.internal", "null", false},
		{"a file page", true, "vila.internal", "file://", false},
		{"not an address", true, "vila.internal", "%zz", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/chat", nil)
		r.Host = c.host
		if c.secure {
			r.TLS = &tls.ConnectionState{}
		}
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if got := SameOrigin(r); got != c.want {
			t.Errorf("%s: SameOrigin = %v, want %v", c.name, got, c.want)
		}
	}
}

// counted counts what the server reads from a body.
type counted struct {
	r io.Reader
	n int64
}

func (c *counted) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func TestGuardReadsFormsOnceAndWithALimit(t *testing.T) {
	pages, err := New("pt-BR", "Vila")
	if err != nil {
		t.Fatal(err)
	}
	var seen string
	handler := pages.Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.FormValue("text")
		w.WriteHeader(http.StatusNoContent)
	}))

	post := func(contentType string, body io.Reader, length int64) (*httptest.ResponseRecorder, *counted) {
		c := &counted{r: body}
		r := httptest.NewRequest("POST", "/chat", c)
		r.ContentLength = length
		r.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w, c
	}

	seen = ""
	if w, _ := post("application/x-www-form-urlencoded", strings.NewReader("text=ol%C3%A1"), -1); w.Code != http.StatusNoContent || seen != "olá" {
		t.Fatalf("a small form: status %d, text %q", w.Code, seen)
	}

	big := "text=" + strings.Repeat("a", MaxForm)
	if w, c := post("application/x-www-form-urlencoded", strings.NewReader(big), -1); w.Code != http.StatusRequestEntityTooLarge || c.n > MaxForm+1024 {
		t.Fatalf("a big form of unknown length: status %d after reading %d bytes", w.Code, c.n)
	}
	if w, c := post("application/x-www-form-urlencoded", strings.NewReader(big), int64(len(big))); w.Code != http.StatusRequestEntityTooLarge || c.n != 0 {
		t.Fatalf("a big form of known length: status %d after reading %d bytes", w.Code, c.n)
	}

	// A file is never read: the form arrives empty and the screen refuses it.
	var body strings.Builder
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("junk", "junk.bin")
	fw.Write(make([]byte, 40<<10))
	mw.WriteField("text", "from a multipart body")
	mw.Close()
	seen = "untouched"
	w, c := post(mw.FormDataContentType(), strings.NewReader(body.String()), int64(body.Len()))
	if w.Code != http.StatusNoContent || seen != "" || c.n != 0 {
		t.Fatalf("a multipart body: status %d, text %q, %d bytes read", w.Code, seen, c.n)
	}

	// Reading is for forms that change something only.
	r := httptest.NewRequest("GET", "/chat?text=in+the+address", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent || seen != "in the address" {
		t.Fatalf("a GET: status %d, text %q", w.Code, seen)
	}
}
