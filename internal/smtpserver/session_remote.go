/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package smtpserver

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/emersion/go-message"
	"github.com/emersion/go-smtp"
	"Glasir-MTA/internal/utils"
)

type SessionRemote struct {
	backend *Backend
	state   *smtp.ConnectionState
	public  ed25519.PublicKey
	from    string
}

func (s *SessionRemote) Mail(from string, opts smtp.MailOptions) error {
	pk, err := utils.ParseAddress(from)
	if err != nil {
		return fmt.Errorf("mail.ParseAddress: %w", err)
	}
	if remote := s.state.RemoteAddr.String(); hex.EncodeToString(pk) != remote {
		return fmt.Errorf("not allowed to send incoming mail as %s", from)
	}
	s.from = from
	return nil
}

func (s *SessionRemote) Rcpt(to string) error {
	pk, err := utils.ParseAddress(to)
	if err != nil {
		return fmt.Errorf("mail.ParseAddress: %w", err)
	}
	if !pk.Equal(s.backend.Config.PublicKey) {
		return fmt.Errorf("unexpected recipient for wrong domain")
	}
	return nil
}

func (s *SessionRemote) Data(r io.Reader) error {
	m, err := message.Read(r)
	if err != nil {
		return fmt.Errorf("message.Read: %w", err)
	}

	m.Header.Add(
		"Received", fmt.Sprintf("from Yggmail %s; %s",
			hex.EncodeToString(s.public),
			time.Now().String(),
		),
	)
	m.Header.Add("Delivery-Date", time.Now().UTC().Format(time.RFC822))

	var b bytes.Buffer
	if err := m.WriteTo(&b); err != nil {
		return fmt.Errorf("m.WriteTo: %w", err)
	}

	// Trova il proprietario dell'alias Glasir-MTA (hex@yggmail)
	yggAlias := hex.EncodeToString(s.backend.Config.PublicKey) + "@yggmail"
	owner, err := s.backend.Storage.ResolveAlias(yggAlias)
	if err != nil {
		s.backend.Log.Printf("Alias %s non trovato: %v", yggAlias, err)
		return fmt.Errorf("nessun utente associato all'alias yggmail")
	}

	id, err := s.backend.Storage.MailCreate(owner, "INBOX", b.Bytes())
	if err != nil {
		return fmt.Errorf("MailCreate: %w", err)
	}
	s.backend.Log.Printf("Mail da %s consegnata a %s", s.from, owner)

	if count, err := s.backend.Storage.MailCount(owner, "INBOX"); err == nil {
		if err := s.backend.Notify.NotifyNew(id, count); err != nil {
			s.backend.Log.Println("Failed to notify:", s.from)
		}
	}

	return nil
}

func (s *SessionRemote) Reset() {}

func (s *SessionRemote) Logout() error {
	return nil
}
