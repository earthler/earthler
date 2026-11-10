package welcome

import "testing"

func TestDeviceAndBrowserDetection(t *testing.T) {
	cases := []struct {
		agent  string
		device string
		safari bool
	}{
		// Safari on the iPhone that ran the first access proof.
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/27.0.1 Mobile/15E148 Safari/604.1", "iphone", true},
		// Chrome on the same iPhone, which could not install the certificate.
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 27_0_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/156.0.8078.54 Mobile/15E148 Safari/604.1", "iphone", false},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/130.0 Mobile/15E148 Safari/605.1.15", "iphone", false},
		// A browser built into another app, with no Safari token at all.
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148", "iphone", false},
		{"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Mobile Safari/537.36", "android", false},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0", "windows", false},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15", "mac", false},
		{"Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0", "linux", false},
		{"Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36", "linux", false},
		{"curl/8.7.1", "computer", false},
	}
	for _, c := range cases {
		if got := device(c.agent); got != c.device {
			t.Errorf("device(%q) = %q, want %q", c.agent, got, c.device)
		}
		if got := device(c.agent) == "iphone" && safari(c.agent); got != c.safari {
			t.Errorf("safari(%q) = %v, want %v", c.agent, got, c.safari)
		}
	}
	if pairs("A1F39C07") != "A1 F3 9C 07" {
		t.Errorf("pairs: %q", pairs("A1F39C07"))
	}
}
