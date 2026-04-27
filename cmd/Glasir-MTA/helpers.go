/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */
// cmd/Glasir-MTA/helpers.go — funzioni di supporto per la CLI: UI, colori, config, DB.
package main

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/fatih/color"
	"Glasir-MTA/internal/config"
	"Glasir-MTA/internal/storage/mariadb"
)

// ── Versione ──────────────────────────────────────────────────────────────────

const Version = "2.3"

// ── Costanti colore ───────────────────────────────────────────────────────────

const (
	magenta = "magenta"
	cyan    = "cyan"
	white   = "white"
	yellow  = "yellow"
	red     = "red"
	green   = "green"
)

const (
	statusOK   = "ok"
	statusInfo = "info"
	statusErr  = "err"
	statusWarn = "warn"
)

func c(col, text string) string {
	switch col {
	case magenta:
		return color.MagentaString(text)
	case cyan:
		return color.CyanString(text)
	case white:
		return color.WhiteString(text)
	case yellow:
		return color.YellowString(text)
	case red:
		return color.RedString(text)
	case green:
		return color.GreenString(text)
	}
	return text
}

func b(text string) string {
	return color.New(color.Bold).Sprint(text)
}

func dim(text string) string {
	return color.New(color.Faint).Sprint(text)
}

// ── Banner ────────────────────────────────────────────────────────────────────

func printBanner() {
	cyanBold := color.New(color.FgCyan, color.Bold)
	faint    := color.New(color.Faint)

	fmt.Println()
	cyanBold.Println("  ╔═══════════════════════════════════════════╗")
	cyanBold.Print("  ║  ")
	b1 := b("GLASIR-MTA") + " · Mail Transfer Agent"
	fmt.Printf("%-48s", b1)
	cyanBold.Println("  ║")
	cyanBold.Print("  ║  ")
	fmt.Printf("%-48s", faint.Sprint("Yggdrasil overlay  ·  v"+Version))
	cyanBold.Println("  ║")
	cyanBold.Println("  ╚═══════════════════════════════════════════╝")
	fmt.Println()
}

// ── Box e sezioni ─────────────────────────────────────────────────────────────

const boxInner = 48 // larghezza interna del box

func printSection(title string) {
	fmt.Printf("\n  %s\n", b(title))
	fmt.Println("  " + strings.Repeat("─", boxInner))
}

// printBox stampa un box con titolo e righe di contenuto.
func printBox(title string, lines []string) {
	bar := strings.Repeat("─", boxInner-len(title)-3)
	fmt.Printf("  ┌─ %s %s┐\n", b(title), bar)
	for _, line := range lines {
		pad := boxInner - len(stripANSI(line))
		if pad < 0 {
			pad = 0
		}
		fmt.Printf("  │  %s%s│\n", line, strings.Repeat(" ", pad))
	}
	fmt.Printf("  └%s┘\n", strings.Repeat("─", boxInner+2))
}

// stripANSI rimuove le sequenze escape ANSI per calcolare la lunghezza visibile.
func stripANSI(s string) string {
	var out strings.Builder
	skip := false
	for _, r := range s {
		if r == '\033' {
			skip = true
		}
		if !skip {
			out.WriteRune(r)
		}
		if skip && r == 'm' {
			skip = false
		}
	}
	return out.String()
}

// ── Output ────────────────────────────────────────────────────────────────────

func printStatus(label, value, status string) {
	var icon string
	switch status {
	case statusOK:
		icon = color.GreenString("✓")
	case statusInfo:
		icon = color.CyanString("·")
	case statusWarn:
		icon = color.YellowString("!")
	case statusErr:
		icon = color.RedString("✗")
	default:
		icon = " "
	}
	fmt.Printf("  %s  %-22s %s\n", icon, b(label)+":", value)
}

func printFatal(msg string, err error) {
	fmt.Println()
	if err != nil {
		color.Red("  ✗  %s: %v", msg, err)
	} else {
		color.Red("  ✗  %s", msg)
	}
	fmt.Println()
	os.Exit(1)
}

func printOK(msg string) {
	fmt.Printf("  %s  %s\n", color.GreenString("✓"), msg)
}

func printInfo(msg string) {
	fmt.Printf("  %s  %s\n", color.CyanString("·"), dim(msg))
}

// ── Tabelle ───────────────────────────────────────────────────────────────────

type TableColumn struct {
	Header string
	Width  int
}

func printTableHeader(cols []TableColumn) {
	fmt.Print("  ")
	for _, col := range cols {
		fmt.Printf("%-*s  ", col.Width, b(col.Header))
	}
	fmt.Println()
	fmt.Print("  ")
	for _, col := range cols {
		fmt.Printf("%s  ", strings.Repeat("─", col.Width))
	}
	fmt.Println()
}

func printTableRow(cols []TableColumn, values []string) {
	fmt.Print("  ")
	for i, col := range cols {
		val := ""
		if i < len(values) {
			val = values[i]
		}
		pad := col.Width - len(stripANSI(val))
		if pad < 0 {
			pad = 0
		}
		fmt.Printf("%s%s  ", val, strings.Repeat(" ", pad))
	}
	fmt.Println()
}

// ── Prompt I/O ────────────────────────────────────────────────────────────────

func promptLine(prompt string) string {
	fmt.Print(c(cyan, prompt))
	var line string
	fmt.Scanln(&line)
	return strings.TrimSpace(line)
}

// ── Config & DB ───────────────────────────────────────────────────────────────

func mustLoadConfig(path string) *config.Config {
	cfg, err := config.LoadWithKeys(path)
	if err != nil {
		printFatal("Impossibile caricare il config", err)
	}
	return cfg
}

func mustOpenDB(cfg *config.Config) *mariadb.DB {
	db, err := mariadb.New(cfg.Database.DSN, 10, 2)
	if err != nil {
		printFatal("Connessione MariaDB fallita", err)
	}
	return db
}

// ── Rete Yggdrasil ────────────────────────────────────────────────────────────

// detectYggdrasilIP trova l'indirizzo IPv6 Yggdrasil (200::/7).
func detectYggdrasilIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() == nil && len(ip) == 16 && ip[0] == 0x02 {
				return ip.String()
			}
		}
	}
	return ""
}

// yggdrasilIP è un alias per comodità nella CLI.
func yggdrasilIP() string { return detectYggdrasilIP() }
