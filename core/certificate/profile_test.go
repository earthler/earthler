package certificate

import (
	"encoding/base64"
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

func TestProfileCarriesTheRoot(t *testing.T) {
	c, _, err := Open(t.TempDir(), "vila.internal", "Vila & Horta", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	profile := string(c.Profile(ProfileText{
		Name:         "Vila & Horta",
		Organization: "Earthler",
		Description:  "Só vale para vila.internal <rede local>",
	}))

	// It must be well-formed XML, with the texts escaped.
	dec := xml.NewDecoder(strings.NewReader(profile))
	dec.Strict = true
	for {
		if _, err := dec.Token(); err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("the profile is not well-formed XML: %v", err)
		}
	}
	for _, want := range []string{
		"<data>" + base64.StdEncoding.EncodeToString(c.Root()) + "</data>",
		"<string>Vila &amp; Horta</string>",
		"<string>Só vale para vila.internal &lt;rede local&gt;</string>",
		"<string>Vila &amp; Horta (Earthler)</string>", // the name the phone lists the root under
		"<string>org.earthler.vila.internal</string>",
		"<string>org.earthler.vila.internal.root</string>",
		"<string>com.apple.security.root</string>",
		"<string>Configuration</string>",
	} {
		if !strings.Contains(profile, want) {
			t.Errorf("the profile is missing %s", want)
		}
	}
	if c.RootName() != "Vila & Horta (Earthler)" {
		t.Errorf("root name: %q", c.RootName())
	}

	// The same chain always gives the same identifiers, so a reinstall
	// replaces the profile instead of adding another.
	again := string(c.Profile(ProfileText{Name: "Vila & Horta"}))
	if first, second := uuids(profile), uuids(again); first != second || !strings.Contains(first, "-4") {
		t.Errorf("profile identifiers changed between two calls, or are not UUIDs:\n%s\n%s", first, second)
	}
}

func TestDisplayNameFitsACertificate(t *testing.T) {
	long := strings.Repeat("Comunidade ", 10)
	name := displayName(long, "Earthler")
	if len([]rune(name)) > 64 || !strings.HasSuffix(name, " (Earthler)") {
		t.Errorf("display name %q", name)
	}
	if got := displayName("  ", "Earthler"); got != "Earthler" {
		t.Errorf("empty community gave %q", got)
	}
}

// uuids pulls the PayloadUUID values out of a profile.
func uuids(profile string) string {
	var out []string
	rest := profile
	for {
		i := strings.Index(rest, "<key>PayloadUUID</key>")
		if i < 0 {
			return strings.Join(out, " ")
		}
		rest = rest[i:]
		start := strings.Index(rest, "<string>") + len("<string>")
		end := strings.Index(rest, "</string>")
		out = append(out, rest[start:end])
		rest = rest[end:]
	}
}
