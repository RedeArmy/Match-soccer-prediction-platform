package dispatcher

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// ── jsonInt ───────────────────────────────────────────────────────────────────

func TestJsonInt_Float64(t *testing.T) {
	if got := jsonInt(float64(42)); got != 42 {
		t.Errorf("jsonInt(float64(42)) = %d; want 42", got)
	}
}

func TestJsonInt_Int(t *testing.T) {
	if got := jsonInt(7); got != 7 {
		t.Errorf("jsonInt(7) = %d; want 7", got)
	}
}

func TestJsonInt_Int64(t *testing.T) {
	if got := jsonInt(int64(99)); got != 99 {
		t.Errorf("jsonInt(int64(99)) = %d; want 99", got)
	}
}

func TestJsonInt_UnsupportedType_ReturnsZero(t *testing.T) {
	if got := jsonInt("not a number"); got != 0 {
		t.Errorf("jsonInt(string) = %d; want 0", got)
	}
}

func TestJsonInt_Nil_ReturnsZero(t *testing.T) {
	if got := jsonInt(nil); got != 0 {
		t.Errorf("jsonInt(nil) = %d; want 0", got)
	}
}

// ── jsonStr ───────────────────────────────────────────────────────────────────

func TestJsonStr_String(t *testing.T) {
	if got := jsonStr("GTQ"); got != "GTQ" {
		t.Errorf("jsonStr(%q) = %q; want %q", "GTQ", got, "GTQ")
	}
}

func TestJsonStr_Nil_ReturnsEmpty(t *testing.T) {
	if got := jsonStr(nil); got != "" {
		t.Errorf("jsonStr(nil) = %q; want empty string", got)
	}
}

func TestJsonStr_NonString_ReturnsEmpty(t *testing.T) {
	if got := jsonStr(42); got != "" {
		t.Errorf("jsonStr(42) = %q; want empty string", got)
	}
}

// ── renderTmpl ────────────────────────────────────────────────────────────────

func TestRenderTmpl_PlainText(t *testing.T) {
	got, err := renderTmpl("hello world", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello world" {
		t.Errorf("got %q; want %q", got, "hello world")
	}
}

func TestRenderTmpl_WithData(t *testing.T) {
	data := map[string]any{"name": "Alice"}
	got, err := renderTmpl("Hello {{.name}}", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Hello Alice" {
		t.Errorf("got %q; want %q", got, "Hello Alice")
	}
}

func TestRenderTmpl_SyntaxError(t *testing.T) {
	_, err := renderTmpl("{{.unclosed", nil)
	if err == nil {
		t.Fatal("expected error for invalid template syntax, got nil")
	}
}

func TestRenderTmpl_FormatCentsFunction(t *testing.T) {
	data := map[string]any{"amount": float64(5000), "currency": "GTQ"}
	got, err := renderTmpl("{{formatCents .amount .currency}}", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "GTQ") {
		t.Errorf("expected GTQ in output, got %q", got)
	}
}

func TestRenderTmpl_IntFunction_AvoidsSciNotation(t *testing.T) {
	data := map[string]any{"match_id": float64(1_000_000)}
	got, err := renderTmpl("/matches/{{int .match_id}}", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/matches/1000000" {
		t.Errorf("got %q; want %q", got, "/matches/1000000")
	}
}

// ── RenderTemplate ────────────────────────────────────────────────────────────

func TestRenderTemplate_HappyPath(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl:     "Payment of {{formatCents .amount .currency}} confirmed",
		BodyTmpl:      "Your payment is confirmed.",
		ActionURLTmpl: "/payments/{{int .payment_id}}",
	}
	payload := json.RawMessage(`{"amount":5000,"currency":"GTQ","payment_id":42}`)

	title, body, actionURL, _, _, err := RenderTemplate(tmpl, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(title, "GTQ") {
		t.Errorf("title %q does not contain GTQ", title)
	}
	if body != "Your payment is confirmed." {
		t.Errorf("body = %q; want 'Your payment is confirmed.'", body)
	}
	if actionURL != "/payments/42" {
		t.Errorf("actionURL = %q; want '/payments/42'", actionURL)
	}
}

func TestRenderTemplate_NoActionURL_ReturnsEmptyString(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl: "Hello",
		BodyTmpl:  "World",
	}
	_, _, actionURL, _, _, err := RenderTemplate(tmpl, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actionURL != "" {
		t.Errorf("actionURL = %q; want empty string", actionURL)
	}
}

func TestRenderTemplate_InvalidPayload_ReturnsError(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl: "t",
		BodyTmpl:  "b",
	}
	_, _, _, _, _, err := RenderTemplate(tmpl, json.RawMessage(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON payload, got nil")
	}
}

func TestRenderTemplate_TitleSyntaxError_ReturnsError(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl: "{{.unclosed",
		BodyTmpl:  "valid",
	}
	_, _, _, _, _, err := RenderTemplate(tmpl, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for invalid title template, got nil")
	}
}

func TestRenderTemplate_BodySyntaxError_ReturnsError(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl: "valid title",
		BodyTmpl:  "{{.unclosed",
	}
	_, _, _, _, _, err := RenderTemplate(tmpl, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for invalid body template, got nil")
	}
}

func TestRenderTemplate_ActionURLSyntaxError_ReturnsError(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl:     "valid title",
		BodyTmpl:      "valid body",
		ActionURLTmpl: "{{.unclosed",
	}
	_, _, _, _, _, err := RenderTemplate(tmpl, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for invalid action_url template, got nil")
	}
}

func TestRenderTemplate_EmailSubjectTmpl_RenderedWithFuncs(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl:        "Title",
		BodyTmpl:         "Body",
		EmailSubjectTmpl: "Payment of {{formatCents .amount .currency}} confirmed",
	}
	payload := json.RawMessage(`{"amount":5000,"currency":"GTQ"}`)

	_, _, _, emailSubject, _, err := RenderTemplate(tmpl, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(emailSubject, "GTQ") {
		t.Errorf("emailSubject = %q; want string containing GTQ", emailSubject)
	}
}

func TestRenderTemplate_NoEmailSubjectTmpl_ReturnsEmpty(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl: "Title",
		BodyTmpl:  "Body",
	}
	_, _, _, emailSubject, _, err := RenderTemplate(tmpl, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if emailSubject != "" {
		t.Errorf("emailSubject = %q; want empty string (no template set)", emailSubject)
	}
}

func TestRenderTemplate_EmailSubjectSyntaxError_ReturnsError(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TitleTmpl:        "valid title",
		BodyTmpl:         "valid body",
		EmailSubjectTmpl: "{{.unclosed",
	}
	_, _, _, _, _, err := RenderTemplate(tmpl, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for invalid email_subject template, got nil")
	}
}

// ── fmtMoney ─────────────────────────────────────────────────────────────────

func TestFmtMoney_Zero(t *testing.T) {
	if got := fmtMoney(0); got != "Q 0.00" {
		t.Errorf("fmtMoney(0) = %q; want %q", got, "Q 0.00")
	}
}

func TestFmtMoney_SmallAmount(t *testing.T) {
	if got := fmtMoney(5000); got != "Q 50.00" {
		t.Errorf("fmtMoney(5000) = %q; want %q", got, "Q 50.00")
	}
}

func TestFmtMoney_WithThousandsSeparator(t *testing.T) {
	if got := fmtMoney(125000); got != "Q 1,250.00" {
		t.Errorf("fmtMoney(125000) = %q; want %q", got, "Q 1,250.00")
	}
}

func TestFmtMoney_LargeAmount(t *testing.T) {
	if got := fmtMoney(100000000); got != "Q 1,000,000.00" {
		t.Errorf("fmtMoney(100000000) = %q; want %q", got, "Q 1,000,000.00")
	}
}

func TestFmtMoney_NegativeAmount(t *testing.T) {
	if got := fmtMoney(-5000); got != "-Q 50.00" {
		t.Errorf("fmtMoney(-5000) = %q; want %q", got, "-Q 50.00")
	}
}

func TestFmtMoney_TemplateFunc_PipeCompatible(t *testing.T) {
	data := map[string]any{"amount_cents": float64(125000)}
	got, err := renderTmpl("{{.amount_cents | formatMoney}}", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Q 1,250.00" {
		t.Errorf("got %q; want %q", got, "Q 1,250.00")
	}
}

// ── fmtTime ──────────────────────────────────────────────────────────────────

func TestFmtTime_RFC3339String(t *testing.T) {
	got := fmtTime("2026-06-15T20:30:00Z")
	if got != "15/06 20:30" {
		t.Errorf("fmtTime(RFC3339) = %q; want %q", got, "15/06 20:30")
	}
}

func TestFmtTime_TimeValue(t *testing.T) {
	ts := time.Date(2026, 5, 19, 20, 30, 0, 0, time.UTC)
	if got := fmtTime(ts); got != "19/05 20:30" {
		t.Errorf("fmtTime(time.Time) = %q; want %q", got, "19/05 20:30")
	}
}

func TestFmtTime_InvalidString_ReturnsRaw(t *testing.T) {
	got := fmtTime("not-a-time")
	if got != "not-a-time" {
		t.Errorf("fmtTime(invalid) = %q; want raw input", got)
	}
}

func TestFmtTime_Nil_ReturnsEmpty(t *testing.T) {
	if got := fmtTime(nil); got != "" {
		t.Errorf("fmtTime(nil) = %q; want empty", got)
	}
}

func TestFmtTime_TemplateFunc_PipeCompatible(t *testing.T) {
	data := map[string]any{"deadline_at": "2026-06-15T20:30:00Z"}
	got, err := renderTmpl("{{.deadline_at | fmtTime}}", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "15/06 20:30" {
		t.Errorf("got %q; want %q", got, "15/06 20:30")
	}
}

// ── pluralize ─────────────────────────────────────────────────────────────────

func TestPluralize_Singular(t *testing.T) {
	data := map[string]any{"n": float64(1)}
	got, err := renderTmpl(`{{.n | pluralize "minuto" "minutos"}}`, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1 minuto" {
		t.Errorf("got %q; want %q", got, "1 minuto")
	}
}

func TestPluralize_Plural(t *testing.T) {
	data := map[string]any{"n": float64(30)}
	got, err := renderTmpl(`{{.n | pluralize "minuto" "minutos"}}`, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "30 minutos" {
		t.Errorf("got %q; want %q", got, "30 minutos")
	}
}

func TestPluralize_Zero_UsesPlural(t *testing.T) {
	data := map[string]any{"n": float64(0)}
	got, err := renderTmpl(`{{.n | pluralize "minuto" "minutos"}}`, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "0 minutos" {
		t.Errorf("got %q; want %q", got, "0 minutos")
	}
}
