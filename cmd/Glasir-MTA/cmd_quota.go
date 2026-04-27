/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */
// cmd/Glasir-MTA/cmd_quota.go — comandi quota e listusers.

package main

import (
	"flag"
	"fmt"
	"os"
)

// ── QUOTA ─────────────────────────────────────────────────────────────────────

func runQuota(configPath string) {
	fs := flag.NewFlagSet("quota", flag.ExitOnError)
	address := fs.String("address", "", "Indirizzo utente")
	set     := fs.String("set", "", "Imposta quota (es. 500MB, 1GB, 0=illimitata)")
	fs.Parse(os.Args[2:])

	printBanner()
	printSection("Quota")

	cfg := mustLoadConfig(configPath)
	db  := mustOpenDB(cfg)
	defer db.Close()

	if *address == "" {
		*address = promptLine("  Indirizzo : ")
	}
	fmt.Println()

	if *set != "" {
		quota, err := parseQuota(*set)
		if err != nil {
			printFatal("Formato quota non valido", err)
		}
		if err := db.UserSetQuota(*address, quota); err != nil {
			printFatal("Errore aggiornamento quota", err)
		}
		label := *set
		if *set == "0" { label = "illimitata" }
		printStatus("Quota aggiornata", label, statusOK)
	} else {
		used, quota, err := db.UserQuotaInfo(*address)
		if err != nil {
			printFatal("Utente non trovato", err)
		}
		if quota > 0 {
			pct := float64(used) / float64(quota) * 100
			printStatus("Utilizzo", fmt.Sprintf("%s / %s  (%.1f%%)",
				formatBytes(used), formatBytes(quota), pct), statusInfo)
		} else {
			printStatus("Utilizzo", fmt.Sprintf("%s / ∞", formatBytes(used)), statusInfo)
		}
	}
	fmt.Println()
}

// ── LISTUSERS ─────────────────────────────────────────────────────────────────

func runListUsers(configPath string) {
	printBanner()
	printSection("Utenti registrati")

	db := mustOpenDB(mustLoadConfig(configPath))
	defer db.Close()

	users, err := db.ListUsers()
	if err != nil {
		printFatal("Impossibile recuperare la lista utenti", err)
	}

	cols := []TableColumn{
		{Header: "Indirizzo", Width: 50},
		{Header: "Quota",     Width: 12},
		{Header: "Creato",    Width: 12},
	}
	fmt.Println()
	printTableHeader(cols)

	for _, u := range users {
		quotaStr := "∞"
		if u.QuotaBytes > 0 {
			quotaStr = formatBytes(u.QuotaBytes)
		}
		printTableRow(cols, []string{
			c(cyan, u.Address),
			quotaStr,
			u.CreatedAt,
		})
	}

	fmt.Printf("\n  %s %s utenti\n\n", dim("Totale:"), b(fmt.Sprintf("%d", len(users))))
}

// ── formatBytes ───────────────────────────────────────────────────────────────

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
