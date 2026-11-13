package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/earthler/earthler/core/certificate"
	"github.com/earthler/earthler/core/copies"
	"github.com/earthler/earthler/core/replica"
)

// pairCode prints what a second machine needs to pair with this one: a code
// that works once, and the fingerprint of the community root, which the
// second machine checks before it trusts anything.
func pairCode(dataDir string) error {
	chain, err := certificate.Load(filepath.Join(dataDir, "secrets", "certificate"))
	if err != nil {
		return fmt.Errorf("this data folder has no community yet, start the system once first: %w", err)
	}
	code, err := replica.NewCode(filepath.Join(dataDir, "replication"), time.Now())
	if err != nil {
		return err
	}
	fmt.Printf("Pairing code: %s\n", code)
	fmt.Printf("It works once, for %d minutes. The system has to be running to answer.\n\n", int(replica.CodeLife.Minutes()))
	fmt.Printf("Fingerprint of the community certificate:\n%s\n\n", chain.Fingerprint())
	fmt.Println("On the second machine, run, with the address of this machine:")
	ips, _ := certificate.LocalAddresses()
	if len(ips) == 0 {
		fmt.Println("  earthler -follow https://ADDRESS -pair-code " + code + " -fingerprint " + chain.Fingerprint())
	}
	for _, ip := range ips {
		fmt.Printf("  earthler -follow https://%s -pair-code %s -fingerprint %s\n", ip, code, chain.Fingerprint())
	}
	return nil
}

// follow is the second machine: it pairs the first time and then pulls the
// copy until it is stopped. It opens no screens and never holds the copy key.
func follow(dir, address, plainAddress, code, fingerprint, label string, every time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	f, err := replica.Open(dir)
	if errors.Is(err, os.ErrNotExist) {
		if code == "" || fingerprint == "" {
			return errors.New("this machine is not paired yet: give -pair-code and -fingerprint, as the main machine printed them with -pair")
		}
		if plainAddress == "" {
			u, perr := url.Parse(address)
			if perr != nil || u.Hostname() == "" {
				return fmt.Errorf("-follow %q is not an address", address)
			}
			plainAddress = "http://" + u.Hostname()
		}
		if f, err = replica.Pair(ctx, replica.PairOptions{URL: address, PlainURL: plainAddress, Code: code,
			Fingerprint: fingerprint, Label: label, Dir: dir}); err != nil {
			return err
		}
		log.Printf("paired with %s; the copy will be kept in %s", address, dir)
	} else if err != nil {
		return err
	}
	err = f.Run(ctx, every, log.Printf)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// checkCopy reads a whole copy with the key, without writing anything: the
// monthly restore test.
func checkCopy(src, keyText string) error {
	key, err := copies.ParseKey(keyText)
	if err != nil {
		return fmt.Errorf("-copy-key: %w", err)
	}
	rep, err := copies.Check(src, key)
	if err != nil {
		return err
	}
	log.Printf("the copy in %s is whole: %d files, %.1f MB, every one checked against the manifest", src, rep.Files, float64(rep.Bytes)/1e6)
	return nil
}

// listMachines says which second machines are paired.
func listMachines(dataDir string) error {
	r, err := replica.OpenRegistry(filepath.Join(dataDir, "replication"))
	if err != nil {
		return err
	}
	machines, err := r.Machines()
	if err != nil {
		return err
	}
	if len(machines) == 0 {
		fmt.Println("No second machine is paired.")
	}
	for _, m := range machines {
		fmt.Printf("%s  %s  paired on %s\n", m.ID, m.Label, m.Made.Local().Format("2006-01-02 15:04"))
	}
	return nil
}

// unpair takes the token of a second machine away, for one that was lost or
// that someone else holds. It can pull no more.
func unpair(dataDir, id string) error {
	r, err := replica.OpenRegistry(filepath.Join(dataDir, "replication"))
	if err != nil {
		return err
	}
	machines, err := r.Machines()
	if err != nil {
		return err
	}
	for _, m := range machines {
		if m.ID == id {
			if err := r.Revoke(id); err != nil {
				return err
			}
			log.Printf("%s (%s) is unpaired and can pull no more", m.ID, m.Label)
			return nil
		}
	}
	return fmt.Errorf("no second machine has the number %q: -machines lists them", id)
}
