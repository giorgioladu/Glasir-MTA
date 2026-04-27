/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */
/*
 *  Glasir-MTA — Stamp
 *  Francobollo digitale basato su Hashcash (SHA-256).
 *  Il mittente risolve un piccolo puzzle crittografico prima dell'invio.
 *  Trasparente per l'utente (~0.5s), insostenibile per uno spammer.
 *
 *  Formato header: X-Glasir-Stamp: 1:<bits>:<data>:<destinatario>:<nonce>:<counter>
 *  Esempio:        X-Glasir-Stamp: 1:20:20260426:bob@201:b8f9:...:a3f9c1:482910
 */

package stamp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	Bits = 20
	Header = "X-Glasir-Stamp"
	Version = "1"
)

func Generate(recipient string) (string, error) {
	date  := time.Now().UTC().Format("20060102")
	nonce, err := randomNonce()
	if err != nil {
		return "", fmt.Errorf("stamp.Generate: random nonce: %w", err)
	}

	//Usiamo la pipe ";" come separatore
	prefix := fmt.Sprintf("%s;%d;%s;%s;%s;", Version, Bits, date, recipient, nonce)
	target := targetPrefix(Bits)

	for counter := 0; ; counter++ {
		token := fmt.Sprintf("%s%d", prefix, counter)
		if hashHasPrefix(token, target) {
			return token, nil
		}
	}
}

func Verify(token string) error {
	//Split con la pipe "|"
	parts := strings.SplitN(token, ";", 6)
	if len(parts) != 6 {
		return fmt.Errorf("formato non valido: attesi 6 campi, trovati %d", len(parts))
	}

	version := parts[0]
	if version != Version {
		return fmt.Errorf("versione non supportata: %s", version)
	}

	date := parts[2]
	t, err := time.Parse("20060102", date)
	if err != nil {
		return fmt.Errorf("data non valida: %s", date)
	}
	if time.Since(t) > 48*time.Hour {
		return fmt.Errorf("stamp scaduto: %s", date)
	}

	target := targetPrefix(Bits)
	if !hashHasPrefix(token, target) {
		return fmt.Errorf("difficoltà non soddisfatta")
	}

	return nil
}

// GenerateTime stima il tempo medio di calcolo per un francobollo.
// Utile per il log di avvio.
func EstimateTime() time.Duration {
	// 2^Bits tentativi / ~500k hash/s su CPU moderna
	attempts := float64(int(1) << Bits)
	hashPerSec := float64(500_000)
	ms := (attempts / hashPerSec) * 1000
	return time.Duration(ms) * time.Millisecond
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// targetPrefix restituisce il prefisso hex che l'hash deve avere.
// Bits/4 caratteri hex = Bits bit di zeri iniziali (approssimazione per multipli di 4).
func targetPrefix(bits int) string {
	return strings.Repeat("0", bits/4)
}

func hashHasPrefix(token, prefix string) bool {
	sum := sha256.Sum256([]byte(token))
	h   := hex.EncodeToString(sum[:])
	return strings.HasPrefix(h, prefix)
}

func randomNonce() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
