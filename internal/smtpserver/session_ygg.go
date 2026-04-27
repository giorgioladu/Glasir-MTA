/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 
  * Glasir-MTA — SessionYgg
 * Gestisce le connessioni SMTP in arrivo da altri nodi Yggdrasil.
 * Implementa TLS obbligatorio e verifica PoW per ogni destinatario.
 */

package smtpserver

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/emersion/go-message"
	"github.com/emersion/go-smtp"
	"Glasir-MTA/internal/stamp" 
	"Glasir-MTA/internal/yggtls"
)

type SessionYgg struct {
	backend *Backend
	state   *smtp.ConnectionState
	from    string
	rcpts   []string
}

// Mail gestisce il comando MAIL FROM: e verifica l'identità TLS del mittente.
func (s *SessionYgg) Mail(from string, opts smtp.MailOptions) error {
	// 1. Controllo TLS Obbligatorio
	if s.state.TLS.HandshakeComplete == false {
		return fmt.Errorf("530 StartTLS richiesto su rete Yggdrasil")
	}

	// 2. Recupero del certificato del mittente
	peerCerts := s.state.TLS.PeerCertificates
	if len(peerCerts) == 0 {
		return fmt.Errorf("530 Identità non fornita (Certificato TLS mancante)")
	}

	clientCert := peerCerts[0]
	remoteAddr, ok := s.state.RemoteAddr.(*net.TCPAddr)
	if !ok {
		return fmt.Errorf("421 Errore interno: impossibile determinare IP remoto")
	}

	// 3. Validazione Incrociata: IP Yggdrasil <-> Chiave Pubblica TLS
	pubKey, ok := clientCert.PublicKey.(ed25519.PublicKey)
	if !ok {
		return fmt.Errorf("530 Algoritmo chiave non supportato (richiesto Ed25519)")
	}

	// Chiamata alla funzione di validazione definita in crypto_ygg.go
	if err := yggtls.VerifyYggdrasilIdentity(remoteAddr.IP, pubKey); err != nil {
		s.backend.Log.Printf("SPOOFING ALERT: %s ha fornito una chiave non valida: %v", remoteAddr.IP, err)
		return fmt.Errorf("530 Identità contraffatta: il tuo IP non corrisponde alla tua chiave TLS")
	}

	s.from = from
	s.rcpts = s.rcpts[:0]
	s.backend.Log.Printf("Ygg: Sessione verificata per %s (%s)", from, remoteAddr.IP)
	return nil
}

func (s *SessionYgg) Rcpt(to string) error {
	// Accetta il destinatario se ben formato
	if !strings.Contains(to, "@") {
		return fmt.Errorf("indirizzo non valido: %s", to)
	}

	s.rcpts = append(s.rcpts, to)
	return nil
}

// Data riceve il contenuto della mail e verifica i francobolli PoW.
func (s *SessionYgg) Data(r io.Reader) error {
	// 1. Lettura del messaggio
	m, err := message.Read(r)
	if err != nil {
		return fmt.Errorf("message.Read: %w", err)
	}

	// 2. Recupero e verifica dei francobolli PoW (X-Glasir-Stamp)
	stamps := m.Header.Values(stamp.Header)
	validatedRcpts := make(map[string]bool)
	
	s.backend.Log.Printf("DEBUG PoW: Ricevuti %d francobolli", len(stamps))

	// Funzione per pulire stringhe da spazi, parentesi e portarle in minuscolo
	clean := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.ReplaceAll(s, "[", "")
		s = strings.ReplaceAll(s, "]", "")
		return s
	}

	for _, token := range stamps {
		token = strings.TrimSpace(token)
		
		if err := stamp.Verify(token); err == nil {
			parts := strings.Split(token, ";")
			if len(parts) >= 4 {
				// Il destinatario scritto dentro il francobollo
				tokenRecipient := clean(parts[3])
				
				for _, r := range s.rcpts {
					if clean(r) == tokenRecipient {
						validatedRcpts[r] = true
						s.backend.Log.Printf("Ygg: PoW verificata per %s", r)
					}
				}
			}
		} else {
			s.backend.Log.Printf("Ygg: Stamp crittograficamente NON valido: %v", err)
		}
	}

	// 3. Controllo finale: ogni destinatario deve avere un pagamento valido
	for _, rcpt := range s.rcpts {
		if !validatedRcpts[rcpt] {
			s.backend.Log.Printf("Ygg: PoW fallita per %s", rcpt)
			return fmt.Errorf("550 PoW (Stamp) valida richiesta per ogni destinatario: %s", rcpt)
		}
	}

	// 4. Aggiunta header di sistema
	m.Header.Add("Received", fmt.Sprintf(
		"from %s via Yggdrasil; %s",
		s.state.RemoteAddr.String(),
		time.Now().Format(time.RFC1123Z),
	))
	m.Header.Add("Delivery-Date", time.Now().UTC().Format(time.RFC822))

	// 5. Preparazione buffer per la consegna
	var b bytes.Buffer
	if err := m.WriteTo(&b); err != nil {
		return fmt.Errorf("m.WriteTo: %w", err)
	}
	msgBytes := b.Bytes()

	// 6. Consegna finale (diretta o tramite alias)
	for _, rcpt := range s.rcpts {
		delivered := false

		// Prova consegna diretta
		if err := s.backend.Storage.DeliverToUser(rcpt, msgBytes); err == nil {
			delivered = true
		} else if alias, err := s.backend.Storage.ResolveAlias(rcpt); err == nil {
			// Prova via alias
			if err := s.backend.Storage.DeliverToUser(alias, msgBytes); err == nil {
				delivered = true
			}
		}

		if delivered {
			s.backend.Log.Printf("Ygg: mail da %s consegnata a %s", s.from, rcpt)
		} else {
			s.backend.Log.Printf("Ygg: impossibile consegnare a %s", rcpt)
			return fmt.Errorf("451 Errore consegna per %s", rcpt)
		}
	}

	return nil
}

func (s *SessionYgg) Reset() {
	s.from = ""
	s.rcpts = s.rcpts[:0]
}

func (s *SessionYgg) Logout() error {
	return nil
}
