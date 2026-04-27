/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */
/*
 *  Glasir-MTA — SMTP Sender
 *  Consegna locale via DeliverToUser.
 *  Consegna remota via TCP diretto su [ygg_ip]:25.
 *  Supporta sia user@200:... che hex@yggmail (convertito automaticamente in IPv6).
 */

package smtpsender

import (
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"
	"crypto/ed25519"
	
	"bytes"
	"crypto/tls"
	"github.com/emersion/go-message" 
	
	"go.uber.org/atomic"
	"github.com/yggdrasil-network/yggdrasil-go/src/address"
	"Glasir-MTA/internal/config"
	"Glasir-MTA/internal/storage"
	"Glasir-MTA/internal/storage/types"
	"Glasir-MTA/internal/stamp"
	"Glasir-MTA/internal/yggtls"

)

const systemUser  = "system"
const yggSMTPPort = "25"

// ── Helpers ───────────────────────────────────────────────────────────────────

// yggKeyToIPv6 converte una chiave pubblica Yggdrasil (hex 64 char)
// nel corrispondente indirizzo IPv6 (200::/7) in modo deterministico.
func yggKeyToIPv6(hexKey string) (string, error) {
	keyBytes, err := hex.DecodeString(hexKey)
	if err != nil {
		return "", fmt.Errorf("hex decode: %w", err)
	}
	if len(keyBytes) != 32 {
		return "", fmt.Errorf("chiave deve essere 32 byte, trovati %d", len(keyBytes))
	}
	
	pubKey := ed25519.PublicKey(keyBytes)
	addr := address.AddrForKey(pubKey)
	ip := net.IP(addr[:])
	return ip.String(), nil
}

// ── Queues ────────────────────────────────────────────────────────────────────

type Queues struct {
	Config  *config.Config
	Log     *log.Logger
	Storage storage.Storage
	queues  sync.Map // ipv6 -> *Queue
}

func NewQueues(cfg *config.Config, log *log.Logger, storage storage.Storage) *Queues {
	qs := &Queues{
		Config:  cfg,
		Log:     log,
		Storage: storage,
	}
	time.AfterFunc(5*time.Second, qs.manager)
	return qs
}

// manager rilancia periodicamente le code pendenti nel DB.
func (qs *Queues) manager() {
	destinations, err := qs.Storage.QueueListDestinations()
	if err == nil {
		for _, dest := range destinations {
			_, _ = qs.queueFor(dest)
		}
	}
	time.AfterFunc(time.Minute, qs.manager)
}

// QueueFor instrada la mail verso i destinatari:
//   - Locale  : consegna diretta in INBOX via DeliverToUser
//   - Remoto  : TCP su [ipv6]:25 — supporta user@ipv6 e hex@yggmail
func (qs *Queues) QueueFor(from string, rcpts []string, content []byte) error {
	for _, rcpt := range rcpts {
		parts := strings.SplitN(rcpt, "@", 2)
		if len(parts) != 2 {
			qs.Log.Printf("Indirizzo non valido ignorato: %s", rcpt)
			continue
		}
		host := parts[1]

		// 1. Prova consegna locale
		if err := qs.Storage.DeliverToUser(rcpt, content); err == nil {
			qs.Log.Printf("Delivered locally to %s", rcpt)
			continue
		}

		// 2. Risoluzione IPv6
		var destIPv6 string
		if host == "yggmail" {
			ipv6, err := yggKeyToIPv6(parts[0])
			if err != nil {
				qs.Log.Printf("Chiave hex non valida: %v", err)
				continue
			}
			destIPv6 = ipv6
		} else {
			destIPv6 = host
		}

		// 3. INIEZIONE PULITA DEL FRANCOBOLLO PoW
		// Leggiamo il messaggio originale come entità MIME
		msg, err := message.Read(bytes.NewReader(content))
		if err != nil {
			qs.Log.Printf("Errore lettura messaggio per PoW: %v", err)
			continue
		}

		// Generiamo il francobollo per questo specifico destinatario
		powStamp, err := stamp.Generate(rcpt)
		if err != nil {
			qs.Log.Printf("Errore generazione PoW: %v", err)
			continue
		}

		// Aggiungiamo l'header correttamente
		msg.Header.Set(stamp.Header, powStamp)

		// Ricostruiamo il buffer del messaggio
		var b bytes.Buffer
		if err := msg.WriteTo(&b); err != nil {
			qs.Log.Printf("Errore scrittura messaggio con PoW: %v", err)
			continue
		}
		finalContent := b.Bytes()

		// 4. Messa in coda del messaggio modificato
		pid, err := qs.Storage.MailCreate(systemUser, "Outbox", finalContent)
		if err != nil {
			return fmt.Errorf("MailCreate: %w", err)
		}
		if err := qs.Storage.QueueInsertDestinationForID(destIPv6, pid, from, rcpt); err != nil {
			return fmt.Errorf("QueueInsertDestinationForID: %w", err)
		}
		_, _ = qs.queueFor(destIPv6)
	}
	return nil
}

func (qs *Queues) queueFor(ipv6 string) (*Queue, error) {
	v, _ := qs.queues.LoadOrStore(ipv6, &Queue{
		queues:      qs,
		destination: ipv6,
	})
	q, ok := v.(*Queue)
	if !ok {
		return nil, fmt.Errorf("type assertion error")
	}
	if q.running.CompareAndSwap(false, true) {
		go q.run()
	}
	return q, nil
}

// ── Queue ─────────────────────────────────────────────────────────────────────

type Queue struct {
	queues      *Queues
	destination string // IPv6 del nodo remoto
	running     atomic.Bool
}

const maxRetries    = 10
const retryInterval = 120 * time.Second

func (q *Queue) run() {
	defer q.running.Store(false)

	refs, err := q.queues.Storage.QueueMailIDsForDestination(q.destination)
	if err != nil {
		q.queues.Log.Println("Queue error:", err)
		return
	}
	defer q.queues.Storage.MailExpunge(systemUser, "Outbox")

	for _, ref := range refs {
		_, mail, err := q.queues.Storage.MailSelect(systemUser, "Outbox", ref.ID)
		if err != nil {
			q.queues.Log.Printf("Cannot read mail %d: %v", ref.ID, err)
			continue
		}

		// Tenta l'invio con retry
		var sent bool
		for attempt := 1; attempt <= maxRetries; attempt++ {
			q.queues.Log.Printf("Invio mail %d a [%s]:%s (tentativo %d/%d)",
				ref.ID, q.destination, yggSMTPPort, attempt, maxRetries)

			if err := q.sendViaIPv6(ref, mail); err != nil {
				q.queues.Log.Printf("Tentativo %d fallito per %s: %v", attempt, q.destination, err)
				if attempt < maxRetries {
					time.Sleep(retryInterval)
				}
				continue
			}
			sent = true
			break
		}

		if !sent {
			q.queues.Log.Printf("Mail %d abbandonata dopo %d tentativi verso %s",
				ref.ID, maxRetries, q.destination)
			continue
		}

		q.queues.Log.Printf("Mail %d consegnata a %s", ref.ID, ref.Rcpt)

		// Pulizia coda
		if err := q.queues.Storage.QueueDeleteDestinationForID(q.destination, ref.ID); err != nil {
			q.queues.Log.Printf("Errore rimozione coda mail %d: %v", ref.ID, err)
		}

		// Sposta in Sent se non ci sono altre destinazioni pendenti
		pending, err := q.queues.Storage.QueueSelectIsMessagePendingSend(systemUser, ref.ID)
		if err == nil && !pending {
			q.queues.Log.Printf("Spostamento mail %d in Sent", ref.ID)
			if err := q.queues.Storage.DeliverToSent(ref.From, mail.Mail); err != nil {
				q.queues.Log.Printf("Errore salvataggio in Sent per %s: %v", ref.From, err)
			}
			// Poi elimina dall'Outbox di system
			_ = q.queues.Storage.MailDelete(systemUser, "Outbox", ref.ID)
		}
	}
}

func (q *Queue) sendViaIPv6(ref types.QueuedMail, mail *types.Mail) error {
	addr := net.JoinHostPort(q.destination, yggSMTPPort)
	
	// Genera certificato client per identificarci
	myCert, err := yggtls.GenerateYggCertificate(q.queues.Config.PrivateKey)
	if err != nil {
		return fmt.Errorf("TLS cert error: %w", err)
	}

	conn, err := net.DialTimeout("tcp6", addr, 15*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Usa emersion/go-smtp.NewClient se possibile, o net/smtp
	client, err := smtp.NewClient(conn, q.destination)
	if err != nil {
		return err
	}
	defer client.Quit()

	// STARTTLS obbligatorio
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{myCert},
		InsecureSkipVerify: true,
		ServerName: "yggmail",
	}
	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("STARTTLS: %w", err)
	}

	// 3. Comandi SMTP standard (ora criptati e identificati)
	if err := client.Mail(ref.From); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	if err := client.Rcpt(ref.Rcpt); err != nil {
		return fmt.Errorf("RCPT TO: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(mail.Mail); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return w.Close()
}


