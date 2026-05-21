package imap

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/mailstore"
	"github.com/go-postnest/postnest/internal/models"
	"github.com/go-postnest/postnest/internal/redis"
	"github.com/google/uuid"
)

// maxIMAPBatchSize limits the number of messages loaded per query.
// IMAP sequence numbering requires all messages to be loaded for correctness;
// large mailboxes may need streaming or cursor-based pagination in future work.
const maxIMAPBatchSize = 5000

type imapBackend struct {
	store mailstore.Store
	auth  *auth.Service
	redis *redis.Client
}

func (b *imapBackend) Login(connInfo *imap.ConnInfo, username, password string) (backend.User, error) {
	ctx := context.Background()
	user, err := b.auth.Authenticate(ctx, username, password)
	if err != nil {
		return nil, backend.ErrInvalidCredentials
	}
	domains, err := b.auth.GetUserDomains(ctx, user.ID)
	if err != nil || len(domains) == 0 {
		return nil, backend.ErrInvalidCredentials
	}
	return &imapUser{backend: b, user: user, domainID: domains[0].DomainID}, nil
}

type imapUser struct {
	backend  *imapBackend
	user     *models.User
	domainID uuid.UUID
}

func (u *imapUser) Username() string { return u.user.Email }

func (u *imapUser) ListMailboxes(subscribed bool) ([]backend.Mailbox, error) {
	ctx := context.Background()
	labels, err := u.backend.store.GetLabels(ctx, u.domainID, u.user.ID)
	if err != nil {
		return nil, err
	}
	var boxes []backend.Mailbox
	for _, l := range labels {
		boxes = append(boxes, &imapMailbox{backend: u.backend, user: u.user, domainID: u.domainID, label: l})
	}
	return boxes, nil
}

func (u *imapUser) GetMailbox(name string) (backend.Mailbox, error) {
	ctx := context.Background()
	label, err := u.backend.store.GetLabelByName(ctx, u.domainID, u.user.ID, name)
	if err != nil {
		return nil, backend.ErrNoSuchMailbox
	}
	return &imapMailbox{backend: u.backend, user: u.user, domainID: u.domainID, label: label}, nil
}

func (u *imapUser) CreateMailbox(name string) error {
	ctx := context.Background()
	l := &models.Label{
		DomainID: u.domainID,
		UserID:   u.user.ID,
		Name:     name,
		Color:    "#4285f4",
	}
	return u.backend.store.CreateLabel(ctx, l)
}

func (u *imapUser) DeleteMailbox(name string) error {
	ctx := context.Background()
	label, err := u.backend.store.GetLabelByName(ctx, u.domainID, u.user.ID, name)
	if err != nil {
		return backend.ErrNoSuchMailbox
	}
	if label.IsSystem {
		return fmt.Errorf("cannot delete system mailbox")
	}
	return u.backend.store.DeleteLabel(ctx, u.domainID, u.user.ID, label.ID)
}

func (u *imapUser) RenameMailbox(existingName, newName string) error {
	return fmt.Errorf("rename not supported")
}

func (u *imapUser) Logout() error { return nil }

type imapMailbox struct {
	backend  *imapBackend
	user     *models.User
	domainID uuid.UUID
	label    *models.Label
}

func (m *imapMailbox) Name() string { return m.label.Name }

func (m *imapMailbox) Info() (*imap.MailboxInfo, error) {
	return &imap.MailboxInfo{
		Name:       m.label.Name,
		Attributes: nil,
		Delimiter:  "/",
	}, nil
}

func (m *imapMailbox) Status(items []imap.StatusItem) (*imap.MailboxStatus, error) {
	ctx := context.Background()
	status := imap.NewMailboxStatus(m.label.Name, items)
	for _, item := range items {
		switch item {
		case imap.StatusMessages:
			total, _ := m.backend.store.CountTotalByLabel(ctx, m.domainID, m.user.ID, m.label.ID)
			status.Messages = uint32(total)
		case imap.StatusUnseen:
			unread, _ := m.backend.store.CountUnreadByLabel(ctx, m.domainID, m.user.ID, m.label.ID)
			status.Unseen = uint32(unread)
		case imap.StatusUidNext:
			maxUID, _ := m.backend.store.GetMaxIMAPUID(ctx, m.user.ID, m.label.Name)
			status.UidNext = maxUID + 1
		case imap.StatusUidValidity:
			// Derive a stable validity value from the label ID.
			id := m.label.ID[:]
			if len(id) >= 4 {
				status.UidValidity = uint32(id[0])<<24 | uint32(id[1])<<16 | uint32(id[2])<<8 | uint32(id[3])
			} else {
				status.UidValidity = 1
			}
		case imap.StatusRecent:
			status.Recent = 0
		}
	}
	// Set UnseenSeqNum for clients that expect it in SELECT response.
	if status.Unseen > 0 {
		status.UnseenSeqNum = 1
	}
	// Always populate Flags and PermanentFlags so SELECT sends required untagged responses.
	status.Flags = []string{imap.SeenFlag, imap.AnsweredFlag, imap.FlaggedFlag, imap.DeletedFlag, imap.DraftFlag}
	status.PermanentFlags = []string{imap.SeenFlag, imap.AnsweredFlag, imap.FlaggedFlag, imap.DeletedFlag, imap.DraftFlag}
	return status, nil
}

func (m *imapMailbox) SetSubscribed(subscribed bool) error { return nil }
func (m *imapMailbox) Check() error                        { return nil }

func (m *imapMailbox) ListMessages(uid bool, seqset *imap.SeqSet, items []imap.FetchItem, ch chan<- *imap.Message) error {
	defer close(ch)
	ctx := context.Background()
	msgs, _, err := m.backend.store.ListMessages(ctx, m.domainID, m.user.ID, &m.label.ID, mailstore.ListOptions{Limit: maxIMAPBatchSize})
	if err != nil {
		return err
	}

	var msgIDs []uuid.UUID
	for _, msg := range msgs {
		msgIDs = append(msgIDs, msg.ID)
	}
	flagMap, _ := m.backend.store.GetFlagsBatch(ctx, msgIDs)

	for i, msg := range msgs {
		seqNum := uint32(i + 1)
		msgUID := imapUID(m.backend.store, ctx, msg, m.user.ID, m.label.Name)
		if seqset != nil {
			if uid && !seqset.Contains(msgUID) {
				continue
			}
			if !uid && !seqset.Contains(seqNum) {
				continue
			}
		}

		im := imap.NewMessage(seqNum, items)
		for _, item := range items {
			switch item {
			case imap.FetchFlags:
				flags := flagMap[msg.ID]
				if msg.IsRead {
					flags = append(flags, imap.SeenFlag)
				}
				if msg.IsFlagged {
					flags = append(flags, imap.FlaggedFlag)
				}
				if msg.IsAnswered {
					flags = append(flags, imap.AnsweredFlag)
				}
				if msg.IsDraft {
					flags = append(flags, imap.DraftFlag)
				}
				im.Flags = flags
			case imap.FetchInternalDate:
				im.InternalDate = msg.CreatedAt
			case imap.FetchRFC822Size:
				im.Size = uint32(msg.SizeBytes)
			case imap.FetchUid:
				im.Uid = msgUID
			case imap.FetchEnvelope:
				im.Envelope = &imap.Envelope{
					Date:      msg.Date,
					Subject:   msg.Subject,
					From:      []*imap.Address{parseAddress(msg.FromAddress, msg.FromName)},
					To:        parseAddresses(msg.ToAddresses),
					Cc:        parseAddresses(msg.CcAddresses),
					Bcc:       parseAddresses(msg.BccAddresses),
					ReplyTo:   parseAddresses([]string{msg.ReplyTo}),
					InReplyTo: msg.InReplyTo,
					MessageId: msg.MessageIDHeader,
				}
			case imap.FetchRFC822:
				if len(msg.Source) > 0 {
					im.Body[&imap.BodySectionName{BodyPartName: imap.BodyPartName{Specifier: imap.EntireSpecifier}}] = bytes.NewReader(msg.Source)
				}
			case imap.FetchBody, imap.FetchBodyStructure:
				im.BodyStructure = &imap.BodyStructure{
					MIMEType:    "text",
					MIMESubType: "plain",
					Size:        uint32(len(msg.PlainText)),
				}
			}
		}
		ch <- im
	}
	return nil
}

func (m *imapMailbox) SearchMessages(uid bool, criteria *imap.SearchCriteria) ([]uint32, error) {
	ctx := context.Background()
	msgs, _, err := m.backend.store.ListMessages(ctx, m.domainID, m.user.ID, &m.label.ID, mailstore.ListOptions{Limit: maxIMAPBatchSize})
	if err != nil {
		return nil, err
	}
	var out []uint32
	for _, msg := range msgs {
		out = append(out, messageUID(msg))
	}
	return out, nil
}

func (m *imapMailbox) CreateMessage(flags []string, date time.Time, body imap.Literal) error {
	ctx := context.Background()

	b, err := io.ReadAll(body)
	if err != nil {
		return err
	}

	msg := &models.Message{
		ID:         uuid.Must(uuid.NewV7()),
		DomainID:   m.domainID,
		UserID:     m.user.ID,
		Source:     b,
		SizeBytes:  len(b),
		Date:       date,
		IsOutbound: false,
		IsRead:     false,
	}

	for _, f := range flags {
		switch f {
		case imap.SeenFlag:
			msg.IsRead = true
		case imap.FlaggedFlag:
			msg.IsFlagged = true
		case imap.AnsweredFlag:
			msg.IsAnswered = true
		case imap.DraftFlag:
			msg.IsDraft = true
		}
	}

	labelIDs := []uuid.UUID{m.label.ID}
	return m.backend.store.CreateMessage(ctx, msg, labelIDs, nil)
}

func (m *imapMailbox) UpdateMessagesFlags(uid bool, seqset *imap.SeqSet, operation imap.FlagsOp, flags []string) error {
	ctx := context.Background()
	msgs, _, err := m.backend.store.ListMessages(ctx, m.domainID, m.user.ID, &m.label.ID, mailstore.ListOptions{Limit: maxIMAPBatchSize})
	if err != nil {
		return err
	}

	for i, msg := range msgs {
		seqNum := uint32(i + 1)
		msgUID := imapUID(m.backend.store, ctx, msg, m.user.ID, m.label.Name)
		if seqset != nil {
			if uid && !seqset.Contains(msgUID) {
				continue
			}
			if !uid && !seqset.Contains(seqNum) {
				continue
			}
		}

		switch operation {
		case imap.AddFlags:
			for _, f := range flags {
				if err := m.backend.store.SetFlag(ctx, msg.ID, f); err != nil {
					return err
				}
				patch := flagToPatch(f, true)
				if patch != nil {
					_ = m.backend.store.UpdateMessage(ctx, m.domainID, m.user.ID, msg.ID, *patch)
				}
			}
		case imap.RemoveFlags:
			for _, f := range flags {
				if err := m.backend.store.ClearFlag(ctx, msg.ID, f); err != nil {
					return err
				}
				patch := flagToPatch(f, false)
				if patch != nil {
					_ = m.backend.store.UpdateMessage(ctx, m.domainID, m.user.ID, msg.ID, *patch)
				}
			}
		case imap.SetFlags:
			// Clear all known flags first
			for _, f := range []string{imap.SeenFlag, imap.FlaggedFlag, imap.AnsweredFlag, imap.DraftFlag, imap.DeletedFlag} {
				_ = m.backend.store.ClearFlag(ctx, msg.ID, f)
			}
			patch := mailstore.MessagePatch{IsRead: boolPtr(false), IsFlagged: boolPtr(false), IsAnswered: boolPtr(false), IsDraft: boolPtr(false)}
			_ = m.backend.store.UpdateMessage(ctx, m.domainID, m.user.ID, msg.ID, patch)
			// Set new flags
			for _, f := range flags {
				if err := m.backend.store.SetFlag(ctx, msg.ID, f); err != nil {
					return err
				}
				patch := flagToPatch(f, true)
				if patch != nil {
					_ = m.backend.store.UpdateMessage(ctx, m.domainID, m.user.ID, msg.ID, *patch)
				}
			}
		}
	}
	return nil
}

func flagToPatch(flag string, set bool) *mailstore.MessagePatch {
	switch flag {
	case imap.SeenFlag:
		return &mailstore.MessagePatch{IsRead: &set}
	case imap.FlaggedFlag:
		return &mailstore.MessagePatch{IsFlagged: &set}
	case imap.AnsweredFlag:
		return &mailstore.MessagePatch{IsAnswered: &set}
	case imap.DraftFlag:
		return &mailstore.MessagePatch{IsDraft: &set}
	}
	return nil
}

func boolPtr(b bool) *bool { return &b }

func (m *imapMailbox) CopyMessages(uid bool, seqset *imap.SeqSet, dest string) error {
	ctx := context.Background()
	destLabel, err := m.backend.store.GetLabelByName(ctx, m.domainID, m.user.ID, dest)
	if err != nil {
		return backend.ErrNoSuchMailbox
	}

	msgs, _, err := m.backend.store.ListMessages(ctx, m.domainID, m.user.ID, &m.label.ID, mailstore.ListOptions{Limit: maxIMAPBatchSize})
	if err != nil {
		return err
	}

	for i, msg := range msgs {
		seqNum := uint32(i + 1)
		msgUID := imapUID(m.backend.store, ctx, msg, m.user.ID, m.label.Name)
		if seqset != nil {
			if uid && !seqset.Contains(msgUID) {
				continue
			}
			if !uid && !seqset.Contains(seqNum) {
				continue
			}
		}
		if err := m.backend.store.ApplyLabels(ctx, msg.ID, []uuid.UUID{destLabel.ID}, nil); err != nil {
			return err
		}
	}
	return nil
}

func (m *imapMailbox) Expunge() error {
	ctx := context.Background()
	offset := 0
	for {
		opts := mailstore.ListOptions{Limit: maxIMAPBatchSize, Offset: offset}
		msgs, _, err := m.backend.store.ListMessages(ctx, m.domainID, m.user.ID, &m.label.ID, opts)
		if err != nil {
			return err
		}
		if len(msgs) == 0 {
			break
		}
		for _, msg := range msgs {
			flags, _ := m.backend.store.GetFlags(ctx, msg.ID)
			for _, f := range flags {
				if f == imap.DeletedFlag {
					if err := m.backend.store.ApplyLabels(ctx, msg.ID, nil, []uuid.UUID{m.label.ID}); err != nil {
						return err
					}
					break
				}
			}
		}
		offset += len(msgs)
		if len(msgs) < maxIMAPBatchSize {
			break
		}
	}
	return nil
}

func messageUID(msg *models.Message) uint32 {
	if len(msg.ID) < 4 {
		return 1
	}
	return uint32(msg.ID[0])<<24 | uint32(msg.ID[1])<<16 | uint32(msg.ID[2])<<8 | uint32(msg.ID[3])
}

func imapUID(store mailstore.Store, ctx context.Context, msg *models.Message, userID uuid.UUID, mailbox string) uint32 {
	uid, _, err := store.GetIMAPUID(ctx, msg.ID, userID, mailbox)
	if err != nil {
		// Fall back to UUID-derived UID
		return messageUID(msg)
	}
	if uid == 0 {
		// Not yet assigned; create one
		uid, _, err = store.GetOrCreateIMAPUID(ctx, msg.ID, userID, mailbox)
		if err != nil {
			return messageUID(msg)
		}
	}
	return uid
}

func parseAddress(addr, name string) *imap.Address {
	mailbox, host := "", ""
	if at := strings.Index(addr, "@"); at >= 0 {
		mailbox = addr[:at]
		host = addr[at+1:]
	}
	return &imap.Address{
		PersonalName: name,
		MailboxName:  mailbox,
		HostName:     host,
	}
}

func parseAddresses(addrs []string) []*imap.Address {
	var out []*imap.Address
	for _, a := range addrs {
		if a != "" {
			out = append(out, parseAddress(a, ""))
		}
	}
	return out
}
