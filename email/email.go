//go:generate protoc -I emailpb --gogofaster_out=plugins=grpc:emailpb emailpb/qemail.proto

package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/mail"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"gopkg.qsoa.cloud/qdevrunner/email/emailpb"
)

type MailboxConfig struct {
	Address      string
	Smtp         string
	SmtpPassword string
	Imap         string
	ImapPassword string
}

type Email struct {
	emailpb.UnimplementedQEmailServer
	mailboxes map[string]*MailboxConfig // address -> config
	ids       map[string]uint64        // address -> ID
	addrs     map[uint64]string        // ID -> address
}

func New(mailboxes map[string]*MailboxConfig) *Email {
	ids := make(map[string]uint64, len(mailboxes))
	addrs := make(map[uint64]string, len(mailboxes))
	var nextID uint64
	for addr := range mailboxes {
		nextID++
		ids[addr] = nextID
		addrs[nextID] = addr
	}
	return &Email{
		mailboxes: mailboxes,
		ids:       ids,
		addrs:     addrs,
	}
}

func (s *Email) ResolveMailbox(_ context.Context, req *emailpb.ResolveMailboxReq) (*emailpb.ResolveMailboxResp, error) {
	id, ok := s.ids[req.Address]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "mailbox %q not configured", req.Address)
	}
	return &emailpb.ResolveMailboxResp{MailboxId: id}, nil
}

func (s *Email) SendEmail(_ context.Context, req *emailpb.SendEmailReq) (*emailpb.SendEmailResp, error) {
	addr, ok := s.addrs[req.MailboxId]
	if !ok {
		return nil, status.Error(codes.NotFound, "mailbox not found")
	}
	cfg := s.mailboxes[addr]
	if cfg.Smtp == "" {
		return nil, status.Error(codes.FailedPrecondition, "SMTP not configured for this mailbox")
	}

	// Compose RFC 5322 message.
	var buf bytes.Buffer
	var h mail.Header
	h.SetDate(time.Now())
	h.SetAddressList("From", []*mail.Address{{Name: req.FromName, Address: addr}})
	h.SetAddressList("To", parseAddresses(req.To))
	if len(req.Cc) > 0 {
		h.SetAddressList("Cc", parseAddresses(req.Cc))
	}
	h.SetSubject(req.Subject)

	domain := addr
	if idx := strings.IndexByte(addr, '@'); idx >= 0 {
		domain = addr[idx+1:]
	}
	h.GenerateMessageIDWithHostname(domain)
	msgID, _ := h.MessageID()

	for _, kv := range req.Headers {
		switch strings.ToLower(kv.Key) {
		case "from", "to", "cc", "subject", "date", "message-id", "mime-version", "content-type":
			continue
		}
		h.Set(kv.Key, kv.Value)
	}

	hasHTML := req.HtmlBody != ""
	hasAttachments := len(req.Attachments) > 0

	switch {
	case hasAttachments:
		mw, err := mail.CreateWriter(&buf, h)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "create writer: %v", err)
		}

		iw, err := mw.CreateInline()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "create inline: %v", err)
		}

		var th mail.InlineHeader
		th.Set("Content-Type", "text/plain; charset=utf-8")
		pw, err := iw.CreatePart(th)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "create text part: %v", err)
		}
		io.WriteString(pw, req.TextBody)
		pw.Close()

		if hasHTML {
			var hh mail.InlineHeader
			hh.Set("Content-Type", "text/html; charset=utf-8")
			pw, err = iw.CreatePart(hh)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "create html part: %v", err)
			}
			io.WriteString(pw, req.HtmlBody)
			pw.Close()
		}

		iw.Close()

		for _, att := range req.Attachments {
			var ah mail.AttachmentHeader
			ah.Set("Content-Type", att.ContentType)
			ah.SetFilename(att.Filename)
			aw, err := mw.CreateAttachment(ah)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "create attachment: %v", err)
			}
			aw.Write(att.Data)
			aw.Close()
		}

		mw.Close()

	case hasHTML:
		iw, err := mail.CreateInlineWriter(&buf, h)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "create inline writer: %v", err)
		}

		var th mail.InlineHeader
		th.Set("Content-Type", "text/plain; charset=utf-8")
		pw, err := iw.CreatePart(th)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "create text part: %v", err)
		}
		io.WriteString(pw, req.TextBody)
		pw.Close()

		var hh mail.InlineHeader
		hh.Set("Content-Type", "text/html; charset=utf-8")
		pw, err = iw.CreatePart(hh)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "create html part: %v", err)
		}
		io.WriteString(pw, req.HtmlBody)
		pw.Close()

		iw.Close()

	default:
		h.Set("Content-Type", "text/plain; charset=utf-8")
		pw, err := mail.CreateSingleInlineWriter(&buf, h)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "create writer: %v", err)
		}
		io.WriteString(pw, req.TextBody)
		pw.Close()
	}

	// Send via SMTP.
	recipients := append(append(req.To, req.Cc...), req.Bcc...)
	if err := sendSMTP(cfg.Smtp, addr, cfg.SmtpPassword, addr, recipients, buf.Bytes()); err != nil {
		return nil, status.Errorf(codes.Internal, "SMTP send: %v", err)
	}

	return &emailpb.SendEmailResp{MessageId: msgID}, nil
}

func (s *Email) ListMessages(_ context.Context, req *emailpb.ListMessagesReq) (*emailpb.ListMessagesResp, error) {
	var result *emailpb.ListMessagesResp
	err := s.withIMAP(req.MailboxId, func(c *imapclient.Client) error {
		folder := req.Folder
		if folder == "" {
			folder = "INBOX"
		}
		selectData, err := c.Select(folder, nil).Wait()
		if err != nil {
			return status.Errorf(codes.Internal, "SELECT %s: %v", folder, err)
		}

		total := selectData.NumMessages
		result = &emailpb.ListMessagesResp{Total: total}

		if total == 0 || req.Offset >= total {
			return nil
		}

		// IMAP sequence numbers: 1=oldest, N=newest. Fetch newest first.
		end := total - req.Offset
		limit := req.Limit
		if limit == 0 {
			limit = 50
		}
		var start uint32
		if end > limit {
			start = end - limit + 1
		} else {
			start = 1
		}

		var seqSet imap.SeqSet
		seqSet.AddRange(start, end)

		fetchCmd := c.Fetch(seqSet, &imap.FetchOptions{
			UID:      true,
			Flags:    true,
			Envelope: true,
			RFC822Size: true,
		})
		msgs, err := fetchCmd.Collect()
		if err != nil {
			return status.Errorf(codes.Internal, "FETCH: %v", err)
		}

		// Reverse so newest first.
		summaries := make([]*emailpb.MessageSummary, len(msgs))
		for i, msg := range msgs {
			env := msg.Envelope
			var from string
			if env != nil && len(env.From) > 0 {
				from = formatAddress(&env.From[0])
			}
			var subject string
			var date int64
			if env != nil {
				subject = env.Subject
				date = env.Date.Unix()
			}

			var seen, flagged bool
			for _, f := range msg.Flags {
				switch f {
				case imap.FlagSeen:
					seen = true
				case imap.FlagFlagged:
					flagged = true
				}
			}

			var toAddrs []string
			if env != nil {
				for _, a := range env.To {
					toAddrs = append(toAddrs, a.Addr())
				}
			}

			summaries[len(msgs)-1-i] = &emailpb.MessageSummary{
				Uid:     strconv.FormatUint(uint64(msg.UID), 10),
				From:    from,
				Subject: subject,
				Date:    date,
				Seen:    seen,
				Flagged: flagged,
				Size_:   uint32(msg.RFC822Size),
				To:      toAddrs,
			}
		}
		result.Messages = summaries
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Email) GetMessage(_ context.Context, req *emailpb.GetMessageReq) (*emailpb.GetMessageResp, error) {
	var result *emailpb.GetMessageResp
	err := s.withIMAP(req.MailboxId, func(c *imapclient.Client) error {
		folder := req.Folder
		if folder == "" {
			folder = "INBOX"
		}
		if _, err := c.Select(folder, nil).Wait(); err != nil {
			return status.Errorf(codes.Internal, "SELECT %s: %v", folder, err)
		}

		uid, err := strconv.ParseUint(req.Uid, 10, 32)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid UID: %v", err)
		}

		bodySection := &imap.FetchItemBodySection{
			Specifier: imap.PartSpecifierNone,
			Peek:      true,
		}
		fetchCmd := c.Fetch(imap.UIDSetNum(imap.UID(uid)), &imap.FetchOptions{
			UID:         true,
			BodySection: []*imap.FetchItemBodySection{bodySection},
		})
		msgs, err := fetchCmd.Collect()
		if err != nil {
			return status.Errorf(codes.Internal, "FETCH: %v", err)
		}
		if len(msgs) == 0 {
			return status.Error(codes.NotFound, "message not found")
		}

		body := msgs[0].FindBodySection(bodySection)
		if body == nil {
			return status.Error(codes.Internal, "no body in FETCH response")
		}

		mr, err := mail.CreateReader(bytes.NewReader(body))
		if err != nil {
			return status.Errorf(codes.Internal, "parse message: %v", err)
		}

		result = &emailpb.GetMessageResp{}

		if date, err := mr.Header.Date(); err == nil {
			result.Date = date.Unix()
		}
		if from, err := mr.Header.AddressList("From"); err == nil && len(from) > 0 {
			if from[0].Name != "" {
				result.From = fmt.Sprintf("%s <%s>", from[0].Name, from[0].Address)
			} else {
				result.From = from[0].Address
			}
		}
		if toList, err := mr.Header.AddressList("To"); err == nil {
			for _, a := range toList {
				result.To = append(result.To, a.Address)
			}
		}
		if ccList, err := mr.Header.AddressList("Cc"); err == nil {
			for _, a := range ccList {
				result.Cc = append(result.Cc, a.Address)
			}
		}
		if subject, err := mr.Header.Subject(); err == nil {
			result.Subject = subject
		}

		var headerBuf bytes.Buffer
		fields := mr.Header.Fields()
		for fields.Next() {
			fmt.Fprintf(&headerBuf, "%s: %s\n", fields.Key(), fields.Value())
		}
		result.RawHeaders = headerBuf.String()

		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}

			switch h := p.Header.(type) {
			case *mail.InlineHeader:
				ct, _, _ := h.ContentType()
				data, err := io.ReadAll(p.Body)
				if err != nil {
					continue
				}
				switch {
				case strings.HasPrefix(ct, "text/plain"):
					result.TextBody = string(data)
				case strings.HasPrefix(ct, "text/html"):
					result.HtmlBody = string(data)
				}
			case *mail.AttachmentHeader:
				filename, _ := h.Filename()
				ct, _, _ := h.ContentType()
				data, err := io.ReadAll(p.Body)
				if err != nil {
					continue
				}
				result.Attachments = append(result.Attachments, &emailpb.Attachment{
					Filename:    filename,
					ContentType: ct,
					Data:        data,
				})
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Email) DeleteMessage(_ context.Context, req *emailpb.DeleteMessageReq) (*emptypb.Empty, error) {
	err := s.withIMAP(req.MailboxId, func(c *imapclient.Client) error {
		folder := req.Folder
		if folder == "" {
			folder = "INBOX"
		}
		if _, err := c.Select(folder, nil).Wait(); err != nil {
			return status.Errorf(codes.Internal, "SELECT %s: %v", folder, err)
		}

		uid, err := strconv.ParseUint(req.Uid, 10, 32)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid UID: %v", err)
		}

		uidSet := imap.UIDSetNum(imap.UID(uid))

		storeCmd := c.Store(uidSet, &imap.StoreFlags{
			Op:     imap.StoreFlagsAdd,
			Silent: true,
			Flags:  []imap.Flag{imap.FlagDeleted},
		}, nil)
		if err := storeCmd.Close(); err != nil {
			return status.Errorf(codes.Internal, "STORE: %v", err)
		}

		if err := c.Expunge().Close(); err != nil {
			return status.Errorf(codes.Internal, "EXPUNGE: %v", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *Email) MoveMessage(_ context.Context, req *emailpb.MoveMessageReq) (*emptypb.Empty, error) {
	err := s.withIMAP(req.MailboxId, func(c *imapclient.Client) error {
		uid, err := strconv.ParseUint(req.Uid, 10, 32)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid UID: %v", err)
		}

		// Search all folders for the UID.
		listCmd := c.List("", "*", nil)
		folders, err := listCmd.Collect()
		if err != nil {
			return status.Errorf(codes.Internal, "LIST: %v", err)
		}

		uidSet := imap.UIDSetNum(imap.UID(uid))

		for _, f := range folders {
			if _, err := c.Select(f.Mailbox, nil).Wait(); err != nil {
				continue
			}

			fetchCmd := c.Fetch(uidSet, &imap.FetchOptions{UID: true})
			msgs, err := fetchCmd.Collect()
			if err != nil || len(msgs) == 0 {
				continue
			}

			// Found the message in this folder, move it.
			if _, err := c.Move(uidSet, req.ToFolder).Wait(); err != nil {
				return status.Errorf(codes.Internal, "MOVE: %v", err)
			}
			return nil
		}

		return status.Error(codes.NotFound, "message not found in any folder")
	})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *Email) withIMAP(mailboxID uint64, fn func(c *imapclient.Client) error) error {
	addr, ok := s.addrs[mailboxID]
	if !ok {
		return status.Error(codes.NotFound, "mailbox not found")
	}
	cfg := s.mailboxes[addr]
	if cfg.Imap == "" {
		return status.Error(codes.FailedPrecondition, "IMAP not configured for this mailbox")
	}

	c, err := imapclient.DialTLS(cfg.Imap, &imapclient.Options{
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
	})
	if err != nil {
		return status.Errorf(codes.Unavailable, "IMAP connect: %v", err)
	}
	defer c.Close()

	if err := c.Login(addr, cfg.ImapPassword).Wait(); err != nil {
		return status.Errorf(codes.Unauthenticated, "IMAP login: %v", err)
	}

	if err := fn(c); err != nil {
		return err
	}

	c.Logout().Wait()
	return nil
}

func sendSMTP(server, username, password, from string, to []string, msg []byte) error {
	host, _, err := net.SplitHostPort(server)
	if err != nil {
		host = server
	}

	conn, err := net.DialTimeout("tcp", server, 10*time.Second)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("client: %w", err)
	}
	defer c.Close()

	// Try STARTTLS.
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true,
		}); err != nil {
			return fmt.Errorf("STARTTLS: %w", err)
		}
	}

	// Authenticate if password is set.
	if password != "" {
		auth := &plainAuth{username: username, password: password}
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("AUTH: %w", err)
		}
	}

	if err := c.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("RCPT TO %s: %w", rcpt, err)
		}
	}

	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}

	return c.Quit()
}

// plainAuth implements smtp.Auth without host checking (needed for dev use with localhost).
type plainAuth struct {
	username, password string
}

func (a *plainAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	resp := []byte("\x00" + a.username + "\x00" + a.password)
	return "PLAIN", resp, nil
}

func (a *plainAuth) Next([]byte, bool) ([]byte, error) {
	return nil, nil
}

func formatAddress(addr *imap.Address) string {
	email := addr.Addr()
	if addr.Name != "" {
		return fmt.Sprintf("%s <%s>", addr.Name, email)
	}
	return email
}

func parseAddresses(addrs []string) []*mail.Address {
	result := make([]*mail.Address, len(addrs))
	for i, a := range addrs {
		result[i] = &mail.Address{Address: a}
	}
	return result
}
