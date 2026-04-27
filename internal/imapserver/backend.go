/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 *
 * 
 *  Autenticazione multi-utente con bcrypt (tabella users)
 */

package imapserver

import (
	"log"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	"Glasir-MTA/internal/config"
	"Glasir-MTA/internal/storage"
)

type Backend struct {
	Config  *config.Config
	Log     *log.Logger
	Storage storage.Storage
	Server  *IMAPServer
}

func (b *Backend) Login(conn *imap.ConnInfo, username, password string) (backend.User, error) {
	// Autentica l'utente controllando address + bcrypt hash nella tabella users
	if err := b.Storage.UserAuthenticate(username, password); err != nil {
		b.Log.Printf("IMAP: autenticazione fallita per %q: %s", username, err)
		return nil, backend.ErrInvalidCredentials
	}

	b.Log.Printf("IMAP: autenticato %q da %s", username, conn.RemoteAddr.String())

	return &User{
		backend:  b,
		username: username,
		conn:     conn,
		log:      b.Log,
	}, nil
}
