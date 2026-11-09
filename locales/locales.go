// Package locales holds every text that people read on a screen, one file
// per language. The code never carries screen text of its own.
package locales

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed *.json
var files embed.FS

// Catalog maps a key, such as "chat.send", to the text in one language.
type Catalog map[string]string

// Load reads the catalog of a language, such as "pt-BR".
func Load(lang string) (Catalog, error) {
	data, err := files.ReadFile(lang + ".json")
	if err != nil {
		return nil, fmt.Errorf("locales: there is no file for %q", lang)
	}
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("locales: %s.json: %w", lang, err)
	}
	return c, nil
}

// T returns the text for a key. A missing key comes back as the key itself,
// so the gap is visible on the screen instead of silent.
func (c Catalog) T(key string) string {
	if text, ok := c[key]; ok {
		return text
	}
	return key
}
