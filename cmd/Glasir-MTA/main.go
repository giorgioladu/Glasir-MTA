/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */
package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/fatih/color"

	"Glasir-MTA/internal/config"
	"Glasir-MTA/internal/imapserver"
	"Glasir-MTA/internal/smtpsender"
	"Glasir-MTA/internal/smtpserver"
	"Glasir-MTA/internal/storage/mariadb"
)

func main() {
	configPath := flag.String("config", "config.json", "Percorso del file config.json")

	flag.Usage = func() {
		fmt.Printf("Utilizzo: %s [opzioni] [comando]\n\n", os.Args[0])
		fmt.Println("Opzioni:")
		flag.PrintDefaults()
		fmt.Println()
		usageCommands()
	}

	flag.Parse()

	// 1. Carica Config + chiavi
	cfg, err := config.LoadWithKeys(*configPath)
	if err != nil {
		color.Red("Errore config: %v", err)
		os.Exit(1)
	}

	privBytes, err := hex.DecodeString(cfg.PrivateKeyHex)
	if err != nil || len(privBytes) < 32 {
		color.Red("Chiave privata non valida nel JSON!")
		os.Exit(1)
	}
	var privKey ed25519.PrivateKey
	if len(privBytes) == 32 {
		privKey = ed25519.NewKeyFromSeed(privBytes)
	} else {
		privKey = ed25519.PrivateKey(privBytes)
	}
	cfg.PublicKey = privKey.Public().(ed25519.PublicKey)
	pubKeyHex := hex.EncodeToString(cfg.PublicKey)

	// 2. CLI Dispatcher
	if args := flag.Args(); len(args) > 0 {
		if dispatch(args[0], *configPath) {
			return
		}
	}

	// 3. Banner
	color.Cyan("GLASIR - MTA  ")
	fmt.Printf("Node: %s\n\n", color.YellowString(pubKeyHex+"@yggmail"))

	// 4. Database
	storage, err := mariadb.New(cfg.Database.DSN, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns)
	if err != nil {
		log.Fatalf("DB Error: %v", err)
	}
	defer storage.Close()

	// 5. Code di invio (senza transport Yggdrasil — usa TCP diretto su tun0)
	queues := smtpsender.NewQueues(cfg, log.Default(), storage)

	// 6. IMAP
	imapBackend := &imapserver.Backend{
		Storage: storage,
		Config:  cfg,
		Log:     log.Default(),
	}
	imapSrv, notify, _ := imapserver.NewIMAPServer(imapBackend, cfg.IMAP.Listen, true)
	go imapSrv.ListenAndServe()
	color.Green("IMAP  in ascolto su %s", cfg.IMAP.Listen)

	// 7. SMTP locale (autenticato, per i client)
	localBackend := &smtpserver.Backend{
		Log:     log.Default(),
		Mode:    smtpserver.BackendModeInternal,
		Config:  cfg,
		Storage: storage,
		Queues:  queues,
		Notify:  notify,
	}
	localSrv := smtp.NewServer(localBackend)
	localSrv.Addr             = cfg.SMTP.Listen
	localSrv.Domain           = cfg.SMTP.Hostname
	localSrv.AllowInsecureAuth = true
  
	localSrv.EnableAuth(sasl.Login, func(conn *smtp.Conn) sasl.Server {
		return sasl.NewLoginServer(func(u, p string) error {
			state := conn.State()
			_, err := localBackend.Login(&state, u, p)
			return err
		})
	})
	go localSrv.ListenAndServe()
	color.Green("SMTP  in ascolto su %s", cfg.SMTP.Listen)

	// 8. SMTP overlay Yggdrasil (anonimo, su IPv6 porta 25)

	yggIP := detectYggdrasilIP()
	if yggIP != "" {
		yggListenAddr := fmt.Sprintf("[%s]:25", yggIP)
		
		yggBackend := &smtpserver.Backend{
			Log:     log.Default(),
			Mode:    smtpserver.BackendModeYgg,
			Config:  cfg,
			Storage: storage,
			Queues:  queues,
			Notify:  notify,
		}

		// CORREZIONE: Usa il costruttore del nostro pacchetto smtpserver
		// che imposta correttamente i certificati e il TLSConfig
		yggWrapper, err := smtpserver.NewSMTPServer(yggBackend, notify)
		if err != nil {
			color.Red("Errore configurazione server Ygg TLS: %v", err)
			os.Exit(1)
		}

		// Sovrascriviamo l'indirizzo di ascolto con quello specifico di Yggdrasil
		yggWrapper.GetInnerServer().Addr = yggListenAddr

		go func() {
			if err := yggWrapper.ListenAndServe(); err != nil {
				log.Printf("Errore server SMTP Ygg: %v", err)
			}
		}()
		
		color.Green("SMTP  overlay in ascolto su %s (TLS Attivo)", yggListenAddr)
	}

	// 9. Attendi segnale di stop
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	color.Cyan("\nGlasir-MTA arrestato.")
}
