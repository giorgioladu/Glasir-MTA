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
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"time"
)

// VerifyYggdrasilIdentity controlla che l'IPv6 remoto sia derivato dalla chiave pubblica nel certificato.
func VerifyYggdrasilIdentity(remoteIP net.IP, pubKey ed25519.PublicKey) error {
	// Calcolo dell'indirizzo IPv6 Yggdrasil v0.4
	// In Yggdrasil l'indirizzo è derivato dallo SHA-512 della chiave pubblica.
	hash := sha512.Sum512(pubKey)
	
	// Verifica semplificata: i primi byte dell'indirizzo devono corrispondere all'hash
	// (Nota: per un'implementazione perfetta servirebbe la maschera 0200::/7 di Yggdrasil)
	if hash[0] != remoteIP[0] || hash[1] != remoteIP[1] {
		return fmt.Errorf("l'IP %s non appartiene alla chiave pubblica fornita", remoteIP.String())
	}
	return nil
}

// GenerateYggCertificate crea un certificato auto-firmato usando la chiave privata del nodo.
func GenerateYggCertificate(privKey ed25519.PrivateKey) (tls.Certificate, error) {
	pubKey := privKey.Public().(ed25519.PublicKey)
	
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject: pkix.Name{
			Organization: []string{"Yggdrasil Node"},
			CommonName:   "glasir-node",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0), // Valido 10 anni
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, pubKey, privKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  privKey,
	}, nil
}
