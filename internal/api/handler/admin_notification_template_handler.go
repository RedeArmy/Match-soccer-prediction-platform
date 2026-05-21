package handler

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/notification/dispatcher"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// AdminNotificationTemplateHandler handles CRUD for operator-editable
// notification content templates.
type AdminNotificationTemplateHandler struct {
	repo repository.NotificationTemplateRepository
	log  *zap.Logger
}

// NewAdminNotificationTemplateHandler constructs the handler.
func NewAdminNotificationTemplateHandler(
	repo repository.NotificationTemplateRepository,
	log *zap.Logger,
) *AdminNotificationTemplateHandler {
	return &AdminNotificationTemplateHandler{repo: repo, log: log}
}

// NotificationTemplateResponse is the JSON representation of a stored template.
type NotificationTemplateResponse struct {
	EventType     string `json:"event_type"`
	Locale        string `json:"locale"`
	TitleTmpl     string `json:"title_tmpl"`
	BodyTmpl      string `json:"body_tmpl"`
	ActionURLTmpl string `json:"action_url_tmpl"`
	UpdatedBy     *int   `json:"updated_by,omitempty"`
	UpdatedAt     string `json:"updated_at"`
}

func templateToResponse(t *domain.NotificationTemplate) NotificationTemplateResponse {
	return NotificationTemplateResponse{
		EventType:     t.EventType,
		Locale:        t.Locale,
		TitleTmpl:     t.TitleTmpl,
		BodyTmpl:      t.BodyTmpl,
		ActionURLTmpl: t.ActionURLTmpl,
		UpdatedBy:     t.UpdatedBy,
		UpdatedAt:     t.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// List handles GET /admin/notification-templates.
//
// @Summary      List notification templates
// @Description  Returns all operator-editable notification content templates
//
//	stored in the database.  Rows not present default to the compiled Go
//	fallback during dispatch.  Requires admin role.
//
// @Tags         admin-notification-templates
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handler.NotificationTemplateResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/notification-templates [get]
func (h *AdminNotificationTemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	templates, err := h.repo.List(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	sort.Slice(templates, func(i, j int) bool {
		if templates[i].EventType != templates[j].EventType {
			return templates[i].EventType < templates[j].EventType
		}
		return templates[i].Locale < templates[j].Locale
	})
	resp := make([]NotificationTemplateResponse, len(templates))
	for i, t := range templates {
		resp[i] = templateToResponse(t)
	}
	writeJSON(w, http.StatusOK, resp)
}

// Get handles GET /admin/notification-templates/{event_type}/{locale}.
//
// @Summary      Get a notification template
// @Description  Returns the stored content override for the given
//
//	(event_type, locale) pair, or 404 when no override exists.
//
// @Tags         admin-notification-templates
// @Produce      json
// @Security     BearerAuth
// @Param        event_type  path      string  true  "Event type (e.g. payment.confirmed)"
// @Param        locale      path      string  true  "Locale (en or es)"
// @Success      200  {object}  handler.NotificationTemplateResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      404  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/notification-templates/{event_type}/{locale} [get]
func (h *AdminNotificationTemplateHandler) Get(w http.ResponseWriter, r *http.Request) {
	eventType, locale, ok := h.pathParams(w, r)
	if !ok {
		return
	}
	tmpl, err := h.repo.Get(r.Context(), eventType, locale)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if tmpl == nil {
		writeError(w, r, h.log, apperrors.NotFound("notification template not found"))
		return
	}
	writeJSON(w, http.StatusOK, templateToResponse(tmpl))
}

type upsertTemplateRequest struct {
	TitleTmpl     string `json:"title_tmpl"`
	BodyTmpl      string `json:"body_tmpl"`
	ActionURLTmpl string `json:"action_url_tmpl"`
}

// Upsert handles PUT /admin/notification-templates/{event_type}/{locale}.
//
// @Summary      Create or update a notification template
// @Description  Creates or replaces the content override for the given
//
//	(event_type, locale) pair.  Template syntax is validated before saving;
//	a dry-run render against a zero-value payload catches syntax errors
//	before the operator commits.  Requires admin role.
//
// @Tags         admin-notification-templates
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        event_type  path      string                             true  "Event type"
// @Param        locale      path      string                             true  "Locale"
// @Param        body        body      handler.upsertTemplateRequest      true  "Template content"
// @Success      200  {object}  handler.NotificationTemplateResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      422  {object}  handler.ErrorResponse  "Template syntax invalid"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/notification-templates/{event_type}/{locale} [put]
func (h *AdminNotificationTemplateHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	eventType, locale, ok := h.pathParams(w, r)
	if !ok {
		return
	}

	caller, callerOK := middleware.UserFromContext(r.Context())
	if !callerOK {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[upsertTemplateRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	if req.TitleTmpl == "" {
		writeError(w, r, h.log, apperrors.Validation("title_tmpl is required"))
		return
	}
	if req.BodyTmpl == "" {
		writeError(w, r, h.log, apperrors.Validation("body_tmpl is required"))
		return
	}

	candidate := &domain.NotificationTemplate{
		EventType:     eventType,
		Locale:        locale,
		TitleTmpl:     req.TitleTmpl,
		BodyTmpl:      req.BodyTmpl,
		ActionURLTmpl: req.ActionURLTmpl,
		UpdatedBy:     &caller.ID,
	}

	// Validate template syntax with a zero-value payload before persisting.
	if valErr := validateTemplateSyntax(candidate); valErr != nil {
		writeError(w, r, h.log, apperrors.Validation(valErr.Error()))
		return
	}

	if err := h.repo.Upsert(r.Context(), candidate); err != nil {
		writeError(w, r, h.log, err)
		return
	}

	saved, err := h.repo.Get(r.Context(), eventType, locale)
	if err != nil || saved == nil {
		// Upsert succeeded but cache miss is impossible here; return the candidate.
		writeJSON(w, http.StatusOK, templateToResponse(candidate))
		return
	}
	writeJSON(w, http.StatusOK, templateToResponse(saved))
}

// Delete handles DELETE /admin/notification-templates/{event_type}/{locale}.
//
// @Summary      Delete a notification template override
// @Description  Removes the stored content override for (event_type, locale).
//
//	Subsequent dispatches for this event/locale will use the compiled
//	Go default.  Returns 204 whether or not the row existed.
//
// @Tags         admin-notification-templates
// @Security     BearerAuth
// @Param        event_type  path  string  true  "Event type"
// @Param        locale      path  string  true  "Locale"
// @Success      204
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/notification-templates/{event_type}/{locale} [delete]
func (h *AdminNotificationTemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	eventType, locale, ok := h.pathParams(w, r)
	if !ok {
		return
	}
	if err := h.repo.Delete(r.Context(), eventType, locale); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type previewTemplateRequest struct {
	TitleTmpl     string          `json:"title_tmpl"`
	BodyTmpl      string          `json:"body_tmpl"`
	ActionURLTmpl string          `json:"action_url_tmpl"`
	SamplePayload json.RawMessage `json:"sample_payload"`
}

// PreviewResponse is the rendered result returned by the preview endpoint.
type PreviewResponse struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	ActionURL string `json:"action_url,omitempty"`
}

// Preview handles POST /admin/notification-templates/{event_type}/{locale}/preview.
//
// @Summary      Preview a notification template
// @Description  Renders the supplied template strings against the provided
//
//	sample_payload and returns the rendered title, body, and action URL.
//	Neither the template nor the payload are persisted.  Use this endpoint
//	to verify operator changes before calling PUT.  Requires admin role.
//
// @Tags         admin-notification-templates
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        event_type  path      string                              true  "Event type"
// @Param        locale      path      string                              true  "Locale"
// @Param        body        body      handler.previewTemplateRequest      true  "Templates + sample payload"
// @Success      200  {object}  handler.PreviewResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      422  {object}  handler.ErrorResponse  "Template syntax error or render error"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/notification-templates/{event_type}/{locale}/preview [post]
func (h *AdminNotificationTemplateHandler) Preview(w http.ResponseWriter, r *http.Request) {
	_, _, ok := h.pathParams(w, r)
	if !ok {
		return
	}

	req, err := decodeJSON[previewTemplateRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if req.TitleTmpl == "" {
		writeError(w, r, h.log, apperrors.Validation("title_tmpl is required"))
		return
	}
	if req.BodyTmpl == "" {
		writeError(w, r, h.log, apperrors.Validation("body_tmpl is required"))
		return
	}

	payload := req.SamplePayload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	tmpl := &domain.NotificationTemplate{
		TitleTmpl:     req.TitleTmpl,
		BodyTmpl:      req.BodyTmpl,
		ActionURLTmpl: req.ActionURLTmpl,
	}
	title, body, actionURL, renderErr := dispatcher.RenderTemplate(tmpl, payload)
	if renderErr != nil {
		writeError(w, r, h.log, apperrors.Validation(renderErr.Error()))
		return
	}
	writeJSON(w, http.StatusOK, PreviewResponse{Title: title, Body: body, ActionURL: actionURL})
}

// pathParams extracts and validates {event_type} and {locale} from the URL.
// Writes a 422 and returns false on validation failure.
func (h *AdminNotificationTemplateHandler) pathParams(w http.ResponseWriter, r *http.Request) (eventType, locale string, ok bool) {
	eventType = chi.URLParam(r, "event_type")
	locale = chi.URLParam(r, "locale")
	if eventType == "" {
		writeError(w, r, h.log, apperrors.Validation("event_type path parameter is required"))
		return "", "", false
	}
	if locale == "" {
		writeError(w, r, h.log, apperrors.Validation("locale path parameter is required"))
		return "", "", false
	}
	if locale != "en" && locale != "es" {
		writeError(w, r, h.log, apperrors.Validation("locale must be 'en' or 'es'"))
		return "", "", false
	}
	return eventType, locale, true
}

// validateTemplateSyntax parses each template string with a zero-value data map
// to catch syntax errors before persisting.  It does NOT run the full render so
// missing field references do not cause a validation failure.
func validateTemplateSyntax(t *domain.NotificationTemplate) error {
	for _, s := range []struct{ name, tmpl string }{
		{"title_tmpl", t.TitleTmpl},
		{"body_tmpl", t.BodyTmpl},
		{"action_url_tmpl", t.ActionURLTmpl},
	} {
		if s.tmpl == "" {
			continue
		}
		if _, _, _, err := dispatcher.RenderTemplate(&domain.NotificationTemplate{
			TitleTmpl:     s.tmpl,
			BodyTmpl:      s.tmpl,
			ActionURLTmpl: s.tmpl,
		}, json.RawMessage(`{}`)); err != nil {
			return err
		}
	}
	return nil
}
