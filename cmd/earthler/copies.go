package main

import (
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/earthler/earthler/core/copies"
)

// makeCopy writes an encrypted copy of the data folder into dest.
func makeCopy(dataDir, dest string) error {
	key, err := copies.LoadKey(filepath.Join(dataDir, "secrets"))
	if err != nil {
		return err
	}
	rep, err := copies.Copy(dataDir, dest, key, time.Now())
	if err != nil {
		return err
	}
	log.Printf("copied %s to %s", rep, dest)
	return nil
}

// restore brings a data folder back from a copy, with the key the person
// wrote down.
func restore(dataDir, src, keyText string) error {
	key, err := copies.ParseKey(keyText)
	if err != nil {
		return fmt.Errorf("-copy-key: %w", err)
	}
	rep, err := copies.Restore(src, dataDir, key)
	if err != nil {
		return err
	}
	log.Printf("restored %d files, %.1f MB, into %s; the indexes are rebuilt when the system starts", rep.Files, float64(rep.Bytes)/1e6, dataDir)
	return nil
}

// printCopyKey shows the key, for the person to write it down.
func printCopyKey(dataDir string) error {
	key, err := copies.LoadKey(filepath.Join(dataDir, "secrets"))
	if err != nil {
		return err
	}
	fmt.Println(key.Text())
	return nil
}
