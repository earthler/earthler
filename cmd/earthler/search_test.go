package main

import "testing"

// TestTheSearchNarrowsByApp is the search as docs/product/portal.md
// describes it: the row of filters, how many results and where, the way
// from a result to its message, and an app that holds nothing yet.
func TestTheSearchNarrowsByApp(t *testing.T) {
	s := startSite(t, t.TempDir())
	ana := newVisitor(t)
	ana.signUp(s, "Ana")
	ana.say(s, garden)

	_, body := ana.get(s, "/search")
	wantIn(t, body, "Escreva no campo acima o que você procura.", "the search with nothing typed")

	_, body = ana.get(s, "/search?q=horta")
	for _, want := range []string{
		"1 resultado em toda a comunidade",
		">Tudo</a>", ">Conversas</a>", ">Aulas</a>", ">Decisões</a>", ">Mentor</a>", ">Lugares</a>",
		`aria-current="true">Tudo</a>`,
		`href="/talk/channel/2#m`,
		`aria-label="Voltar ao portal"`,
		`aria-label="Limpar a busca"`,
	} {
		wantIn(t, body, want, "the search for horta")
	}

	_, body = ana.get(s, "/search?q=horta&in=talk")
	wantIn(t, body, "1 resultado em Conversas", "the search in Talk")

	_, body = ana.get(s, "/search?q=horta&in=learn")
	wantIn(t, body, "Nada em Aulas para “horta”.", "the search in an app that holds nothing yet")
	wantIn(t, body, `href="/search?q=horta"`, "the way back to every app")
	wantOut(t, plain(body), garden, "the search in Learn")

	_, body = ana.get(s, "/search?q=horta&in=nowhere")
	wantIn(t, body, "1 resultado em toda a comunidade", "a filter that does not exist")

	_, body = ana.get(s, "/talk/channel/2")
	wantIn(t, body, `<li id="m`, "the anchor a result opens")

	// The author is never marked, only the message.
	ana.say(s, "Ana plantou ana-roxa")
	_, body = ana.get(s, "/search?q=ana")
	wantIn(t, body, "Ana: <mark>Ana</mark> plantou", "the result for ana")
	wantOut(t, body, "<mark>Ana</mark>:", "the author's name")

	// A part of an app that is not built answers only at its own address.
	if status, _ := ana.get(s, "/maps/somewhere"); status != 404 {
		t.Fatalf("/maps/somewhere answered %d, want 404", status)
	}
}
