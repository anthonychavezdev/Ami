package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"strings"
)

// One-time invite codes, made with `ami-server -new-invite` against the
// database the running server uses, so nothing restarts: 80 random bits shown
// as four groups of base32, kept as a SHA-256 hash, spent by the registration
// that uses them. The first code ever made closes open registration
// (inviteOnly), and spent codes stay on record, so it does not open again by
// itself once they are used up. The standing -invite code keeps working too.

// newInvite makes a code, records its hash and returns the code to show once.
func newInvite(db *sql.DB) (string, error) {
	raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(randomBytes(10)) // 16 characters
	code := raw[:4] + "-" + raw[4:8] + "-" + raw[8:12] + "-" + raw[12:16]
	_, err := db.Exec(`INSERT INTO invites(code_hash, created_at) VALUES (?, ?)`, inviteHash(code), nowString())
	return code, err
}

// inviteHash keys a code as people type it: case and the dashes do not matter.
func inviteHash(code string) string {
	clean := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
	sum := sha256.Sum256([]byte(clean))
	return hex.EncodeToString(sum[:])
}

// inviteOnly says whether registration needs a code right now.
func (s *server) inviteOnly() (bool, error) {
	if s.invite != "" {
		return true, nil
	}
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM invites`).Scan(&n)
	return n > 0, err
}

// inviteUnspent locks a one-time code for the transaction that will spend
// it, so two registrations cannot both use it, and says whether it is good.
func inviteUnspent(tx *sql.Tx, code string) (bool, error) {
	var one int
	err := tx.QueryRow(`SELECT 1 FROM invites WHERE code_hash = ? AND used_at IS NULL FOR UPDATE`, inviteHash(code)).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// spendInvite records who used a code, in the same transaction as the account.
func spendInvite(tx *sql.Tx, code, userID string) error {
	_, err := tx.Exec(`UPDATE invites SET used_at = ?, used_by = ? WHERE code_hash = ?`, nowString(), userID, inviteHash(code))
	return err
}

// printNewInvite is `ami-server -new-invite`.
func printNewInvite(cfg config) error {
	db, err := openDB(cfg.dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()
	code, err := newInvite(db)
	if err != nil {
		return err
	}
	fmt.Printf("Invite code: %s\nIt registers one account. Registration needs a code from now on.\n", code)
	return nil
}
