package welcome

import "strings"

// device guesses what is asking, from the browser's description of itself:
// iphone, android, windows, mac, linux or computer. The guess only chooses
// which page opens first, and every page links to the others. An iPad asking
// for desktop pages calls itself a Macintosh, which is why the Mac page
// points iPads to the iPhone page.
func device(userAgent string) string {
	has := func(s string) bool { return strings.Contains(userAgent, s) }
	switch {
	case has("iPhone"), has("iPad"), has("iPod"):
		return "iphone"
	case has("Android"):
		return "android"
	case has("Windows"):
		return "windows"
	case has("Macintosh"):
		return "mac"
	case has("Linux"), has("X11"), has("CrOS"):
		return "linux"
	}
	return "computer"
}

// safari reports whether an iPhone is using Safari itself. Only Safari hands
// a downloaded profile to the Settings app: Chrome and the other browsers
// save it as a file that installs nothing. Safari announces a version, the
// others announce themselves (CriOS, FxiOS, EdgiOS) and leave the version
// out, and a browser built into another app leaves Safari out as well.
func safari(userAgent string) bool {
	return strings.Contains(userAgent, "Version/") && strings.Contains(userAgent, "Safari/")
}

// pairs writes hex characters two by two, the way phones show fingerprints.
func pairs(hex string) string {
	var b strings.Builder
	for i := 0; i < len(hex); i += 2 {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(hex[i : i+2])
	}
	return b.String()
}
