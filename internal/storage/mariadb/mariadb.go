/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package mariadb

import (
        "fmt"
	"database/sql"
	"strings"
	"log"
    "golang.org/x/crypto/bcrypt"
	_ "github.com/go-sql-driver/mysql"
	"Glasir-MTA/internal/storage/types"
)

type DB struct {
	db *sql.DB
}

// Close chiude la connessione al database MariaDB.
func (s *DB) Close() error {
	return s.db.Close()
}

func New(dsn string, maxOpen, maxIdle int) (*DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &DB{db: db}, nil
}

// --- Config ---
func (s *DB) ConfigGet(key string) (string, error) {
	var val string
	err := s.db.QueryRow("SELECT value FROM config WHERE name = ?", key).Scan(&val)
	if err == sql.ErrNoRows { return "", nil }
	return val, err
}
func (s *DB) ConfigSet(key, value string) error {
	_, err := s.db.Exec("INSERT INTO config (name, value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value = ?", key, value, value)
	return err
}
func (s *DB) ConfigSetPassword(password string) error { return s.ConfigSet("password", password) }
func (s *DB) ConfigTryPassword(password string) (bool, error) {
	stored, err := s.ConfigGet("password")
	if err != nil { return false, err }
	return stored == password, nil
}

// --- mailbox e Mail ---
// ── Metodi Mailbox user-scoped ────────────────────────────────────────────────
// Sostituisci tutti i vecchi metodi Mailbox* e Mail* in mariadb.go con questi.

func (s *DB) MailboxSelect(username, mailbox string) (bool, error) {
	var id int
	err := s.db.QueryRow(`
		SELECT mb.id FROM mailboxes mb
		JOIN users u ON mb.user_id = u.id
		WHERE u.address = ? AND mb.name = ?`, username, mailbox).Scan(&id)
	return err == nil, nil
}

func (s *DB) MailNextID(username, mailbox string) (int, error) {
	var next int
	err := s.db.QueryRow(`
		SELECT mb.uid_next FROM mailboxes mb
		JOIN users u ON mb.user_id = u.id
		WHERE u.address = ? AND mb.name = ?`, username, mailbox).Scan(&next)
	return next, err
}

func (s *DB) MailIDForSeq(username, mailbox string, seq int) (int, error) {
	var uid int
	err := s.db.QueryRow(`
		SELECT m.uid FROM messages m
		JOIN mailboxes mb ON m.mailbox_id = mb.id
		JOIN users u ON mb.user_id = u.id
		WHERE u.address = ? AND mb.name = ?
		ORDER BY m.uid LIMIT 1 OFFSET ?`, username, mailbox, seq-1).Scan(&uid)
	return uid, err
}

func (s *DB) MailUnseen(username, mailbox string) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM messages m
		JOIN mailboxes mb ON m.mailbox_id = mb.id
		JOIN users u ON mb.user_id = u.id
		WHERE u.address = ? AND mb.name = ?
		AND (m.flags IS NULL OR m.flags NOT LIKE '%\\Seen%')`,
		username, mailbox).Scan(&count)
	return count, err
}

func (s *DB) MailboxList(username string, onlySubscribed bool) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT mb.name FROM mailboxes mb
		JOIN users u ON mb.user_id = u.id
		WHERE u.address = ?
		ORDER BY mb.name`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			list = append(list, name)
		}
	}
	return list, rows.Err()
}

func (s *DB) MailboxCreate(username, name string) error {
	_, err := s.db.Exec(`
		INSERT IGNORE INTO mailboxes (user_id, name)
		SELECT id, ? FROM users WHERE address = ?`, name, username)
	return err
}

func (s *DB) MailboxRename(username, old, new string) error {
	_, err := s.db.Exec(`
		UPDATE mailboxes mb JOIN users u ON mb.user_id = u.id
		SET mb.name = ?
		WHERE u.address = ? AND mb.name = ?`, new, username, old)
	return err
}

func (s *DB) MailboxDelete(username, name string) error {
	_, err := s.db.Exec(`
		DELETE mb FROM mailboxes mb
		JOIN users u ON mb.user_id = u.id
		WHERE u.address = ? AND mb.name = ?`, username, name)
	return err
}

func (s *DB) MailboxSubscribe(username, name string, subscribed bool) error {
	_, err := s.db.Exec(`
		UPDATE mailboxes mb JOIN users u ON mb.user_id = u.id
		SET mb.subscribed = ?
		WHERE u.address = ? AND mb.name = ?`, subscribed, username, name)
	return err
}

// ── Metodi Mail user-scoped ───────────────────────────────────────────────────

func (s *DB) MailCreate(username, mailbox string, data []byte) (int, error) {
	var mbID, nextUID int
	err := s.db.QueryRow(`
		SELECT mb.id, mb.uid_next FROM mailboxes mb
		JOIN users u ON mb.user_id = u.id
		WHERE u.address = ? AND mb.name = ?`, username, mailbox).Scan(&mbID, &nextUID)
	if err != nil {
		return 0, fmt.Errorf("mailbox %q non trovata per %s: %w", mailbox, username, err)
	}
	_, err = s.db.Exec(
		`INSERT INTO messages (mailbox_id, uid, body, size) VALUES (?, ?, ?, ?)`,
		mbID, nextUID, data, len(data))
	if err != nil {
		return 0, err
	}
	s.db.Exec(`UPDATE mailboxes SET uid_next = uid_next + 1 WHERE id = ?`, mbID)
	return nextUID, nil
}

func (s *DB) MailSelect(username, mailbox string, id int) (int, *types.Mail, error) {
	var body []byte
	var flags string
	err := s.db.QueryRow(`
		SELECT m.body, COALESCE(m.flags, '')
		FROM messages m
		JOIN mailboxes mb ON m.mailbox_id = mb.id
		JOIN users u ON mb.user_id = u.id
		WHERE u.address = ? AND mb.name = ? AND m.uid = ?`,
		username, mailbox, id).Scan(&body, &flags)
	if err != nil {
		return 0, nil, err
	}
	return len(body), &types.Mail{
		Mailbox:  mailbox,
		ID:       id,
		Mail:     body,
		Seen:     strings.Contains(flags, `\Seen`),
		Answered: strings.Contains(flags, `\Answered`),
		Flagged:  strings.Contains(flags, `\Flagged`),
		Deleted:  strings.Contains(flags, `\Deleted`),
	}, nil
}

func (s *DB) MailSearch(username, mailbox string) ([]uint32, error) {
	rows, err := s.db.Query(`
		SELECT m.uid FROM messages m
		JOIN mailboxes mb ON m.mailbox_id = mb.id
		JOIN users u ON mb.user_id = u.id
		WHERE u.address = ? AND mb.name = ?
		ORDER BY m.uid`, username, mailbox)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var uids []uint32
	for rows.Next() {
		var uid uint32
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		uids = append(uids, uid)
	}
	return uids, rows.Err()
}

func (s *DB) MailUpdateFlags(username, mailbox string, id int, seen, answered, flagged, deleted bool) error {
	var flags []string
	if seen     { flags = append(flags, `\Seen`) }
	if answered { flags = append(flags, `\Answered`) }
	if flagged  { flags = append(flags, `\Flagged`) }
	if deleted  { flags = append(flags, `\Deleted`) }
	_, err := s.db.Exec(`
		UPDATE messages m
		JOIN mailboxes mb ON m.mailbox_id = mb.id
		JOIN users u ON mb.user_id = u.id
		SET m.flags = ?
		WHERE u.address = ? AND mb.name = ? AND m.uid = ?`,
		strings.Join(flags, " "), username, mailbox, id)
	return err
}

func (s *DB) MailDelete(username, mailbox string, id int) error {
	_, err := s.db.Exec(`
		DELETE m FROM messages m
		JOIN mailboxes mb ON m.mailbox_id = mb.id
		JOIN users u ON mb.user_id = u.id
		WHERE u.address = ? AND mb.name = ? AND m.uid = ?`,
		username, mailbox, id)
	return err
}

func (s *DB) MailExpunge(username, mailbox string) error {
	_, err := s.db.Exec(`
		DELETE m FROM messages m
		JOIN mailboxes mb ON m.mailbox_id = mb.id
		JOIN users u ON mb.user_id = u.id
		WHERE u.address = ? AND mb.name = ?
		AND m.flags LIKE '%\\Deleted%'`,
		username, mailbox)
	return err
}

func (s *DB) MailCount(username, mailbox string) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM messages m
		JOIN mailboxes mb ON m.mailbox_id = mb.id
		JOIN users u ON mb.user_id = u.id
		WHERE u.address = ? AND mb.name = ?`, username, mailbox).Scan(&count)
	return count, err
}

func (s *DB) MailMove(username, mailbox string, id int, dest string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var destID, nextUID int
	err = tx.QueryRow(`
		SELECT mb_dest.id, mb_dest.uid_next
		FROM mailboxes mb_src
		JOIN users u ON mb_src.user_id = u.id
		JOIN mailboxes mb_dest ON mb_dest.user_id = u.id
		WHERE u.address = ? AND mb_src.name = ? AND mb_dest.name = ?`,
		username, mailbox, dest).Scan(&destID, &nextUID)
	if err != nil {
		return fmt.Errorf("mailbox dest %q non trovata: %w", dest, err)
	}

	_, err = tx.Exec(`
		UPDATE messages m
		JOIN mailboxes mb ON m.mailbox_id = mb.id
		JOIN users u ON mb.user_id = u.id
		SET m.mailbox_id = ?, m.uid = ?
		WHERE u.address = ? AND mb.name = ? AND m.uid = ?`,
		destID, nextUID, username, mailbox, id)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		`UPDATE mailboxes SET uid_next = uid_next + 1 WHERE id = ?`, destID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// --- Queue ---
func (s *DB) QueueListDestinations() ([]string, error) {
    rows, err := s.db.Query(`SELECT DISTINCT destination FROM queue`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var dests []string
    for rows.Next() {
        var d string
        if err := rows.Scan(&d); err != nil {
            return nil, err
        }
        dests = append(dests, d)
    }
    return dests, rows.Err()
}

func (s *DB) QueueMailIDsForDestination(dest string) ([]types.QueuedMail, error) {
    rows, err := s.db.Query(`
        SELECT message_id, from_addr, rcpt_addr 
        FROM queue WHERE destination = ?`, dest)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var mails []types.QueuedMail
    for rows.Next() {
        var m types.QueuedMail
        if err := rows.Scan(&m.ID, &m.From, &m.Rcpt); err != nil {
            return nil, err
        }
        mails = append(mails, m)
    }
    return mails, rows.Err()
}

func (s *DB) QueueInsertDestinationForID(dest string, id int, from, rcpt string) error {
    _, err := s.db.Exec(`
        INSERT INTO queue (destination, message_id, from_addr, rcpt_addr) 
        VALUES (?, ?, ?, ?)`,
        dest, id, from, rcpt)
    return err
}

func (s *DB) QueueDeleteDestinationForID(dest string, id int) error {
    _, err := s.db.Exec(`
        DELETE FROM queue WHERE destination = ? AND message_id = ?`,
        dest, id)
    return err
}

func (s *DB) QueueSelectIsMessagePendingSend(mailbox string, id int) (bool, error) {
    var count int
    err := s.db.QueryRow(`
        SELECT COUNT(*) FROM queue WHERE message_id = ?`, id).Scan(&count)
    return count > 0, err
}

// DeliverToUser consegna un messaggio direttamente all'INBOX di un utente locale.
func (s *DB) DeliverToUser(address string, content []byte) error {
    var mbID, nextUID int
    err := s.db.QueryRow(`
        SELECT mb.id, mb.uid_next 
        FROM mailboxes mb
        JOIN users u ON mb.user_id = u.id
        WHERE u.address = ? AND mb.name = 'INBOX'`, address).Scan(&mbID, &nextUID)
    if err != nil {
        return fmt.Errorf("mailbox non trovata per %s: %w", address, err)
    }
    _, err = s.db.Exec(
        `INSERT INTO messages (mailbox_id, uid, body, size) VALUES (?, ?, ?, ?)`,
        mbID, nextUID, content, len(content),
    )
    if err != nil {
        return err
    }
    _, err = s.db.Exec(
        `UPDATE mailboxes SET uid_next = uid_next + 1 WHERE id = ?`, mbID,
    )
    return err
}

func (s *DB) DeliverToSent(address string, content []byte) error {
    // Assicura che la cartella Sent esista
    s.db.Exec(`
        INSERT IGNORE INTO mailboxes (user_id, name)
        SELECT id, 'Sent' FROM users WHERE address = ?`, address)

    var mbID, nextUID int
    err := s.db.QueryRow(`
        SELECT mb.id, mb.uid_next
        FROM mailboxes mb
        JOIN users u ON mb.user_id = u.id
        WHERE u.address = ? AND mb.name = 'Sent'`, address).Scan(&mbID, &nextUID)
    if err != nil {
        return fmt.Errorf("mailbox Sent non trovata per %s: %w", address, err)
    }
    _, err = s.db.Exec(
        `INSERT INTO messages (mailbox_id, uid, body, size, flags)
         VALUES (?, ?, ?, ?, '\\Seen')`,
        mbID, nextUID, content, len(content))
    if err != nil {
        return err
    }
    _, err = s.db.Exec(
        `UPDATE mailboxes SET uid_next = uid_next + 1 WHERE id = ?`, mbID)
    return err
}

// ResolveAlias cerca a quale indirizzo reale punta un alias (es. hex@yggmail -> alice@200:...)
func (s *DB) ResolveAlias(alias string) (string, error) {
	var targetAddress string
	err := s.db.QueryRow(`
		SELECT u.address FROM users u
		JOIN aliases a ON u.id = a.user_id
		WHERE a.alias = ?`, alias).Scan(&targetAddress)
	if err != nil {
		return "", err // Restituisce sql.ErrNoRows se non trovato
	}
	return targetAddress, nil
}

// AssignYggmailAlias assegna l'alias speciale hex@yggmail a un utente, rimuovendolo da altri
func (s *DB) AssignYggmailAlias(address string, yggAlias string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Rimuoviamo il flag is_Glasir-MTA da chiunque lo abbia
	_, err = tx.Exec(`UPDATE aliases SET is_Glasir = 0 WHERE is_Glasir = 1`)
	if err != nil {
		return err
	}

	// 2. Eliminiamo l'alias Glasir-MTA esistente se presente
	_, err = tx.Exec(`DELETE FROM aliases WHERE alias = ?`, yggAlias)
	if err != nil {
		return err
	}

	// 3. Inseriamo il nuovo alias per l'utente target
	_, err = tx.Exec(`
		INSERT INTO aliases (alias, user_id, is_Glasir)
		SELECT ?, id, 1 FROM users WHERE address = ?`, yggAlias, address)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// YggmailAliasOwner restituisce l'utente che attualmente possiede l'alias del nodo
func (s *DB) YggmailAliasOwner() (string, error) {
	var addr string
	err := s.db.QueryRow(`
		SELECT u.address FROM users u
		JOIN aliases a ON u.id = a.user_id
		WHERE a.is_Glasir = 1 LIMIT 1`).Scan(&addr)
	return addr, err
}

func (s *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			address VARCHAR(320) NOT NULL UNIQUE,
			passhash VARCHAR(255) NOT NULL,
			quota_bytes BIGINT UNSIGNED DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS aliases (
			id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			alias VARCHAR(320) NOT NULL UNIQUE,
			user_id BIGINT UNSIGNED NOT NULL,
			is_Glasir BOOLEAN NOT NULL DEFAULT 0,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS mailboxes (
			id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			user_id BIGINT UNSIGNED NOT NULL,
			name VARCHAR(255) NOT NULL,
			subscribed BOOLEAN DEFAULT 1,
			UNIQUE(user_id, name),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		// Aggiungi qui le altre tabelle (messages, etc.)
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migrazione fallita: %w", err)
		}
	}
	return nil
}

// ── Strutture ──────────────────────────────────────────────────────────────

// AliasInfo descrive un alias con il suo destinatario reale.
type AliasInfo struct {
	Alias     string
	Target    string
	IsYggmail bool
}

// UserInfo descrive un utente per la listusers.
type UserInfo struct {
	Address    string
	QuotaBytes int64
	CreatedAt  string
}

// ── Metodi Utenti ──────────────────────────────────────────────────────────

// UserExists controlla se un utente esiste già nel DB.
func (s *DB) UserExists(address string) (bool, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM users WHERE address = ?`, address).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// UserCreate crea un nuovo utente con hash password e quota.
func (s *DB) UserCreate(address, passwordHash string, quotaBytes int64) error {
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    res, err := tx.Exec(
        `INSERT INTO users (address, passhash, quota_bytes) VALUES (?, ?, ?)`,
        address, passwordHash, quotaBytes,
    )
    if err != nil {
        return err
    }
    userID, _ := res.LastInsertId()

    // Crea INBOX di default
    _, err = tx.Exec(
        `INSERT INTO mailboxes (user_id, name) VALUES (?, 'INBOX')`, userID,
    )
    if err != nil {
        return err
    }
    return tx.Commit()
}

// UserDelete elimina un utente (e in cascata mailbox, messaggi, alias).
func (s *DB) UserDelete(address string) error {
	res, err := s.db.Exec(`DELETE FROM users WHERE address = ?`, address)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("utente non trovato: %s", address)
	}
	return nil
}

// UserUpdatePassword aggiorna l'hash della password di un utente.
func (s *DB) UserUpdatePassword(address, passwordHash string) error {
	res, err := s.db.Exec(
		`UPDATE users SET passhash = ? WHERE address = ?`,
		passwordHash, address,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("utente non trovato: %s", address)
	}
	return nil
}

// UserAuthenticate verifica indirizzo e password bcrypt nella tabella users.
func (s *DB) UserAuthenticate(address, password string) error {
    var hash []byte
    err := s.db.QueryRow(
        `SELECT passhash FROM users WHERE address = ?`, address,
    ).Scan(&hash)
    if err != nil {
        log.Printf("DEBUG UserAuthenticate: user %q not found: %v", address, err)
        return fmt.Errorf("credenziali non valide")
    }
    log.Printf("DEBUG UserAuthenticate: hash=%q len=%d", hash[:10], len(hash))
    if err := bcrypt.CompareHashAndPassword(hash, []byte(password)); err != nil {
        log.Printf("DEBUG UserAuthenticate: bcrypt mismatch: %v", err)
        return fmt.Errorf("credenziali non valide")
    }
    
    return nil
}

// UserSetQuota imposta la quota in bytes per un utente (0 = illimitata).
func (s *DB) UserSetQuota(address string, quotaBytes int64) error {
	_, err := s.db.Exec(
		`UPDATE users SET quota_bytes = ? WHERE address = ?`,
		quotaBytes, address,
	)
	return err
}

// UserQuotaInfo restituisce (bytesUsati, quotaMax, errore) per un utente.
func (s *DB) UserQuotaInfo(address string) (int64, int64, error) {
	var quotaBytes int64
	var userID int64
	err := s.db.QueryRow(
		`SELECT id, quota_bytes FROM users WHERE address = ?`, address,
	).Scan(&userID, &quotaBytes)
	if err != nil {
		return 0, 0, err
	}

	var used int64
	err = s.db.QueryRow(`
		SELECT COALESCE(SUM(m.size), 0)
		FROM messages m
		JOIN mailboxes mb ON m.mailbox_id = mb.id
		WHERE mb.user_id = ?`, userID,
	).Scan(&used)
	return used, quotaBytes, err
}

// ListUsers restituisce tutti gli utenti registrati.
func (s *DB) ListUsers() ([]UserInfo, error) {
	rows, err := s.db.Query(`
		SELECT address, quota_bytes, DATE_FORMAT(created_at, '%Y-%m-%d') 
		FROM users ORDER BY address`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []UserInfo
	for rows.Next() {
		var u UserInfo
		if err := rows.Scan(&u.Address, &u.QuotaBytes, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// ── Metodi Alias ───────────────────────────────────────────────────────────

// ListAliases restituisce tutti gli alias, opzionalmente filtrati per utente.
func (s *DB) ListAliases(filterAddress string) ([]AliasInfo, error) {
	query := `
		SELECT a.alias, u.address, a.is_Glasir
		FROM aliases a
		JOIN users u ON a.user_id = u.id`
	args := []interface{}{}

	if filterAddress != "" {
		query += ` WHERE u.address = ?`
		args = append(args, filterAddress)
	}
	query += ` ORDER BY a.alias`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var aliases []AliasInfo
	for rows.Next() {
		var a AliasInfo
		if err := rows.Scan(&a.Alias, &a.Target, &a.IsYggmail); err != nil {
			return nil, err
		}
		aliases = append(aliases, a)
	}
	return aliases, rows.Err()
}

// AddAlias crea un alias generico che punta a un utente esistente.
func (s *DB) AddAlias(alias, targetAddress string) error {
	_, err := s.db.Exec(`
		INSERT INTO aliases (alias, user_id, is_Glasir)
		SELECT ?, id, 0 FROM users WHERE address = ?`,
		alias, targetAddress,
	)
	return err
}

// DeleteAlias rimuove un alias dal DB.
func (s *DB) DeleteAlias(alias string) error {
	res, err := s.db.Exec(`DELETE FROM aliases WHERE alias = ?`, alias)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("alias non trovato: %s", alias)
	}
	return nil
}

// Ping verifica che la connessione al DB sia attiva.
func (s *DB) Ping() error {
	return s.db.Ping()
}
 
// CountAllMessages conta tutti i messaggi nel sistema (esclusa Outbox di system).
func (s *DB) CountAllMessages() (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM messages m
		JOIN mailboxes mb ON m.mailbox_id = mb.id
		JOIN users u ON mb.user_id = u.id
		WHERE u.address != 'system'`).Scan(&count)
	return count, err
}
 
// CountQueue conta i messaggi attualmente in coda di invio.
func (s *DB) CountQueue() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM queue`).Scan(&count)
	return count, err
}
 
