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
	"crypto/tls"
	"fmt"
	"time"
	"crypto/x509"
	"crypto/ed25519"
	
	"github.com/emersion/go-smtp"
	"Glasir-MTA/internal/imapserver"
	"Glasir-MTA/internal/yggtls"
)

type SMTPServer struct {
	server  *smtp.Server
	backend *Backend
}

func NewSMTPServer(b *Backend, notify *imapserver.IMAPNotify) (*SMTPServer, error) {
	s := &SMTPServer{
		server:  smtp.NewServer(b),
		backend: b,
	}
	// 1. Generazione del certificato basato sulla chiave privata del nodo
	cert, err := yggtls.GenerateYggCertificate(b.Config.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("errore certificato TLS: %w", err)
	}

	// 2. Configurazione parametri server
	s.server.Addr = b.Config.SMTP.Listen
	s.server.Domain = b.Config.SMTP.Hostname
	s.server.MaxMessageBytes = int(b.Config.SMTP.MaxMessageBytes)
	s.server.ReadTimeout = time.Duration(b.Config.SMTP.ReadTimeout) * time.Second
	s.server.WriteTimeout = time.Duration(b.Config.SMTP.WriteTimeout) * time.Second
   
	// 3. AGGANCIO TLS CONFIG
	// Senza questo blocco, il server risponderà "502 TLS not supported"
	
	s.server.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		
		// Richiediamo il certificato al client (Fedora) per validarne l'identità Ygg
		ClientAuth: tls.RequireAnyClientCert, 
		
		// Saltiamo la verifica CA standard perché usiamo la rete Yggdrasil come garante
		InsecureSkipVerify: true,

		// Logica di verifica: controlliamo che chi ci parla sia chi dice di essere
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("nessun certificato presentato")
			}
			cert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return err
			}
			pub, ok := cert.PublicKey.(ed25519.PublicKey)
			if !ok {
				return fmt.Errorf("richiesta chiave Ed25519")
			}
			
			// Nota: la validazione IP <-> PubKey finale avviene in session_ygg.go
			// Qui verifichiamo solo che la chiave sia formalmente valida.
			b.Log.Printf("TLS: Handshake in corso con chiave %x", pub)
			return nil
		},
	}

	return s, nil
}

func (s *SMTPServer) GetInnerServer() *smtp.Server {
    return s.server
}

func (s *SMTPServer) ListenAndServe() error {
	s.backend.Log.Printf("Glasir SMTP in ascolto su %s (TLS Attivo)", s.server.Addr)
	return s.server.ListenAndServe()
}
