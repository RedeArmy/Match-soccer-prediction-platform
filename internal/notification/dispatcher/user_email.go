package dispatcher

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	infraemail "github.com/rede/world-cup-quiniela/internal/infrastructure/email"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/unsubscribe"
)

// UserLocaleResolver resolves the stored locale preference for a user by ID.
// The production implementation wraps repository.UserRepository.GetByID.
// Returns domain.DefaultLocale on any error so that locale resolution never
// blocks notification delivery.
type UserLocaleResolver interface {
	ResolveLocaleByID(ctx context.Context, userID int) (domain.Locale, error)
}

// UserEmailResolver resolves the email address and display name for a user by ID.
// The production implementation wraps repository.UserRepository.GetByID.
// Tests supply a stub.
type UserEmailResolver interface {
	ResolveEmailByID(ctx context.Context, userID int) (email, name string, err error)
}

// userEmailData is the bag of values injected into the user-facing email template.
type userEmailData struct {
	Name             string
	Subject          string
	Headline         string
	Body             string
	ActionURL        string
	ActionLabel      string
	UnsubscribeURL   string // empty omits the unsubscribe link from the footer
	UnsubscribeLabel string // localised anchor text for the unsubscribe link
	GeneratedAt      string
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
  .footer a{color:#888;text-decoration:underline}
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
  <div class="footer">
    Sent at {{.GeneratedAt}} &bull; You are receiving this because you have an account on World Cup Quiniela.
    {{if .UnsubscribeURL}}&bull; <a href="{{.UnsubscribeURL}}">{{.UnsubscribeLabel}}</a>{{end}}
  </div>
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

	// Honour the user's global email opt-out (one-click unsubscribe link).
	if opted, _ := d.prefRepo.GlobalEmailOptedOut(ctx, userID); opted {
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

	unsubURL := ""
	if d.unsubscribeSecret != "" && d.appBaseURL != "" {
		tok := unsubscribe.SignToken(userID, d.unsubscribeSecret, timeNow())
		unsubURL = d.appBaseURL + "/api/v1/notifications/unsubscribe?token=" + tok
	}

	// Resolve relative action URLs to absolute so the CTA button in the email
	// is a working hyperlink.  Email clients interpret bare-path hrefs relative
	// to the mail service's domain, not the application origin.
	if d.appBaseURL != "" && strings.HasPrefix(content.actionURL, "/") {
		content.actionURL = d.appBaseURL + content.actionURL
	}

	renderTimeoutMs := domain.DefaultNotifyRenderTimeoutMs
	from := d.fromAddr
	if d.params != nil {
		renderTimeoutMs = d.params.GetInt(ctx, domain.ParamKeyNotifyRenderTimeoutMs, domain.DefaultNotifyRenderTimeoutMs)
		from = d.params.GetString(ctx, domain.ParamKeyNotifyFromAddress, d.fromAddr)
	}
	if from == "" {
		from = "World Cup Quiniela <noreply@quiniela.example.com>"
	}
	renderTimeout := time.Duration(renderTimeoutMs) * time.Millisecond

	var subject, html string
	renderErr := withRenderTimeout(ctx, renderTimeout, func() error {
		var e error
		if content.emailHTMLTmpl != "" {
			subject, html, e = renderUserEmailFromTmpl(content.emailHTMLTmpl, content, name, unsubURL)
		} else {
			subject, html, e = renderUserEmail(content, name, unsubURL)
		}
		return e
	})
	if renderErr != nil {
		log.Warn("user dispatcher: render user email failed",
			zap.Duration("render_timeout", renderTimeout),
			zap.Error(renderErr),
		)
		return
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
		d.recordEmailMetric(ctx, "failed")
		return
	}
	d.recordEmailMetric(ctx, "sent")
}

func (d *UserDispatcher) recordEmailMetric(ctx context.Context, status string) {
	if d.instruments.emails != nil {
		d.instruments.emails.Add(ctx, 1, metric.WithAttributes(attribute.String("status", status)))
	}
}

// timeNow is a package-level variable pointing to time.Now, allowing tests
// to inject a fixed clock when they need deterministic token expiry.
var timeNow = time.Now

// renderUserEmail returns the subject and rendered HTML body for a user event.
func renderUserEmail(content userContent, recipientName, unsubURL string) (subject, html string, err error) {
	data := buildUserEmailData(content, recipientName, unsubURL)
	var buf bytes.Buffer
	if tmplErr := userBaseTemplate.Execute(&buf, data); tmplErr != nil {
		return "", "", fmt.Errorf("dispatcher: render user email: %w", tmplErr)
	}
	return data.Subject, buf.String(), nil
}

// renderUserEmailFromTmpl renders a custom operator-supplied html/template string
// using the same userEmailData that the default userBaseTemplate receives.  This
// lets operators replace the entire email HTML while keeping access to all
// standard fields (.Headline, .Body, .ActionURL, .UnsubscribeURL, etc.).
func renderUserEmailFromTmpl(tmplStr string, content userContent, recipientName, unsubURL string) (subject, html string, err error) {
	t, parseErr := template.New("user-email-custom").Parse(tmplStr)
	if parseErr != nil {
		return "", "", fmt.Errorf("dispatcher: parse email_html_tmpl: %w", parseErr)
	}
	data := buildUserEmailData(content, recipientName, unsubURL)
	var buf bytes.Buffer
	if execErr := t.Execute(&buf, data); execErr != nil {
		return "", "", fmt.Errorf("dispatcher: render email_html_tmpl: %w", execErr)
	}
	return data.Subject, buf.String(), nil
}

func buildUserEmailData(content userContent, name, unsubURL string) userEmailData {
	locale := content.locale
	greeting := name
	if greeting == "" {
		greeting = localeStr("there", "amig@", locale)
	}
	hi := localeStr("Hi", "Hola", locale)
	subject := content.emailSubject
	if subject == "" {
		subject = content.title
	}
	return userEmailData{
		Name:             greeting,
		Subject:          subject,
		Headline:         content.title,
		Body:             fmt.Sprintf("%s %s, %s", hi, greeting, content.body),
		ActionURL:        content.actionURL,
		ActionLabel:      localeStr("Open app", "Abrir aplicación", locale),
		UnsubscribeURL:   unsubURL,
		UnsubscribeLabel: localeStr("Unsubscribe from emails", "Darse de baja de correos", locale),
		GeneratedAt:      time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	}
}
