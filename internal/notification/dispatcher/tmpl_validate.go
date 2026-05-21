package dispatcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	htmltemplate "html/template"
	"text/template"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
)

// ValidateTemplate checks that a NotificationTemplate is safe to persist for
// the given event type.  It performs three checks in order:
//
//  1. event_type must exist in notification.KnownEventTypes.
//  2. Every template field renders without error against a realistic sample
//     payload for the event type.  renderTmplStrict uses missingkey=error, so
//     any variable that does not exist in the payload (e.g. a typo like
//     {{.hometeam}} instead of {{.home_team}}) causes an immediate failure.
//  3. If action_url_tmpl is non-empty, the rendered path must start with "/".
func ValidateTemplate(eventType string, t *domain.NotificationTemplate) error {
	et := notification.EventType(eventType)
	if _, ok := notification.KnownEventTypes[et]; !ok {
		return fmt.Errorf("unknown event_type %q: not in the notification event catalogue", eventType)
	}

	sample := notification.SamplePayload(et)
	var data map[string]any
	if err := json.Unmarshal(sample, &data); err != nil {
		return fmt.Errorf("dispatcher: validate: internal sample payload error: %w", err)
	}

	if _, err := renderTmplStrict(t.TitleTmpl, data); err != nil {
		return fmt.Errorf("title_tmpl: %w", err)
	}
	if _, err := renderTmplStrict(t.BodyTmpl, data); err != nil {
		return fmt.Errorf("body_tmpl: %w", err)
	}
	if t.ActionURLTmpl != "" {
		actionURL, err := renderTmplStrict(t.ActionURLTmpl, data)
		if err != nil {
			return fmt.Errorf("action_url_tmpl: %w", err)
		}
		if len(actionURL) > 0 && actionURL[0] != '/' {
			return fmt.Errorf("action_url_tmpl must render to a relative path starting with '/'; got %q", actionURL)
		}
	}
	if t.EmailSubjectTmpl != "" {
		if _, err := renderTmplStrict(t.EmailSubjectTmpl, data); err != nil {
			return fmt.Errorf("email_subject_tmpl: %w", err)
		}
	}
	if t.EmailHTMLTmpl != "" {
		if err := validateEmailHTMLTmpl(t.EmailHTMLTmpl); err != nil {
			return fmt.Errorf("email_html_tmpl: %w", err)
		}
	}
	return nil
}

// validateEmailHTMLTmpl parses and dry-runs an email_html_tmpl string with
// html/template against a fully-populated userEmailData sample so that typos
// in field references are caught at save time rather than at delivery time.
func validateEmailHTMLTmpl(tmplStr string) error {
	t, err := htmltemplate.New("email-html").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("dispatcher: parse email_html_tmpl: %w", err)
	}
	sample := userEmailData{
		Name:             "Juan García",
		Subject:          "Payment confirmed",
		Headline:         "Payment confirmed",
		Body:             "Hi Juan García, your payment is confirmed.",
		ActionURL:        "https://quiniela.example.com/api/v1/users/me/balance",
		ActionLabel:      "Open app",
		UnsubscribeURL:   "https://quiniela.example.com/api/v1/notifications/unsubscribe?token=sample",
		UnsubscribeLabel: "Unsubscribe from emails",
		GeneratedAt:      "2026-05-21 08:00:00 UTC",
	}
	if err := t.Execute(bytes.NewBuffer(nil), sample); err != nil {
		return fmt.Errorf("dispatcher: execute email_html_tmpl: %w", err)
	}
	return nil
}

// renderTmplStrict parses and executes a template string with missingkey=error.
// Any reference to a map key that is absent in data returns an error, making
// this function suitable for template validation (not production rendering).
func renderTmplStrict(tmplStr string, data map[string]any) (string, error) {
	t, err := template.New("").Funcs(notifTemplateFuncs).Option("missingkey=error").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("dispatcher: parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("dispatcher: execute template: %w", err)
	}
	return buf.String(), nil
}
