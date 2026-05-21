package dispatcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"

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
	// int converts a JSON float64 to int64 for safe use in URL templates.
	"int": func(v any) int64 { return int64(jsonInt(v)) },
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

// RenderTemplate renders title, body, and action URL from a stored template
// using the raw outbox payload as template data.  It is exported so the admin
// preview handler can call it without depending on UserDispatcher internals.
//
// Returns ("", "", "", error) when the payload cannot be decoded or a template
// string is syntactically invalid.
func RenderTemplate(t *domain.NotificationTemplate, payload json.RawMessage) (title, body, actionURL string, err error) {
	var data map[string]any
	if err = json.Unmarshal(payload, &data); err != nil {
		return "", "", "", fmt.Errorf("dispatcher: decode payload: %w", err)
	}
	if title, err = renderTmpl(t.TitleTmpl, data); err != nil {
		return "", "", "", err
	}
	if body, err = renderTmpl(t.BodyTmpl, data); err != nil {
		return "", "", "", err
	}
	if t.ActionURLTmpl != "" {
		if actionURL, err = renderTmpl(t.ActionURLTmpl, data); err != nil {
			return "", "", "", err
		}
	}
	return title, body, actionURL, nil
}
