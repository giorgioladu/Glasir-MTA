/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */
// cmd/Glasir-MTA/cmd_alias.go — gestione alias.
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
)

func runAlias(configPath string) {
	if len(os.Args) < 3 {
		printAliasUsage()
		os.Exit(1)
	}
	sub := os.Args[2]
	os.Args = append(os.Args[:2], os.Args[3:]...)

	switch sub {
	case "list":
		runAliasList(configPath)
	case "assign":
		runAliasAssign(configPath)
	case "add":
		runAliasAdd(configPath)
	case "remove", "delete", "del":
		runAliasRemove(configPath)
	default:
		printAliasUsage()
	}
}

func runAliasList(configPath string) {
	fs := flag.NewFlagSet("alias list", flag.ExitOnError)
	address := fs.String("address", "", "Filtra per indirizzo utente")
	fs.Parse(os.Args[2:])

	printBanner()
	printSection("Aliases")

	db := mustOpenDB(mustLoadConfig(configPath))
	defer db.Close()

	aliases, err := db.ListAliases(*address)
	if err != nil {
		printFatal("Impossibile recuperare gli alias", err)
	}

	cols := []TableColumn{
		{Header: "Alias",       Width: 70},
		{Header: "Destinatario", Width: 52},
		{Header: "Tipo",        Width: 14},
	}
	fmt.Println()
	printTableHeader(cols)

	for _, a := range aliases {
		tipo := dim("generico")
		if a.IsYggmail {
			tipo = c(yellow, "nodo")
		}
		printTableRow(cols, []string{
			c(cyan, a.Alias),
			a.Target,
			tipo,
		})
	}
	fmt.Printf("\n  %s %s alias\n\n", dim("Totale:"), b(fmt.Sprintf("%d", len(aliases))))
}

func runAliasAssign(configPath string) {
	fs := flag.NewFlagSet("alias assign", flag.ExitOnError)
	address := fs.String("address", "", "Utente a cui assegnare l'alias del nodo")
	fs.Parse(os.Args[2:])

	printBanner()
	printSection("Assegna alias nodo")

	cfg := mustLoadConfig(configPath)
	db  := mustOpenDB(cfg)
	defer db.Close()

	if *address == "" {
		*address = promptLine("  Indirizzo utente : ")
	}

	yggAlias := hex.EncodeToString(cfg.PublicKey) + "@yggmail"
	fmt.Println()

	if err := db.AssignYggmailAlias(*address, yggAlias); err != nil {
		printFatal("Errore durante l'assegnazione", err)
	}
	printStatus("Alias assegnato", yggAlias+" → "+*address, statusOK)
	fmt.Println()
}

func runAliasAdd(configPath string) {
	fs := flag.NewFlagSet("alias add", flag.ExitOnError)
	alias  := fs.String("alias", "", "Alias da creare")
	target := fs.String("target", "", "Indirizzo utente reale")
	fs.Parse(os.Args[2:])

	printBanner()
	printSection("Crea alias")

	db := mustOpenDB(mustLoadConfig(configPath))
	defer db.Close()

	if *alias == ""  { *alias  = promptLine("  Alias  : ") }
	if *target == "" { *target = promptLine("  Target : ") }
	fmt.Println()

	if err := db.AddAlias(*alias, *target); err != nil {
		printFatal("Impossibile creare l'alias", err)
	}
	printStatus("Alias creato", *alias+" → "+*target, statusOK)
	fmt.Println()
}

func runAliasRemove(configPath string) {
	fs := flag.NewFlagSet("alias remove", flag.ExitOnError)
	alias := fs.String("alias", "", "Alias da rimuovere")
	fs.Parse(os.Args[2:])

	printBanner()
	printSection("Rimuovi alias")

	db := mustOpenDB(mustLoadConfig(configPath))
	defer db.Close()

	if *alias == "" { *alias = promptLine("  Alias : ") }
	fmt.Println()

	if err := db.DeleteAlias(*alias); err != nil {
		printFatal("Impossibile rimuovere l'alias", err)
	}
	printStatus("Rimosso", *alias, statusOK)
	fmt.Println()
}

func printAliasUsage() {
	printBanner()
	printSection("Comandi alias")
	fmt.Println("  list   [-address <addr>]          Elenca gli alias")
	fmt.Println("  assign [-address <addr>]          Assegna l'alias del nodo")
	fmt.Println("  add    -alias <a> -target <b>     Crea un alias generico")
	fmt.Println("  remove -alias <a>                 Rimuove un alias")
	fmt.Println()
}
