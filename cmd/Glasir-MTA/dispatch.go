/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */
// cmd/Glasir-MTA/dispatch.go — router dei sottocomandi CLI.
package main

import (
	"fmt"
	"os"
)

func dispatch(cmd string, configPath string) bool {
	switch cmd {
	case "adduser":
		runAddUser(configPath)
	case "deluser":
		runDelUser(configPath)
	case "passwd":
		runPasswd(configPath)
	case "quota":
		runQuota(configPath)
	case "listusers":
		runListUsers(configPath)
	case "alias":
		runAlias(configPath)
	case "status":
		runStatus(configPath)
	default:
		if cmd == "serve" || cmd == "" {
			return false
		}
		usageCommands()
		os.Exit(1)
	}
	return true
}

func usageCommands() {
	printBanner()
	printSection("Comandi disponibili")
	fmt.Println()

	cmds := [][2]string{
		{"serve",         "Avvia SMTP e IMAP (default)"},
		{"status",        "Dashboard: stato del nodo e dei servizi"},
		{"adduser",       "Crea una nuova mailbox"},
		{"deluser",       "Elimina un utente e le sue mail"},
		{"passwd",        "Cambia la password di un utente"},
		{"quota",         "Visualizza o imposta la quota"},
		{"listusers",     "Elenca tutti gli utenti"},
		{"alias list",    "Elenca gli alias"},
		{"alias assign",  "Assegna l'alias del nodo (hex@yggmail)"},
		{"alias add",     "Crea un alias generico"},
		{"alias remove",  "Rimuove un alias"},
	}

	for _, cmd := range cmds {
		fmt.Printf("  %-20s %s\n", b(cmd[0]), dim(cmd[1]))
	}
	fmt.Println()
}
