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
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/emersion/go-message"
	"github.com/emersion/go-smtp"
	"Glasir-MTA/internal/stamp"
)

type SessionLocal struct {
	backend *Backend
	state   *smtp.ConnectionState
	from    string
	rcpt    []string
}

func (s *SessionLocal) Mail(from string, opts smtp.MailOptions) error {
    s.rcpt = s.rcpt[:0]
    s.from = from
    return nil
}

func (s *SessionLocal) Rcpt(to string) error {
	s.rcpt = append(s.rcpt, to)
	return nil
}

func (s *SessionLocal) Data(r io.Reader) error {
	m, err := message.Read(r)
	if err != nil {
		return fmt.Errorf("message.Read: %w", err)
	}
	
	// --- AGGIUNTA LOGICA POW (GENERAZIONE) ---
	// Generiamo un francobollo per ogni destinatario.
	// Nota: stamp.Generate blocca la goroutine per ~0.5s.
	for _, rcpt := range s.rcpt {
		token, err := stamp.Generate(rcpt)
		if err != nil {
			s.backend.Log.Printf("Errore generazione stamp per %s: %v", rcpt, err)
			continue
		}
		m.Header.Add(stamp.Header, token)
	}
	// ------------------------------------------
	
	remoteAddr := "local"
	if s.state != nil {
		remoteAddr = s.state.RemoteAddr.String()
	}

	m.Header.Add(
		"Received", fmt.Sprintf("from %s by Yggmail %s; %s",
			remoteAddr,
			hex.EncodeToString(s.backend.Config.PublicKey),
			time.Now().String(),
		),
	)
	if !m.Header.Has("Date") {
		m.Header.Add(
			"Date", time.Now().UTC().Format(time.RFC822),
		)
	}

	var b bytes.Buffer
	if err := m.WriteTo(&b); err != nil {
		return fmt.Errorf("m.WriteTo: %w", err)
	}

	if err := s.backend.Queues.QueueFor(s.from, s.rcpt, b.Bytes()); err != nil {
		return fmt.Errorf("s.backend.Queues.QueueFor: %w", err)
	}

	s.backend.Log.Println("Queued mail (with PoW) for", s.rcpt)
	return nil
}

func (s *SessionLocal) Reset() {
	s.rcpt = s.rcpt[:0]
	s.from = ""
}

func (s *SessionLocal) Logout() error {
	return nil
}
