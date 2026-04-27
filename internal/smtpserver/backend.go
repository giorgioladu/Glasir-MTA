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
	"encoding/hex"
	"fmt"
	"log"

	"github.com/emersion/go-smtp"
	"Glasir-MTA/internal/config"
	"Glasir-MTA/internal/imapserver"
	"Glasir-MTA/internal/smtpsender"
	"Glasir-MTA/internal/storage"
	//"Glasir-MTA/internal/utils"
)

type BackendMode int

const (
	BackendModeInternal BackendMode = iota
	BackendModeExternal
	BackendModeYgg  
)

type Backend struct {
	Mode    BackendMode
	Log     *log.Logger
	Config  *config.Config
	Queues  *smtpsender.Queues
	Storage storage.Storage
	Notify  *imapserver.IMAPNotify
}

func (b *Backend) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	switch b.Mode {
		case BackendModeInternal:
			// Autentica con indirizzo completo e password bcrypt
			if err := b.Storage.UserAuthenticate(username, password); err != nil {
				b.Log.Printf("SMTP: autenticazione fallita per %q: %s", username, err)
				return nil, smtp.ErrAuthRequired
			}
			
			addr := "unknown"
				if state != nil {
					addr = state.RemoteAddr.String()
				}
				b.Log.Printf("SMTP: autenticato %q da %s", username, addr)
			
			return &SessionLocal{
				backend: b,
				state:   state,
			}, nil

			case BackendModeExternal:
				return nil, fmt.Errorf("not expecting authenticated connection on external backend")
				
		    case BackendModeYgg:
			b.Log.Println("Incoming Ygg SMTP from", state.RemoteAddr.String())
			return &SessionYgg{backend: b, state: state}, nil
	}
		
	return nil, fmt.Errorf("authenticated login failed")
}

func (b *Backend) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
    switch b.Mode {
    case BackendModeInternal:
        return nil, fmt.Errorf("not expecting anonymous connection on internal backend")

	  case BackendModeExternal:
			// The connection came from our overlay listener, so we should check
			// that they are who they claim to be
			pks, err := hex.DecodeString(state.RemoteAddr.String())
			if err != nil {
				return nil, fmt.Errorf("hex.DecodeString: %w", err)
			}
			remote := hex.EncodeToString(pks)
			if state.Hostname != remote {
				return nil, fmt.Errorf("you are not who you claim to be")
			}

			b.Log.Println("Incoming SMTP session from", remote)
			return &SessionRemote{
				backend: b,
				state:   state,
				public:  pks[:],
			}, nil

    case BackendModeYgg:
        // Connessione diretta IPv6 Yggdrasil — nessuna auth richiesta
        b.Log.Println("Incoming Ygg SMTP from", state.RemoteAddr.String())
        return &SessionYgg{
            backend: b,
            state:   state,
        }, nil
    }
    return nil, fmt.Errorf("anonymous login failed")
}
