// Command earthler is the core of the system: the program a community runs
// on its own machine and reaches through the browser.
//
// It is in early development. It serves over plain HTTP, has no passphrase
// and protects nothing personal yet, so it must not be used to run a real
// community.
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
	"syscall"
	"time"
)

func main() {
	var cfg config
	flag.StringVar(&cfg.dataDir, "data", "earthler-data", "folder for the journal, the indexes and the secrets")
	flag.StringVar(&cfg.community, "community", "Comunidade", "name of the community, used only when it is born")
	flag.StringVar(&cfg.lang, "lang", "pt-BR", "language of the screens")
	listen := flag.String("listen", ":8080", "address to serve on")
	flag.Parse()

	if err := run(cfg, *listen); err != nil {
		fmt.Fprintln(os.Stderr, "earthler:", err)
		os.Exit(1)
	}
}

func run(cfg config, listen string) error {
	a, err := open(cfg)
	if err != nil {
		return err
	}
	defer a.Close()

	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	last, _ := a.journal.Last()
	log.Printf("%s: journal at entry %d, serving on %s", a.name, last, ln.Addr())

	srv := &http.Server{Handler: a.handler, ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errs := make(chan error, 1)
	go func() { errs <- srv.Serve(ln) }()

	select {
	case <-ctx.Done():
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}
