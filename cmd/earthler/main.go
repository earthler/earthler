// Command earthler is the core of the system: the program a community runs
// on its own machine and reaches through the browser.
//
// It is in early development. It has no passphrase and protects nothing
// personal yet, so it must not be used to run a real community.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/earthler/earthler/core/certificate"
)

func main() {
	var cfg config
	flag.StringVar(&cfg.dataDir, "data", "earthler-data", "folder for the journal, the indexes and the secrets")
	flag.StringVar(&cfg.community, "community", "Comunidade", "name of the community, used only when it is born")
	flag.StringVar(&cfg.short, "name", "comunidade", "short name for the address <name>.internal, fixed when the community is born")
	flag.StringVar(&cfg.lang, "lang", "pt-BR", "language of the screens")
	plain := flag.String("http", ":80", "address of the welcome page")
	secure := flag.String("https", ":443", "address of the system")
	dev := flag.String("dev", "", "serve the system over plain HTTP on this address, for development only")
	flag.StringVar(&cfg.copyDir, "copies", "", "folder that receives an encrypted copy of the data every night")
	copyTo := flag.String("copy", "", "write an encrypted copy of the data into this folder, then exit")
	restoreFrom := flag.String("restore", "", "bring the data back from the copy in this folder into an empty data folder, then exit; needs -copy-key")
	copyKey := flag.String("copy-key", "", "the copy key, as written down, for -restore")
	showKey := flag.Bool("print-copy-key", false, "print the copy key, to write it down, then exit")
	checkDir := flag.String("check", "", "read the whole copy in this folder with -copy-key and say if it is whole, then exit")
	pairing := flag.Bool("pair", false, "print a code and a fingerprint to pair a second machine, then exit")
	followURL := flag.String("follow", "", "run as a second machine of the one at this https address, keeping its encrypted copy")
	followDir := flag.String("follow-dir", "earthler-second", "folder where the second machine keeps the copy")
	givenCode := flag.String("pair-code", "", "the code that -pair printed on the main machine, for the first -follow")
	fingerprint := flag.String("fingerprint", "", "the fingerprint that -pair printed on the main machine, for the first -follow")
	plainURL := flag.String("plain-url", "", "address of the main machine's welcome page, if it is not the same host on port 80")
	label := flag.String("label", "", "a name for this second machine, shown on the main one")
	every := flag.Duration("follow-every", time.Minute, "how often the second machine pulls")
	machines := flag.Bool("machines", false, "list the second machines that are paired, then exit")
	unpairID := flag.String("unpair", "", "take the token of the second machine with this number away, then exit")
	flag.Parse()

	var err error
	switch {
	case *restoreFrom != "":
		err = restore(cfg.dataDir, *restoreFrom, *copyKey)
	case *copyTo != "":
		err = makeCopy(cfg.dataDir, *copyTo)
	case *showKey:
		err = printCopyKey(cfg.dataDir)
	case *checkDir != "":
		err = checkCopy(*checkDir, *copyKey)
	case *pairing:
		err = pairCode(cfg.dataDir)
	case *machines:
		err = listMachines(cfg.dataDir)
	case *unpairID != "":
		err = unpair(cfg.dataDir, *unpairID)
	case *followURL != "":
		err = follow(*followDir, *followURL, *plainURL, *givenCode, *fingerprint, *label, *every)
	default:
		err = run(cfg, *plain, *secure, *dev)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "earthler:", err)
		os.Exit(1)
	}
}

func run(cfg config, plain, secure, dev string) error {
	_, port, err := net.SplitHostPort(secure)
	if err != nil {
		return fmt.Errorf("-https %q: %w", secure, err)
	}
	if cfg.httpsPort, err = strconv.Atoi(port); err != nil {
		return fmt.Errorf("-https %q: %w", secure, err)
	}
	a, err := open(cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	last, _ := a.journal.Last()
	log.Printf("%s: journal at entry %d", a.name, last)

	var servers []*http.Server
	errs := make(chan error, 2)
	serve := func(ln net.Listener, h http.Handler) {
		srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
		servers = append(servers, srv)
		go func() { errs <- srv.Serve(ln) }()
	}

	if dev != "" {
		ln, err := net.Listen("tcp", dev)
		if err != nil {
			return err
		}
		log.Printf("development mode, with no HTTPS: http://%s/", ln.Addr())
		serve(ln, a.handler)
	} else {
		ips, err := certificate.LocalAddresses()
		if err != nil {
			return err
		}
		cert, err := a.chain.Issue(time.Now(), ips)
		if err != nil {
			return err
		}
		secureLn, err := tls.Listen("tcp", secure, &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{*cert}})
		if err != nil {
			return err
		}
		plainLn, err := net.Listen("tcp", plain)
		if err != nil {
			return err
		}
		serve(secureLn, a.handler)
		serve(plainLn, a.welcome)
		_, plainPort, _ := net.SplitHostPort(plainLn.Addr().String())
		log.Printf("certificate for %s, fingerprint starts with %s", a.chain.Domain(), a.chain.Fingerprint()[:8])
		for _, ip := range ips {
			log.Printf("new devices open http://%s/", hostPort(ip.String(), plainPort, "80"))
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-ctx.Done():
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	a.Hush()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, srv := range servers {
		srv.Shutdown(shutdown)
	}
	return nil
}

// hostPort leaves the port out when it is the usual one.
func hostPort(host, port, usual string) string {
	if port == usual {
		return host
	}
	return net.JoinHostPort(host, port)
}
