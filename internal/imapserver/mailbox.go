/*
 * Copyright (c) 2021 Neil Alexander
 * Copyright (c) 2026 Giorgio Ladu
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/.
 */
 
package imapserver

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend/backendutil"
	"github.com/emersion/go-message/textproto"
	"Glasir-MTA/internal/storage/types"
)

type Mailbox struct {
	backend *Backend
	name    string
	user    *User
}

func (mbox *Mailbox) u() string { return mbox.user.username }

func (mbox *Mailbox) getIDsFromSeqSet(uid bool, seqSet *imap.SeqSet) ([]int32, error) {
	var ids []int32
	for _, set := range seqSet.Set {
		if set.Stop == 0 {
			next, err := mbox.backend.Storage.MailNextID(mbox.u(), mbox.name)
			if err != nil {
				return nil, fmt.Errorf("MailNextID: %w", err)
			}
			set.Stop = uint32(next - 1)
		}
		for i := set.Start; i <= set.Stop; i++ {
			if !uid {
				pid, err := mbox.backend.Storage.MailIDForSeq(mbox.u(), mbox.name, int(i))
				if err != nil {
					return nil, fmt.Errorf("MailIDForSeq: %w", err)
				}
				ids = append(ids, int32(pid))
			} else {
				ids = append(ids, int32(i))
			}
		}
	}
	return ids, nil
}

func (mbox *Mailbox) Name() string { return mbox.name }

func (mbox *Mailbox) Info() (*imap.MailboxInfo, error) {
	return &imap.MailboxInfo{
		Attributes: []string{},
		Delimiter:  "/",
		Name:       mbox.name,
	}, nil
}

func (mbox *Mailbox) Status(items []imap.StatusItem) (*imap.MailboxStatus, error) {
	status := imap.NewMailboxStatus(mbox.name, items)
	status.PermanentFlags = []string{"\\Seen", "\\Answered", "\\Flagged", "\\Deleted"}
	status.Flags = status.PermanentFlags

	for _, name := range items {
		switch name {
		case imap.StatusMessages:
			count, err := mbox.backend.Storage.MailCount(mbox.u(), mbox.name)
			if err != nil {
				return nil, fmt.Errorf("MailCount: %w", err)
			}
			status.Messages = uint32(count)

		case imap.StatusUidNext:
			id, err := mbox.backend.Storage.MailNextID(mbox.u(), mbox.name)
			if err != nil {
				return nil, fmt.Errorf("MailNextID: %w", err)
			}
			status.UidNext = uint32(id)

		case imap.StatusUidValidity:
			status.UidValidity = 1

		case imap.StatusRecent:
			status.Recent = 0

		case imap.StatusUnseen:
			unseen, err := mbox.backend.Storage.MailUnseen(mbox.u(), mbox.name)
			if err != nil {
				return nil, fmt.Errorf("MailUnseen: %w", err)
			}
			status.Unseen = uint32(unseen)
		}
	}
	return status, nil
}

func (mbox *Mailbox) SetSubscribed(subscribed bool) error {
	return mbox.backend.Storage.MailboxSubscribe(mbox.u(), mbox.name, subscribed)
}

func (mbox *Mailbox) Check() error { return nil }

func (mbox *Mailbox) ListMessages(uid bool, seqSet *imap.SeqSet, items []imap.FetchItem, ch chan<- *imap.Message) error {
	defer close(ch)

	ids, err := mbox.getIDsFromSeqSet(uid, seqSet)
	if err != nil {
		return fmt.Errorf("getIDsFromSeqSet: %w", err)
	}

	for _, id := range ids {
		mseq, mail, err := mbox.backend.Storage.MailSelect(mbox.u(), mbox.name, int(id))
		if err != nil {
			continue
		}

		fetched := imap.NewMessage(uint32(id), items)
		fetched.SeqNum = uint32(mseq)
		fetched.Uid = uint32(mail.ID)

		get := func() (io.Reader, textproto.Header, error) {
			bodyreader := bufio.NewReader(bytes.NewReader(mail.Mail))
			hdr, err := textproto.ReadHeader(bodyreader)
			if err != nil {
				return nil, textproto.Header{}, fmt.Errorf("textproto.ReadHeader: %w", err)
			}
			return bodyreader, hdr, err
		}

		for _, item := range items {
			switch item {
			case imap.FetchEnvelope:
				_, hdr, err := get()
				if err != nil {
					continue
				}
				fetched.Envelope, _ = backendutil.FetchEnvelope(hdr)

			case imap.FetchBody, imap.FetchBodyStructure:
				bodyreader, hdr, err := get()
				if err != nil {
					continue
				}
				fetched.BodyStructure, _ = backendutil.FetchBodyStructure(hdr, bodyreader, item == imap.FetchBodyStructure)

			case imap.FetchFlags:
				fetched.Flags = []string{}
				if mail.Seen     { fetched.Flags = append(fetched.Flags, "\\Seen") }
				if mail.Answered { fetched.Flags = append(fetched.Flags, "\\Answered") }
				if mail.Flagged  { fetched.Flags = append(fetched.Flags, "\\Flagged") }
				if mail.Deleted  { fetched.Flags = append(fetched.Flags, "\\Deleted") }

			case imap.FetchInternalDate:
				fetched.InternalDate = mail.Date

			case imap.FetchRFC822Size:
				fetched.Size = uint32(len(mail.Mail))

			case imap.FetchUid:
				fetched.Uid = uint32(id)

			default:
				section, err := imap.ParseBodySectionName(item)
				if err != nil {
					continue
				}
				bodyreader, hdr, err := get()
				if err != nil {
					continue
				}
				l, err := backendutil.FetchBodySection(hdr, bodyreader, section)
				if err != nil {
					continue
				}
				fetched.Body[section] = l
			}
		}
		ch <- fetched
	}
	return nil
}

func (mbox *Mailbox) SearchMessages(uid bool, criteria *imap.SearchCriteria) ([]uint32, error) {
	return mbox.backend.Storage.MailSearch(mbox.u(), mbox.name)
}

func (mbox *Mailbox) CreateMessage(flags []string, date time.Time, body imap.Literal) error {
	b, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("io.ReadAll: %w", err)
	}
	id, err := mbox.backend.Storage.MailCreate(mbox.u(), mbox.name, b)
	if err != nil {
		return fmt.Errorf("MailCreate: %w", err)
	}
	for _, flag := range flags {
		var seen, answered, flagged, deleted bool
		switch flag {
		case "\\Seen":     seen = true
		case "\\Answered": answered = true
		case "\\Flagged":  flagged = true
		case "\\Deleted":  deleted = true
		}
		if err := mbox.backend.Storage.MailUpdateFlags(mbox.u(), mbox.name, id, seen, answered, flagged, deleted); err != nil {
			return err
		}
	}
	return nil
}

func (mbox *Mailbox) UpdateMessagesFlags(uid bool, seqSet *imap.SeqSet, op imap.FlagsOp, flags []string) error {
	ids, err := mbox.getIDsFromSeqSet(uid, seqSet)
	if err != nil {
		return fmt.Errorf("getIDsFromSeqSet: %w", err)
	}

	for _, id := range ids {
		var mail *types.Mail
		if op != imap.SetFlags {
			_, mail, err = mbox.backend.Storage.MailSelect(mbox.u(), mbox.name, int(id))
			if err != nil {
				return fmt.Errorf("MailSelect: %w", err)
			}
		} else {
			mail = &types.Mail{}
		}
		for _, flag := range flags {
			switch flag {
			case "\\Seen":     mail.Seen     = op != imap.RemoveFlags
			case "\\Answered": mail.Answered = op != imap.RemoveFlags
			case "\\Flagged":  mail.Flagged  = op != imap.RemoveFlags
			case "\\Deleted":  mail.Deleted  = op != imap.RemoveFlags
			}
		}
		if err := mbox.backend.Storage.MailUpdateFlags(mbox.u(), mbox.name, int(id), mail.Seen, mail.Answered, mail.Flagged, mail.Deleted); err != nil {
			return err
		}
	}
	return nil
}

func (mbox *Mailbox) CopyMessages(uid bool, seqSet *imap.SeqSet, destName string) error {
	if destName == "Outbox" {
		return fmt.Errorf("can't copy into Outbox: protected folder")
	}
	ids, err := mbox.getIDsFromSeqSet(uid, seqSet)
	if err != nil {
		return fmt.Errorf("getIDsFromSeqSet: %w", err)
	}
	for _, id := range ids {
		_, mail, err := mbox.backend.Storage.MailSelect(mbox.u(), mbox.name, int(id))
		if err != nil {
			return fmt.Errorf("MailSelect: %w", err)
		}
		pid, err := mbox.backend.Storage.MailCreate(mbox.u(), destName, mail.Mail)
		if err != nil {
			return fmt.Errorf("MailCreate: %w", err)
		}
		if err = mbox.backend.Storage.MailUpdateFlags(mbox.u(), mbox.name, pid, mail.Seen, mail.Answered, mail.Flagged, mail.Deleted); err != nil {
			return err
		}
	}
	return nil
}

func (mbox *Mailbox) Expunge() error {
	return mbox.backend.Storage.MailExpunge(mbox.u(), mbox.name)
}

func (mbox *Mailbox) MoveMessages(uid bool, seqset *imap.SeqSet, dest string) error {
	if dest == "Outbox" {
		return fmt.Errorf("can't move into Outbox: protected folder")
	}
	ids, err := mbox.getIDsFromSeqSet(uid, seqset)
	if err != nil {
		return fmt.Errorf("getIDsFromSeqSet: %w", err)
	}
	for _, id := range ids {
		if err := mbox.backend.Storage.MailMove(mbox.u(), mbox.name, int(id), dest); err != nil {
			return err
		}
		if mbox.name == "Outbox" {
			if err := mbox.backend.Storage.QueueDeleteDestinationForID("Outbox", int(id)); err != nil {
				return err
			}
		}
	}
	return nil
}
