package dispatcher

import (
	"strings"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
)

// ── ValidateTemplate ──────────────────────────────────────────────────────────

func TestValidateTemplate_HappyPath_StaticStrings(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl:     "Prediction confirmed",
		BodyTmpl:      "Your prediction has been recorded.",
		ActionURLTmpl: "/api/v1/predictions/me",
	}
	if err := ValidateTemplate(string(notification.EventPredictionConfirmed), tmpl); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateTemplate_HappyPath_WithEventVariables(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl:     "Prediction confirmed",
		BodyTmpl:      "Your prediction for {{.home_team}} vs {{.away_team}} has been recorded.",
		ActionURLTmpl: "/api/v1/matches/{{int .match_id}}",
	}
	if err := ValidateTemplate(string(notification.EventPredictionConfirmed), tmpl); err != nil {
		t.Errorf("unexpected error for valid template with event variables: %v", err)
	}
}

func TestValidateTemplate_HappyPath_FormatMoneyInBody(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl: "Payment confirmed",
		BodyTmpl:  "Your payment of {{.amount_cents | formatMoney}} has been confirmed.",
	}
	if err := ValidateTemplate(string(notification.EventPaymentConfirmed), tmpl); err != nil {
		t.Errorf("unexpected error for formatMoney template: %v", err)
	}
}

func TestValidateTemplate_HappyPath_FmtTimeInBody(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl: "Deadline approaching",
		BodyTmpl:  "Deadline: {{.deadline_at | fmtTime}}.",
	}
	if err := ValidateTemplate(string(notification.EventPredictionDeadlineApproach), tmpl); err != nil {
		t.Errorf("unexpected error for fmtTime template: %v", err)
	}
}

func TestValidateTemplate_HappyPath_PluralizeInBody(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl: "Deadline approaching",
		BodyTmpl:  `Kicks off in {{.minutes_left | pluralize "minute" "minutes"}}.`,
	}
	if err := ValidateTemplate(string(notification.EventPredictionDeadlineApproach), tmpl); err != nil {
		t.Errorf("unexpected error for pluralize template: %v", err)
	}
}

func TestValidateTemplate_HappyPath_EmailSubjectTmpl(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl:        "Payment confirmed",
		BodyTmpl:         "Your payment has been confirmed.",
		EmailSubjectTmpl: "Payment of {{.amount_cents | formatMoney}} confirmed",
	}
	if err := ValidateTemplate(string(notification.EventPaymentConfirmed), tmpl); err != nil {
		t.Errorf("unexpected error for email_subject_tmpl: %v", err)
	}
}

func TestValidateTemplate_UnknownEventType_ReturnsError(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl: "Hello",
		BodyTmpl:  "World",
	}
	err := ValidateTemplate("nonexistent.event", tmpl)
	if err == nil {
		t.Fatal("expected error for unknown event type, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent.event") {
		t.Errorf("error %q should mention the unknown event type", err.Error())
	}
}

func TestValidateTemplate_TitleUnknownVariable_ReturnsError(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl: "Hello {{.nonexistent_field}}",
		BodyTmpl:  "Body text.",
	}
	err := ValidateTemplate(string(notification.EventPredictionConfirmed), tmpl)
	if err == nil {
		t.Fatal("expected error for unknown variable in title_tmpl, got nil")
	}
	if !strings.Contains(err.Error(), "title_tmpl") {
		t.Errorf("error %q should mention title_tmpl", err.Error())
	}
}

func TestValidateTemplate_BodyUnknownVariable_ReturnsError(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl: "Prediction confirmed",
		BodyTmpl:  "Score: {{.wrong_score_field}}",
	}
	err := ValidateTemplate(string(notification.EventPredictionScored), tmpl)
	if err == nil {
		t.Fatal("expected error for unknown variable in body_tmpl, got nil")
	}
	if !strings.Contains(err.Error(), "body_tmpl") {
		t.Errorf("error %q should mention body_tmpl", err.Error())
	}
}

func TestValidateTemplate_ActionURLNotRelative_ReturnsError(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl:     "Prediction confirmed",
		BodyTmpl:      "Your prediction has been recorded.",
		ActionURLTmpl: "https://external.com/path",
	}
	err := ValidateTemplate(string(notification.EventPredictionConfirmed), tmpl)
	if err == nil {
		t.Fatal("expected error for non-relative action URL, got nil")
	}
	if !strings.Contains(err.Error(), "action_url_tmpl") {
		t.Errorf("error %q should mention action_url_tmpl", err.Error())
	}
}

func TestValidateTemplate_ActionURLSyntaxError_ReturnsError(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl:     "title",
		BodyTmpl:      "body",
		ActionURLTmpl: "{{.unclosed",
	}
	err := ValidateTemplate(string(notification.EventPredictionConfirmed), tmpl)
	if err == nil {
		t.Fatal("expected error for action_url_tmpl syntax error, got nil")
	}
}

func TestValidateTemplate_TitleSyntaxError_ReturnsError(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl: "{{.unclosed",
		BodyTmpl:  "body",
	}
	err := ValidateTemplate(string(notification.EventPredictionConfirmed), tmpl)
	if err == nil {
		t.Fatal("expected error for title_tmpl syntax error, got nil")
	}
}

func TestValidateTemplate_EmailSubjectUnknownVariable_ReturnsError(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl:        "Payment confirmed",
		BodyTmpl:         "Your payment has been confirmed.",
		EmailSubjectTmpl: "{{.no_such_field}} confirmed",
	}
	err := ValidateTemplate(string(notification.EventPaymentConfirmed), tmpl)
	if err == nil {
		t.Fatal("expected error for unknown variable in email_subject_tmpl, got nil")
	}
	if !strings.Contains(err.Error(), "email_subject_tmpl") {
		t.Errorf("error %q should mention email_subject_tmpl", err.Error())
	}
}

func TestValidateTemplate_WrongEventVariableForPaymentEvent_ReturnsError(t *testing.T) {
	// Template uses a prediction variable in a payment event — should fail.
	tmpl := &domain.NotificationTemplate{
		TitleTmpl: "Payment confirmed",
		BodyTmpl:  "Match: {{.home_team}} vs {{.away_team}}",
	}
	err := ValidateTemplate(string(notification.EventPaymentConfirmed), tmpl)
	if err == nil {
		t.Fatal("expected error when using prediction variables in payment event template, got nil")
	}
}

func TestValidateTemplate_AllKnownEventTypes_StaticTemplateIsAlwaysValid(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl: "Notification",
		BodyTmpl:  "You have a new notification.",
	}
	for et := range notification.KnownEventTypes {
		if err := ValidateTemplate(string(et), tmpl); err != nil {
			t.Errorf("static template should be valid for event type %q: %v", et, err)
		}
	}
}

// ── renderTmplStrict ──────────────────────────────────────────────────────────

func TestRenderTmplStrict_MissingKey_ReturnsError(t *testing.T) {
	_, err := renderTmplStrict("{{.nonexistent}}", map[string]any{"other": "value"})
	if err == nil {
		t.Fatal("expected error for missing key with missingkey=error, got nil")
	}
}

func TestRenderTmplStrict_PresentKey_Renders(t *testing.T) {
	got, err := renderTmplStrict("Hello {{.name}}", map[string]any{"name": "World"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Hello World" {
		t.Errorf("got %q; want %q", got, "Hello World")
	}
}

// ── validateEmailHTMLTmpl ─────────────────────────────────────────────────────

func TestValidateEmailHTMLTmpl_HappyPath_StaticHTML(t *testing.T) {
	err := validateEmailHTMLTmpl(`<!DOCTYPE html><html><body><p>Hello</p></body></html>`)
	if err != nil {
		t.Errorf("unexpected error for valid static HTML: %v", err)
	}
}

func TestValidateEmailHTMLTmpl_HappyPath_WithKnownFields(t *testing.T) {
	err := validateEmailHTMLTmpl(`<html><body><h1>{{.Headline}}</h1><p>{{.Body}}</p><a href="{{.ActionURL}}">{{.ActionLabel}}</a></body></html>`)
	if err != nil {
		t.Errorf("unexpected error for template using known userEmailData fields: %v", err)
	}
}

func TestValidateEmailHTMLTmpl_ParseError_ReturnsError(t *testing.T) {
	err := validateEmailHTMLTmpl(`{{.unclosed`)
	if err == nil {
		t.Fatal("expected parse error for malformed template, got nil")
	}
}

func TestValidateEmailHTMLTmpl_UnknownField_ReturnsError(t *testing.T) {
	err := validateEmailHTMLTmpl(`<p>{{.NonExistentField}}</p>`)
	if err == nil {
		t.Fatal("expected execute error for unknown struct field, got nil")
	}
}

func TestValidateTemplate_EmailHTMLTmpl_HappyPath(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl:     "Payment confirmed",
		BodyTmpl:      "Your payment is confirmed.",
		EmailHTMLTmpl: `<html><body><p>{{.Headline}}</p><p>{{.Body}}</p></body></html>`,
	}
	if err := ValidateTemplate(string(notification.EventPaymentConfirmed), tmpl); err != nil {
		t.Errorf("unexpected error for valid email_html_tmpl: %v", err)
	}
}

func TestValidateTemplate_EmailHTMLTmpl_Invalid_ReturnsError(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl:     "Payment confirmed",
		BodyTmpl:      "Your payment is confirmed.",
		EmailHTMLTmpl: `{{.unclosed`,
	}
	err := ValidateTemplate(string(notification.EventPaymentConfirmed), tmpl)
	if err == nil {
		t.Fatal("expected error for invalid email_html_tmpl, got nil")
	}
	if !strings.Contains(err.Error(), "email_html_tmpl") {
		t.Errorf("error %q should mention email_html_tmpl", err.Error())
	}
}
