/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */

package yggtls

import (
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/big"
	"net"
	"time"

	"github.com/yggdrasil-network/yggdrasil-go/src/address"
)

// GenerateYggCertificate crea un certificato auto-firmato basato sulla chiave Yggdrasil.
func GenerateYggCertificate(priv ed25519.PrivateKey) (tls.Certificate, error) {
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	derBytes, err := x509.CreateCertificate(nil, &template, &template, priv.Public(), priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}, nil
}

// VerifyYggdrasilIdentity controlla che l'indirizzo IPv6 corrisponda alla chiave pubblica.
func VerifyYggdrasilIdentity(remoteIP net.IP, pub ed25519.PublicKey) error {
	expectedAddr := address.AddrForKey(pub)
	if !remoteIP.Equal(net.IP(expectedAddr[:])) {
		return fmt.Errorf("IP %s non corrisponde alla chiave pubblica fornita", remoteIP)
	}
	return nil
}
