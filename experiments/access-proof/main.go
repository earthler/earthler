// Command access-proof is a throwaway experiment, not part of the Earthler
// system. It checks, on real phones, the access design that everything else
// depends on. See README.md in this folder.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var validName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

type options struct {
	name      string
	httpPort  int
	httpsPort int
	dataDir   string
	ip        string
}

func main() {
	var o options
	flag.StringVar(&o.name, "name", "demo", "short community name; the certificate covers <name>.internal")
	flag.IntVar(&o.httpPort, "http", 80, "port for the welcome page")
	flag.IntVar(&o.httpsPort, "https", 443, "port for the secure site")
	flag.StringVar(&o.dataDir, "data", "access-proof-data", "folder for the certificate chain and the results")
	flag.StringVar(&o.ip, "ip", "", "local address of this machine, if the automatic choice is wrong")
	flag.Parse()
	log.SetFlags(log.Ltime)
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "access-proof:", err)
		os.Exit(1)
	}
}

func run(o options) error {
	if !validName.MatchString(o.name) {
		return fmt.Errorf("name %q: use lowercase letters, digits and hyphens", o.name)
	}
	domain := o.name + ".internal"
	ips, err := localAddresses(o.ip)
	if err != nil {
		return err
	}
	c, created, err := openChain(o.dataDir, domain)
	if err != nil {
		return err
	}
	cert, err := c.issue(time.Now(), []string{domain}, ips)
	if err != nil {
		return err
	}
	srv, err := newServer(c, cert, ips, o)
	if err != nil {
		return err
	}

	const portHint = "\nIf the port is busy or not allowed, choose others: -http 8080 -https 8443"
	httpLn, err := net.Listen("tcp", ":"+strconv.Itoa(o.httpPort))
	if err != nil {
		return fmt.Errorf("%w%s", err, portHint)
	}
	httpsLn, err := net.Listen("tcp", ":"+strconv.Itoa(o.httpsPort))
	if err != nil {
		return fmt.Errorf("%w%s", err, portHint)
	}
	printBanner(c, created, ips, o)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	plain := &http.Server{Handler: srv.handler(false), ReadHeaderTimeout: 10 * time.Second}
	secure := &http.Server{Handler: srv.handler(true), ReadHeaderTimeout: 10 * time.Second}
	ready := newReadyConns(httpsLn.Addr())
	errs := make(chan error, 2)
	go srv.acceptTLS(httpsLn, ready)
	go func() { errs <- plain.Serve(httpLn) }()
	go func() { errs <- secure.Serve(ready) }()

	select {
	case <-ctx.Done():
	case err := <-errs:
		return err
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	httpsLn.Close()
	plain.Shutdown(shutdown)
	secure.Shutdown(shutdown)
	return nil
}

// openChain loads the chain from dir, or creates and saves a new one.
func openChain(dir, domain string) (c *chain, created bool, err error) {
	c, err = loadChain(dir)
	if errors.Is(err, os.ErrNotExist) {
		c, err = newChain(domain, time.Now())
		if err != nil {
			return nil, false, err
		}
		return c, true, c.save(dir)
	}
	if err != nil {
		return nil, false, err
	}
	if c.domain != domain {
		return nil, false, fmt.Errorf("the chain in %s was created for %s; pass -name %s or delete that folder to start over",
			dir, c.domain, strings.TrimSuffix(c.domain, ".internal"))
	}
	return c, false, nil
}

// localAddresses returns the private IPv4 addresses of this machine, or the
// single address the user chose.
func localAddresses(chosen string) ([]net.IP, error) {
	if chosen != "" {
		ip := net.ParseIP(chosen)
		if ip == nil || !ip.IsPrivate() {
			return nil, fmt.Errorf("-ip %q is not a private network address", chosen)
		}
		return []net.IP{ip}, nil
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	var ips []net.IP
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if ip := ipnet.IP.To4(); ip != nil && ip.IsPrivate() {
			ips = append(ips, ip)
		}
	}
	if len(ips) == 0 {
		return nil, errors.New("no private network address found; join the Wi-Fi network or pass -ip")
	}
	return ips, nil
}

func printBanner(c *chain, created bool, ips []net.IP, o options) {
	fmt.Println()
	fmt.Println("Earthler access proof. An experiment, not the Earthler system.")
	fmt.Println()
	if created {
		fmt.Printf("Created a new certificate chain for %s in %s.\n", c.domain, o.dataDir)
	} else {
		fmt.Printf("Using the certificate chain for %s found in %s.\n", c.domain, o.dataDir)
	}
	fmt.Printf("Root fingerprint, first characters: %s\n", grouped(c.fingerprint()[:8]))
	fmt.Println()
	fmt.Println("On each phone, joined to this Wi-Fi network, open:")
	for _, ip := range ips {
		fmt.Printf("  %s\n", baseURL("http", ip.String(), o.httpPort))
	}
	fmt.Println()
	fmt.Printf("Results are saved in %s.\n", filepath.Join(o.dataDir, resultsFile))
	fmt.Println("Press Ctrl-C to stop.")
	fmt.Println()
}

// baseURL builds scheme://host[:port]/ and leaves out the default port.
func baseURL(scheme, host string, port int) string {
	if (scheme == "http" && port == 80) || (scheme == "https" && port == 443) {
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		return scheme + "://" + host + "/"
	}
	return scheme + "://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/"
}

// grouped writes hex characters in pairs, the way phones show fingerprints.
func grouped(hex string) string {
	var b strings.Builder
	for i := 0; i < len(hex); i += 2 {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(hex[i : i+2])
	}
	return b.String()
}
