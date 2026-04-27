/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */
// cmd/Glasir-MTA/cmd_users.go — gestione utenti: adduser, deluser, passwd.
package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

// ── ADDUSER ───────────────────────────────────────────────────────────────────

func runAddUser(configPath string) {
	fs := flag.NewFlagSet("adduser", flag.ExitOnError)
	address  := fs.String("address", "", "Indirizzo email completo")
	quotaStr := fs.String("quota", "0", "Quota: 0=illimitata, es. 500MB, 1GB")
	fs.Parse(os.Args[2:])

	printBanner()
	printSection("Crea utente")

	cfg := mustLoadConfig(configPath)
	db  := mustOpenDB(cfg)
	defer db.Close()
	
	yggIP := yggdrasilIP()
	if yggIP == "" {
		printFatal("Nessun IP Yggdrasil trovato — yggdrasil è avviato?", nil)
	}

	if *address == "" {
    // Modalità interattiva — chiedi solo il nome
    // Rimuovi eventuali @ che l'utente potrebbe aver digitato per sbaglio
    name := strings.Split(promptLine("  Nome utente : "), "@")[0]
    *address = name + "@" + yggIP
    fmt.Printf("  %s %s\n\n", dim("→ Indirizzo:"), b(*address))
	} else {
    // Modalità flag — valida che il dominio sia il nostro IP
    parts := strings.SplitN(*address, "@", 2)
    if len(parts) != 2 || parts[1] != yggIP {
        printFatal("Il dominio deve essere l'IP di questo nodo: "+yggIP, nil)
    }
  }

	if exists, _ := db.UserExists(*address); exists {
		printFatal("L'utente esiste già", nil)
	}

	password := promptPassword("  Password        : ")
	confirm  := promptPassword("  Conferma        : ")
	if password != confirm {
		printFatal("Le password non corrispondono", nil)
	}

	quota, err := parseQuota(*quotaStr)
	if err != nil {
		printFatal("Formato quota non valido", err)
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err := db.UserCreate(*address, string(hash), quota); err != nil {
		printFatal("Errore creazione utente", err)
	}

	fmt.Println()
	printStatus("Utente",  *address, statusOK)
	quotaLabel := "illimitata"
	if *quotaStr != "0" { quotaLabel = *quotaStr }
	printStatus("Quota", quotaLabel, statusInfo)

	// Alias Yggmail
	yggAlias := hex.EncodeToString(cfg.PublicKey) + "@yggmail"
	owner, err := db.YggmailAliasOwner()
	if err != nil {
		if db.AssignYggmailAlias(*address, yggAlias) == nil {
			printStatus("Alias nodo", yggAlias, statusOK)
			printInfo("Le mail all'alias del nodo arriveranno in questa mailbox.")
		}
	} else {
		printStatus("Alias nodo", "già assegnato a "+owner, statusWarn)
		printInfo("Usa 'alias assign' per cambiare.")
	}
	fmt.Println()
}

// ── DELUSER ───────────────────────────────────────────────────────────────────

func runDelUser(configPath string) {
	printBanner()
	printSection("Elimina utente")

	address := promptLine("  Indirizzo : ")
	fmt.Printf("\n  %s Questa operazione è irreversibile.\n", c("!", yellow))
	confirm := promptLine("  Scrivi DELETE per confermare : ")
	if confirm != "DELETE" {
		printInfo("Operazione annullata.")
		return
	}

	db := mustOpenDB(mustLoadConfig(configPath))
	defer db.Close()

	if err := db.UserDelete(address); err != nil {
		printFatal("Errore eliminazione", err)
	}
	fmt.Println()
	printStatus("Eliminato", address, statusOK)
	fmt.Println()
}

// ── PASSWD ────────────────────────────────────────────────────────────────────

func runPasswd(configPath string) {
	printBanner()
	printSection("Cambia password")

	address := promptLine("  Indirizzo : ")
	db := mustOpenDB(mustLoadConfig(configPath))
	defer db.Close()

	newPass := promptPassword("  Nuova password  : ")
	confirm := promptPassword("  Conferma        : ")
	if newPass != confirm {
		printFatal("Le password non corrispondono", nil)
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(newPass), bcrypt.DefaultCost)
	if err := db.UserUpdatePassword(address, string(hash)); err != nil {
		printFatal("Errore aggiornamento", err)
	}
	fmt.Println()
	printStatus("Password aggiornata", address, statusOK)
	fmt.Println()
}

// ── I/O ───────────────────────────────────────────────────────────────────────

func promptPassword(prompt string) string {
	fmt.Print(c(cyan, prompt))
	if term.IsTerminal(int(syscall.Stdin)) {
		pw, _ := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		return string(pw)
	}
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func parseQuota(s string) (int64, error) {
	if s == "0" || s == "" {
		return 0, nil
	}
	multipliers := map[string]int64{
		"TB": 1 << 40, "GB": 1 << 30, "MB": 1 << 20, "KB": 1 << 10,
	}
	s = strings.ToUpper(strings.TrimSpace(s))
	for suffix, mult := range multipliers {
		if strings.HasSuffix(s, suffix) {
			var n int64
			fmt.Sscanf(strings.TrimSuffix(s, suffix), "%d", &n)
			return n * mult, nil
		}
	}
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n, nil
}
