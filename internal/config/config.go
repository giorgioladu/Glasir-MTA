/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */
package config

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

// Config è la struttura di configurazione principale di Glasir-MTA.
// La chiave privata viene usata solo per derivare l'identità del nodo (pubkey hex).
// Il networking Yggdrasil è gestito interamente dal demone di sistema.
type Config struct {
	Database DatabaseConfig `json:"database"`
	SMTP     SMTPConfig     `json:"smtp"`
	IMAP     IMAPConfig     `json:"imap"`

	// Chiave privata hex — usata solo per derivare la pubkey del nodo
	PrivateKeyHex string `json:"private_key"`

	// Calcolata a runtime da PrivateKeyHex, non nel JSON
	PrivateKey ed25519.PrivateKey `json:"-"` 
	PublicKey  ed25519.PublicKey  `json:"-"`
	
}

type DatabaseConfig struct {
	DSN          string `json:"dsn"`
	MaxOpenConns int    `json:"max_open_conns"`
	MaxIdleConns int    `json:"max_idle_conns"`
}

type SMTPConfig struct {
	Listen          string `json:"listen"`
	Hostname        string `json:"hostname"`
	MaxMessageBytes int64  `json:"max_message_bytes"`
	ReadTimeout     int    `json:"read_timeout"`
	WriteTimeout    int    `json:"write_timeout"`
}

type IMAPConfig struct {
	Listen       string `json:"listen"`
	ReadTimeout  int    `json:"read_timeout"`
	WriteTimeout int    `json:"write_timeout"`
}

// Load legge il file JSON e restituisce la configurazione validata.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open config %q: %w", path, err)
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("cannot parse config %q: %w", path, err)
	}

	// Valori di default
	if cfg.SMTP.ReadTimeout == 0  { cfg.SMTP.ReadTimeout = 60 }
	if cfg.SMTP.WriteTimeout == 0 { cfg.SMTP.WriteTimeout = 60 }
	if cfg.IMAP.ReadTimeout == 0  { cfg.IMAP.ReadTimeout = 60 }
	if cfg.IMAP.WriteTimeout == 0 { cfg.IMAP.WriteTimeout = 60 }
	if cfg.SMTP.Listen == ""      { cfg.SMTP.Listen = "127.0.0.1:2525" }
	if cfg.IMAP.Listen == ""      { cfg.IMAP.Listen = "127.0.0.1:143" }
	if cfg.SMTP.MaxMessageBytes == 0 { cfg.SMTP.MaxMessageBytes = 52428800 } // 50MB
	if cfg.SMTP.Hostname == ""    { cfg.SMTP.Hostname = "yggmail.local" }
	if cfg.Database.MaxOpenConns == 0 { cfg.Database.MaxOpenConns = 10 }
	if cfg.Database.MaxIdleConns == 0 { cfg.Database.MaxIdleConns = 2 }

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config non valida: %w", err)
	}
	return &cfg, nil
}

// LoadWithKeys carica la config e deriva la chiave pubblica dalla privata.
// Da usare nel server e nella CLI dove serve l'identità del nodo.
func LoadWithKeys(path string) (*Config, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	if cfg.PrivateKeyHex == "" {
		return nil, fmt.Errorf("private_key mancante nel config")
	}
	privBytes, err := hex.DecodeString(cfg.PrivateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("private_key non è hex valido: %w", err)
	}
	if len(privBytes) < 32 {
		return nil, fmt.Errorf("private_key troppo corta (%d byte, minimo 32)", len(privBytes))
	}
	var privKey ed25519.PrivateKey
	if len(privBytes) == 32 {
		privKey = ed25519.NewKeyFromSeed(privBytes)
	} else {
		privKey = ed25519.PrivateKey(privBytes)
	}
	cfg.PrivateKey = privKey
	cfg.PublicKey = privKey.Public().(ed25519.PublicKey)
	return cfg, nil
	
}

func (c *Config) validate() error {
	if c.Database.DSN == "" {
		return fmt.Errorf("database.dsn è obbligatorio")
	}
	return nil
}
