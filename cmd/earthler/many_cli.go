package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/earthler/earthler/core/certificate"
	"github.com/earthler/earthler/core/machines"
	"github.com/earthler/earthler/core/replica"
)

// invite prints what a new machine needs to join this community as an image
// of the others: a code that works once, and the fingerprint of the community
// root, which the new machine checks before it trusts anything. Only the
// machine that counts takes machines in.
func invite(dataDir string) error {
	id, err := machines.LoadIdentity(filepath.Join(dataDir, "secrets"))
	if err != nil {
		return err
	}
	if id.Number != machines.Counter {
		return errors.New("only the machine that founded the community invites machines, for now")
	}
	chain, err := certificate.Load(filepath.Join(dataDir, "secrets", "certificate"))
	if err != nil {
		return fmt.Errorf("this data folder has no community yet, start the system once first: %w", err)
	}
	code, err := replica.NewCode(filepath.Join(dataDir, "machines"), time.Now())
	if err != nil {
		return err
	}
	fmt.Printf("Invitation code: %s\n", code)
	fmt.Printf("It works once, for %d minutes. The system has to be running to answer.\n\n", int(replica.CodeLife.Minutes()))
	fmt.Printf("Fingerprint of the community certificate:\n%s\n\n", chain.Fingerprint())
	fmt.Println("The new machine receives the community's secrets, serves the residents and keeps a copy of every journal.")
	fmt.Println("On the new machine, with an empty data folder and the address of this machine, run:")
	ips, _ := certificate.LocalAddresses()
	if len(ips) == 0 {
		ips = append(ips, net.ParseIP("192.0.2.1"))
	}
	for _, ip := range ips {
		fmt.Printf("  earthler -join https://%s -pair-code %s -fingerprint %s -label \"a name for it\"\n", ip, code, chain.Fingerprint())
	}
	return nil
}

// joinOptions is what -join needs, from the command line.
type joinOptions struct {
	url, plainURL, code, fingerprint, reach string
}

// join brings this data folder into a community, catches up with the journal
// of the machine that counts, and leaves the folder ready to start.
func join(cfg config, o joinOptions) error {
	u, err := url.Parse(o.url)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return fmt.Errorf("-join %q is not an https address", o.url)
	}
	if o.plainURL == "" {
		o.plainURL = "http://" + u.Hostname()
	}
	if o.reach == "" {
		ips, err := certificate.LocalAddresses()
		if err != nil || len(ips) == 0 {
			return errors.New("give -reach, the https address at which the other machines reach this one")
		}
		o.reach = "https://" + hostPort(ips[0].String(), fmt.Sprint(cfg.httpsPort), "443")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	w, err := machines.Join(ctx, machines.JoinOptions{URL: o.url, PlainURL: o.plainURL, Code: o.code,
		Fingerprint: o.fingerprint, Label: cfg.label, Reach: o.reach, DataDir: cfg.dataDir})
	if err != nil {
		return err
	}
	log.Printf("this is machine %d of the community, with the journal of machine 0 up to now; the others reach it at %s", w.Machine, o.reach)
	return nil
}

// reachOf is the address the others reach a machine at, from its -https.
func reachOf(secure string) string {
	host, port, err := net.SplitHostPort(secure)
	if err != nil || host == "" {
		return ""
	}
	return "https://" + hostPort(strings.Trim(host, "[]"), port, "443")
}
