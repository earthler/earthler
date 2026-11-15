// Package machines is the part of a machine that knows the other machines of
// its community: who each one is, how a new one joins, how each follows the
// journals of the others as they are written, and how what is counted
// reaches the machine that counts.
//
// The design is in docs/design/many-machines.md.
package machines

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/earthler/earthler/core/durable"
	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
)

// Counter is the machine that counts: what can only have one owner or one
// result, such as a name or a role, is decided by it alone. For now it is
// the founding machine. With the list of machines, it becomes the first
// listed machine, and what is counted waits for all of them.
const Counter = 0

// Identity is who this machine is in its community: its number, and the
// token it shows the other machines. The founding machine has number 0 and
// gets a token the first time it invites another.
type Identity struct {
	Number uint64 `json:"number"`
	Token  string `json:"token,omitempty"`
}

// IdentityFile is the name of the file, in the secrets folder, that holds
// the identity. It is the one secret a machine never hands to another.
const IdentityFile = "machine.json"

// LoadIdentity reads the identity of the machine. A folder without one is
// the founding machine's, from before there were many machines.
func LoadIdentity(secrets string) (Identity, error) {
	var id Identity
	raw, err := os.ReadFile(filepath.Join(secrets, IdentityFile))
	if errors.Is(err, os.ErrNotExist) {
		return id, nil
	}
	if err != nil {
		return id, err
	}
	if err := json.Unmarshal(raw, &id); err != nil || id.Number > event.MaxMachine {
		return id, fmt.Errorf("machines: %s is damaged: restore it from a copy", filepath.Join(secrets, IdentityFile))
	}
	return id, nil
}

// SaveIdentity writes the identity of the machine.
func SaveIdentity(secrets string, id Identity) error {
	if err := os.MkdirAll(secrets, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(id)
	if err != nil {
		return err
	}
	return durable.WriteFile(filepath.Join(secrets, IdentityFile), raw, 0o600)
}

// Area is the index of the machines of the community. Only the machine that
// counts adds a machine, so an entry that says otherwise, from any other
// journal, changes nothing.
var Area = index.Area{
	Name:   "machines",
	Schema: `CREATE TABLE machines (number INTEGER PRIMARY KEY, label TEXT NOT NULL, token TEXT NOT NULL UNIQUE, added INTEGER NOT NULL);`,
	Apply:  apply,
}

func apply(tx *sql.Tx, entry index.Entry) error {
	e, err := event.Decode(entry.Content)
	if err != nil || e.Type != event.MachineAdded || entry.Machine != Counter {
		return nil
	}
	var d event.MachineAddedData
	if e.Read(&d) != nil || d.Machine > event.MaxMachine || len(d.Token) != 64 {
		return nil
	}
	_, err = tx.Exec(`INSERT OR IGNORE INTO machines (number, label, token, added) VALUES (?, ?, ?, ?)`,
		d.Machine, d.Label, d.Token, e.Time)
	return err
}

// Machine is one machine of the community.
type Machine struct {
	Number uint64
	Label  string
	Added  time.Time
}

// List returns the machines of the community, by number.
func List(ctx context.Context, ix *index.Index) ([]Machine, error) {
	rows, err := ix.DB().QueryContext(ctx, `SELECT number, label, added FROM machines ORDER BY number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []Machine
	for rows.Next() {
		var m Machine
		var added int64
		if err := rows.Scan(&m.Number, &m.Label, &added); err != nil {
			return nil, err
		}
		m.Added = time.UnixMilli(added)
		all = append(all, m)
	}
	return all, rows.Err()
}

// byToken says which machine a token belongs to.
func byToken(ctx context.Context, ix *index.Index, token string) (uint64, bool, error) {
	var n uint64
	err := ix.DB().QueryRowContext(ctx, `SELECT number FROM machines WHERE token = ?`, digest(token)).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return n, err == nil, err
}

func digest(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

// tidyLabel keeps a name for a machine short and on one line.
func tidyLabel(label string) string {
	label = strings.Join(strings.Fields(label), " ")
	if r := []rune(label); len(r) > 60 {
		label = string(r[:60])
	}
	return label
}
