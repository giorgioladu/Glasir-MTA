/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package imapserver

import (
	"errors"
	"fmt"
	"log"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
)

type User struct {
	backend  *Backend
	username string
	conn     *imap.ConnInfo
	log      *log.Logger
}

func (u *User) Username() string {
	return u.username // ← restituisce il vero indirizzo utente
}

func (u *User) ListMailboxes(subscribed bool) ([]backend.Mailbox, error) {
	names, err := u.backend.Storage.MailboxList(u.username, subscribed)
	if err != nil {
		return nil, err
	}
	var mailboxes []backend.Mailbox
	for _, name := range names {
		mailboxes = append(mailboxes, &Mailbox{
			backend: u.backend,
			user:    u,
			name:    name,
		})
	}
	return mailboxes, nil
}

func (u *User) GetMailbox(name string) (backend.Mailbox, error) {
	if name == "" {
		return &Mailbox{backend: u.backend, user: u, name: ""}, nil
	}
	ok, _ := u.backend.Storage.MailboxSelect(u.username, name)
	if !ok {
		return nil, fmt.Errorf("mailbox %q not found", name)
	}
	return &Mailbox{backend: u.backend, user: u, name: name}, nil
}

func (u *User) CreateMailbox(name string) error {
	u.log.Printf("Creating mailbox '%s' for %s\n", name, u.username)
	if err := u.backend.Storage.MailboxCreate(u.username, name); err != nil {
		u.log.Printf("Error creating mailbox '%s': %v\n", name, err)
		return err
	}
	u.log.Printf("Created mailbox '%s'\n", name)
	return nil
}

func (u *User) DeleteMailbox(name string) error {
	switch name {
	case "INBOX", "Outbox", "Sent":
		return errors.New("Cannot delete " + name)
	default:
		if err := u.backend.Storage.MailboxDelete(u.username, name); err != nil {
			u.log.Printf("Error deleting mailbox '%s': %v\n", name, err)
			return err
		}
		u.log.Printf("Deleted mailbox '%s'\n", name)
		return nil
	}
}

func (u *User) RenameMailbox(existingName, newName string) error {
	switch existingName {
	case "INBOX", "Outbox", "Sent":
		return errors.New("Cannot rename " + existingName)
	default:
		return u.backend.Storage.MailboxRename(u.username, existingName, newName)
	}
}

func (u *User) Logout() error {
	return nil
}
