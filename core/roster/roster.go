// Package roster follows who has which role, from the journal. Any index
// that needs to know a role, or to know whether an act of someone's counted,
// puts the roster table in its schema and calls Apply, so every index reads
// the roles by the same rules and a forged entry changes nothing anywhere.
//
// The rules: the first account of a community is its administrator, and a
// change of role counts only when the one who made it was an administrator
// at that moment, the role can be given by hand, and the account is not a
// child.
package roster

import (
	"database/sql"

	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/permissions"
)

// Schema is the table to add to the schema of an index.
const Schema = `CREATE TABLE roster (account INTEGER PRIMARY KEY, role TEXT NOT NULL);`

// Apply updates the roster for one entry. It reports whether the entry was a
// change of role that took effect, for an index that records the act.
func Apply(tx *sql.Tx, entry journal.Entry) (changed bool, err error) {
	e, err := event.Decode(entry.Content)
	if err != nil {
		return false, nil
	}
	switch e.Type {
	case event.AccountCreated:
		var members int
		if err := tx.QueryRow(`SELECT count(*) FROM roster`).Scan(&members); err != nil {
			return false, err
		}
		role := permissions.Resident
		if members == 0 {
			role = permissions.Administrator
		}
		_, err = tx.Exec(`INSERT OR IGNORE INTO roster (account, role) VALUES (?, ?)`, entry.Number, string(role))
		return false, err
	case event.RoleChanged:
		var d event.RoleChangedData
		if e.Read(&d) != nil || !permissions.Assignable(permissions.Role(d.Role)) {
			return false, nil
		}
		res, err := tx.Exec(`
			UPDATE roster SET role = ?
			WHERE account = ? AND role != 'child' AND role != ?
			AND EXISTS (SELECT 1 FROM roster WHERE account = ? AND role = 'administrator')`,
			d.Role, d.Account, d.Role, e.Account)
		if err != nil {
			return false, err
		}
		n, _ := res.RowsAffected()
		return n > 0, nil
	}
	return false, nil
}
