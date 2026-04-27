/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */
// cmd/Glasir-MTA/cmd_status.go — dashboard del nodo Glasir-MTA.
package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"github.com/fatih/color"
	"Glasir-MTA/internal/storage/mariadb"
)

func runStatus(configPath string) {
	printBanner()

	cfg := mustLoadConfig(configPath)
	db  := mustOpenDB(cfg)
	defer db.Close()

	pubKeyHex := hex.EncodeToString(cfg.PublicKey)
	yggIP     := detectYggdrasilIP()
	now       := time.Now().Format("2006-01-02  15:04:05")

	// ── NODO ─────────────────────────────────────────────────────────────────
	shortKey := pubKeyHex[:8] + "…" + pubKeyHex[56:]
	printBox("NODE", []string{
		row("Key",     color.YellowString(shortKey+"@yggmail")),
		row("IPv6",       fmtIPv6(yggIP)),
		row("Timestamp",  dim(now)),
	})

	// ── SERVIZI ───────────────────────────────────────────────────────────────
	yggSMTPAddr := "n/d"
	if yggIP != "" {
		yggSMTPAddr = fmt.Sprintf("[%s]:25", yggIP)
	}
	printBox("SERVICES", []string{
		rowSvc("Local SMTP",   cfg.SMTP.Listen,  portStatus(cfg.SMTP.Listen)),
		rowSvc("IMAP",          cfg.IMAP.Listen,  portStatus(cfg.IMAP.Listen)),
		rowSvc("SMTP overlay",  yggSMTPAddr,       portStatus(yggSMTPAddr)),
		rowSvc("Database",      dbAddr(cfg.Database.DSN), dbStatus(db)),
	})

	// ── STATISTICHE ───────────────────────────────────────────────────────────
	users,   _ := db.ListUsers()
	aliases, _ := db.ListAliases("")
	msgs,    _ := db.CountAllMessages()
	queued,  _ := db.CountQueue()

	printBox("STATISTICS", []string{
		row("Users",    color.CyanString("%d", len(users))),
		row("Aliases",   color.CyanString("%d", len(aliases))),
		row("Messages",  color.CyanString("%d", msgs)),
		row("Queue",   fmtQueue(queued)),
	})

	fmt.Println()
}

// ── Helper di formattazione ───────────────────────────────────────────────────

// row formatta una riga label: valore per printBox.
func row(label, value string) string {
	return fmt.Sprintf("%-10s %-5s", b(label)+":", value)
}

// rowSvc formatta una riga con indirizzo e stato per la sezione servizi.
func rowSvc(label, addr, status string) string {
	return fmt.Sprintf("%-10s %-10s %s", b(label)+":", dim(addr), status)
}

func fmtIPv6(ip string) string {
	if ip == "" {
		return color.RedString("✗ non trovato — yggdrasil è avviato?")
	}
	return color.GreenString("✓ ") + ip
}

func fmtQueue(n int) string {
	if n == 0 {
		return color.GreenString("✓ vuota")
	}
	return color.YellowString("! %d in attesa", n)
}

// portStatus verifica se una porta TCP è in ascolto.
func portStatus(addr string) string {
	if addr == "n/d" {
		return color.YellowString("· disabilitato")
	}
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return color.RedString("✗ offline")
	}
	conn.Close()
	return color.GreenString("✓ online")
}

// dbStatus verifica la connessione al database.
func dbStatus(db *mariadb.DB) string {
	if err := db.Ping(); err != nil {
		return color.RedString("✗ offline")
	}
	return color.GreenString("✓ online")
}

// dbAddr estrae host:port dal DSN MariaDB per la visualizzazione.
func dbAddr(dsn string) string {
	// formato: user:pass@tcp(host:port)/dbname
	start := len(dsn)
	for i, ch := range dsn {
		if ch == '(' {
			start = i + 1
		}
		if ch == ')' && start < i {
			return dsn[start:i]
		}
	}
	return "database"
}
