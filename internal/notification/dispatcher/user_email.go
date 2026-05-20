package dispatcher

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"

	"go.uber.org/zap"

	infraemail "github.com/rede/world-cup-quiniela/internal/infrastructure/email"
	"github.com/rede/world-cup-quiniela/internal/notification"
)

// UserEmailResolver resolves the email address and display name for a user by ID.
// The production implementation wraps repository.UserRepository.GetByID.
// Tests supply a stub.
type UserEmailResolver interface {
	ResolveEmailByID(ctx context.Context, userID int) (email, name string, err error)
}

// userEmailData is the bag of values injected into the user-facing email template.
type userEmailData struct {
	Name        string
	Subject     string
	Headline    string
	Body        string
	ActionURL   string
	ActionLabel string
	GeneratedAt string
}

var userBaseTemplate = template.Must(template.New("user-base").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Subject}}</title>
<style>
  body{font-family:Arial,sans-serif;background:#f4f4f4;margin:0;padding:0}
  .wrap{max-width:600px;margin:40px auto;background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,.1)}
  .header{background:#1a1a2e;color:#fff;padding:24px 32px}
  .header h1{margin:0;font-size:20px;letter-spacing:.5px}
  .content{padding:32px}
  .content h2{color:#1a1a2e;margin-top:0;font-size:18px}
  .content p{color:#444;line-height:1.6}
  .cta{display:inline-block;margin-top:24px;padding:12px 24px;background:#e74c3c;color:#fff;text-decoration:none;border-radius:4px;font-weight:700;font-size:14px}
  .footer{background:#f8f9fa;padding:16px 32px;font-size:12px;color:#888;text-align:center}
</style>
</head>
<body>
<div class="wrap">
  <div class="header">
    <h1>World Cup Quiniela</h1>
  </div>
  <div class="content">
    <h2>{{.Headline}}</h2>
    <p>{{.Body}}</p>
    {{if .ActionURL}}<a class="cta" href="{{.ActionURL}}">{{.ActionLabel}}</a>{{end}}
  </div>
  <div class="footer">Sent at {{.GeneratedAt}} &bull; You are receiving this because you have an account on World Cup Quiniela.</div>
</div>
</body>
</html>`))

// deliverEmail resolves the user's email address, renders the user-facing
// HTML email, and delivers it via the mailer.  On delivery failure the error
// is written to the DLQ and logged; it is not propagated so a single failed
// email channel does not block the outbox worker retry loop.
func (d *UserDispatcher) deliverEmail(
	ctx context.Context,
	entry *notification.OutboxEntry,
	userID int,
	content userContent,
	log *zap.Logger,
) {
	if d.mailer == nil || d.emailResolver == nil {
		return
	}

	toAddr, name, err := d.emailResolver.ResolveEmailByID(ctx, userID)
	if err != nil {
		log.Warn("user dispatcher: resolve user email failed",
			zap.Int("user_id", userID),
			zap.Error(err),
		)
		return
	}

	subject, html, err := renderUserEmail(content, name)
	if err != nil {
		log.Warn("user dispatcher: render user email failed", zap.Error(err))
		return
	}

	from := d.fromAddr
	if from == "" {
		from = "World Cup Quiniela <noreply@quiniela.example.com>"
	}

	_, sendErr := d.mailer.Send(ctx, infraemail.Message{
		From:    from,
		To:      []string{toAddr},
		Subject: subject,
		HTML:    html,
	})
	if sendErr != nil {
		log.Warn("user dispatcher: email send failed",
			zap.Int("user_id", userID),
			zap.String("event_type", string(entry.EventType)),
			zap.Error(sendErr),
		)
		d.writeDLQEntry(ctx, entry, userID, "email", sendErr)
	}
}

// renderUserEmail returns the subject and rendered HTML body for a user event.
func renderUserEmail(content userContent, recipientName string) (subject, html string, err error) {
	data := buildUserEmailData(content, recipientName)
	var buf bytes.Buffer
	if tmplErr := userBaseTemplate.Execute(&buf, data); tmplErr != nil {
		return "", "", fmt.Errorf("dispatcher: render user email: %w", tmplErr)
	}
	return data.Subject, buf.String(), nil
}

func buildUserEmailData(content userContent, name string) userEmailData {
	greeting := name
	if greeting == "" {
		greeting = "there"
	}
	return userEmailData{
		Name:        greeting,
		Subject:     content.title,
		Headline:    content.title,
		Body:        fmt.Sprintf("Hi %s, %s", greeting, content.body),
		ActionURL:   content.actionURL,
		ActionLabel: "Open app",
		GeneratedAt: time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	}
}
