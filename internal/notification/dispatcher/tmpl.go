package dispatcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"text/template"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// notifTemplateFuncs is the FuncMap registered on every content template.
// It mirrors the helper functions available in the compiled Go defaults so
// that operators writing DB templates see identical behaviour.
//
// All numeric values arrive as float64 from JSON unmarshalling.
// The int function converts them to int64, which renders as a plain integer
// in template output — essential for URL path segments (e.g.
// "/api/v1/groups/{{int .quiniela_id}}/members").
var notifTemplateFuncs = template.FuncMap{
	// formatCents formats a cent value as "50.00 GTQ".
	// Accepts float64 (JSON default) and int.
	"formatCents": func(centsAny, currencyAny any) string {
		return formatCents(jsonInt(centsAny), jsonStr(currencyAny))
	},
	// formatMoney is a pipeline-friendly variant of formatCents for GTQ amounts.
	// Usage: {{.amount_cents | formatMoney}} → "Q 1,250.00"
	"formatMoney": func(centsAny any) string {
		return fmtMoney(jsonInt(centsAny))
	},
	// fmtTime formats a time value as "DD/MM HH:MM" (UTC).
	// Accepts time.Time or an RFC3339 string (the shape JSON produces).
	// Usage: {{.deadline_at | fmtTime}} → "19/05 20:30"
	"fmtTime": func(v any) string { return fmtTime(v) },
	// pluralize returns "N singular" when N == 1, otherwise "N plural".
	// Usage: {{.minutes_left | pluralize "minuto" "minutos"}} → "30 minutos"
	"pluralize": func(singular, plural string, count any) string {
		n := jsonInt(count)
		word := plural
		if n == 1 {
			word = singular
		}
		return fmt.Sprintf("%d %s", n, word)
	},
	// int converts a JSON float64 to int64 for safe use in URL templates.
	"int": func(v any) int64 { return int64(jsonInt(v)) },
}

// ── helper functions ──────────────────────────────────────────────────────────

// fmtMoney formats an integer cent value as a GTQ amount string.
// Example: fmtMoney(125000) → "Q 1,250.00"
func fmtMoney(cents int) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	whole := cents / 100
	frac := cents % 100
	return fmt.Sprintf("%sQ %s.%02d", sign, commaInt(whole), frac)
}

// commaInt formats a non-negative integer with thousands-separator commas.
// Example: commaInt(1250000) → "1,250,000"
func commaInt(n int) string {
	s := strconv.Itoa(n)
	out := make([]byte, 0, len(s)+(len(s)-1)/3)
	for i, b := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, b)
	}
	return string(out)
}

// fmtTime formats a time value as "DD/MM HH:MM" (UTC).
// Accepts time.Time or an RFC3339/RFC3339Nano string; returns the raw value
// for unsupported types or unparseable strings.
func fmtTime(v any) string {
	const layout = "02/01 15:04"
	var t time.Time
	switch val := v.(type) {
	case time.Time:
		t = val
	case string:
		var err error
		for _, tfmt := range []string{time.RFC3339Nano, time.RFC3339} {
			if t, err = time.Parse(tfmt, val); err == nil {
				break
			}
		}
		if err != nil {
			return val
		}
	default:
		return ""
	}
	return t.UTC().Format(layout)
}

// jsonInt converts a JSON-decoded number (float64, int, int64) to int.
func jsonInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

// jsonStr returns v as a string, or "" when v is nil or an unsupported type.
func jsonStr(v any) string {
	s, _ := v.(string)
	return s
}

// renderTmpl parses and executes a single Go text/template string against data.
func renderTmpl(tmplStr string, data map[string]any) (string, error) {
	t, err := template.New("").Funcs(notifTemplateFuncs).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("dispatcher: parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("dispatcher: execute template: %w", err)
	}
	return buf.String(), nil
}

// RenderTemplate renders title, body, action URL, and optional email subject
// from a stored template using the raw outbox payload as template data.  It is
// exported so the admin preview handler can call it without depending on
// UserDispatcher internals.
//
// emailSubject is empty when EmailSubjectTmpl is not set; callers should fall
// back to the notification title in that case.
//
// emailHTMLTmpl is the raw template string from EmailHTMLTmpl, passed through
// without rendering so the caller can later render it with html/template against
// a fully-populated userEmailData struct that includes recipient-specific fields.
//
// Returns ("", "", "", "", "", error) when the payload cannot be decoded or any
// text/template field is syntactically invalid.
func RenderTemplate(t *domain.NotificationTemplate, payload json.RawMessage) (title, body, actionURL, emailSubject, emailHTMLTmpl string, err error) {
	var data map[string]any
	if err = json.Unmarshal(payload, &data); err != nil {
		return "", "", "", "", "", fmt.Errorf("dispatcher: decode payload: %w", err)
	}
	if title, err = renderTmpl(t.TitleTmpl, data); err != nil {
		return "", "", "", "", "", err
	}
	if body, err = renderTmpl(t.BodyTmpl, data); err != nil {
		return "", "", "", "", "", err
	}
	if t.ActionURLTmpl != "" {
		if actionURL, err = renderTmpl(t.ActionURLTmpl, data); err != nil {
			return "", "", "", "", "", err
		}
	}
	if t.EmailSubjectTmpl != "" {
		if emailSubject, err = renderTmpl(t.EmailSubjectTmpl, data); err != nil {
			return "", "", "", "", "", err
		}
	}
	return title, body, actionURL, emailSubject, t.EmailHTMLTmpl, nil
}
