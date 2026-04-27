/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */
 
package storage

import (
	"fmt"
	"time"

	"Glasir-MTA/internal/storage/types"
)

type ErrQuotaExceeded struct {
	Used      int64
	Quota     int64
	Attempted int64
}

func (e *ErrQuotaExceeded) Error() string {
	return fmt.Sprintf(
		"quota exceeded: used %s of %s, message is %s",
		FormatBytes(e.Used), FormatBytes(e.Quota), FormatBytes(e.Attempted),
	)
}

type ErrAliasNotFound struct{ Alias string }

func (e *ErrAliasNotFound) Error() string {
	return fmt.Sprintf("alias %q not found", e.Alias)
}

func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

type Message struct {
	UID      uint32
	Folder   string
	Flags    []string
	Size     int
	Body     []byte
	Received time.Time
}

type Mailbox struct {
	Name        string
	UIDValidity uint32
	UIDNext     uint32
	Messages    int
}

type UserInfo struct {
	Address    string
	CreatedAt  time.Time
	QuotaBytes int64
	UsedBytes  int64
	Aliases    []string
}

func (u *UserInfo) QuotaPercent() float64 {
	if u.QuotaBytes == 0 {
		return 0
	}
	return float64(u.UsedBytes) / float64(u.QuotaBytes) * 100
}

func (u *UserInfo) QuotaUnlimited() bool { return u.QuotaBytes == 0 }

type AliasInfo struct {
	Alias     string
	Target    string
	IsYggmail bool
}

type Storage interface {
	// Auth & Config (legacy, mantenuti per compatibilità)
	ConfigGet(key string) (string, error)
	ConfigSet(key, value string) error
	ConfigSetPassword(password string) error
	ConfigTryPassword(password string) (bool, error)

	// Autenticazione multi-utente
	UserAuthenticate(address, password string) error

	// Mailbox — tutti richiedono username per scoping multi-utente
	MailboxSelect(username, mailbox string) (bool, error)
	MailNextID(username, mailbox string) (int, error)
	MailIDForSeq(username, mailbox string, id int) (int, error)
	MailUnseen(username, mailbox string) (int, error)
	MailboxList(username string, onlySubscribed bool) ([]string, error)
	MailboxCreate(username, name string) error
	MailboxRename(username, old, new string) error
	MailboxDelete(username, name string) error
	MailboxSubscribe(username, name string, subscribed bool) error

	// Mail
	MailCreate(username, mailbox string, data []byte) (int, error)
	MailSelect(username, mailbox string, id int) (int, *types.Mail, error)
	MailSearch(username, mailbox string) ([]uint32, error)
	MailUpdateFlags(username, mailbox string, id int, seen, answered, flagged, deleted bool) error
	MailDelete(username, mailbox string, id int) error
	MailExpunge(username, mailbox string) error
	MailCount(username, mailbox string) (int, error)
	MailMove(username, mailbox string, id int, destination string) error

	// Queue
	QueueListDestinations() ([]string, error)
	QueueMailIDsForDestination(destination string) ([]types.QueuedMail, error)
	QueueInsertDestinationForID(destination string, id int, from, rcpt string) error
	QueueDeleteDestinationForID(destination string, id int) error
	QueueSelectIsMessagePendingSend(mailbox string, id int) (bool, error)
	
	ResolveAlias(alias string) (string, error)
	DeliverToUser(address string, content []byte) error
	DeliverToSent(address string, content []byte) error
}
