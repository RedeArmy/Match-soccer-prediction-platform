package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// WebhookHandler handles incoming Clerk webhook events.
type WebhookHandler struct {
	userRepo      repository.UserRepository
	webhookSecret string
	log           *zap.Logger
}

// NewWebhookHandler constructs a WebhookHandler.
//
// webhookSecret is the "whsec_<base64>" value from the Clerk webhook dashboard.
// When empty, signature verification is skipped and a warning is logged —
// acceptable for local development, never for production.
func NewWebhookHandler(userRepo repository.UserRepository, webhookSecret string, log *zap.Logger) *WebhookHandler {
	if webhookSecret == "" {
		log.Warn("WebhookHandler: WCQ_CLERK_WEBHOOKSECRET is not set — webhook signature verification is DISABLED; do not use in production")
	}
	return &WebhookHandler{userRepo: userRepo, webhookSecret: webhookSecret, log: log}
}

// clerkEmailAddress is the email entry inside a Clerk user object.
type clerkEmailAddress struct {
	EmailAddress string `json:"email_address"`
}

// clerkUserPayload is the subset of Clerk's user object we care about.
type clerkUserPayload struct {
	ID             string              `json:"id"`
	FirstName      string              `json:"first_name"`
	LastName       string              `json:"last_name"`
	EmailAddresses []clerkEmailAddress `json:"email_addresses"`
	PrimaryEmailID string              `json:"primary_email_address_id"`
}

// clerkWebhookEvent is the top-level envelope for Clerk webhook payloads.
type clerkWebhookEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// HandleClerkWebhook handles POST /webhooks/clerk.
//
// @Summary      Clerk webhook receiver
// @Description  Receives and processes Clerk user lifecycle events (user.created, user.updated).
//
//	Requests are authenticated via Svix signature validation using the webhook
//	secret configured in WCQ_CLERK_WEBHOOKSECRET.
//
// @Tags         webhooks
// @Accept       json
// @Produce      json
// @Success      204  "Event processed"
// @Failure      400  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /webhooks/clerk [post]
func (h *WebhookHandler) HandleClerkWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB limit
	if err != nil {
		middleware.WriteError(w, r, h.log, apperrors.Internal(err))
		return
	}

	if h.webhookSecret != "" {
		if err := h.verifySvixSignature(r, body); err != nil {
			h.log.Warn("webhook signature verification failed", zap.Error(err))
			writeJSON(w, http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{Code: "bad_request", Message: "invalid webhook signature"},
			})
			return
		}
	}

	var event clerkWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		middleware.WriteError(w, r, h.log, apperrors.Validation("could not parse webhook payload"))
		return
	}

	switch event.Type {
	case "user.created", "user.updated":
		if err := h.upsertUser(r, event.Data); err != nil {
			middleware.WriteError(w, r, h.log, err)
			return
		}
	default:
		// Unknown event types are acknowledged without processing so Clerk
		// does not retry them indefinitely.
		h.log.Debug("webhook: ignoring unknown event type", zap.String("type", event.Type))
	}

	w.WriteHeader(http.StatusNoContent)
}

// upsertUser creates or updates an internal User from a Clerk user payload.
func (h *WebhookHandler) upsertUser(r *http.Request, data json.RawMessage) error {
	var payload clerkUserPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return apperrors.Validation("could not parse user data in webhook payload")
	}

	email := ""
	if len(payload.EmailAddresses) > 0 {
		email = payload.EmailAddresses[0].EmailAddress
	}
	if email != "" {
		if err := domain.ValidateEmail(email); err != nil {
			return apperrors.Validation("webhook payload contains an invalid email address")
		}
	}

	name := strings.TrimSpace(payload.FirstName + " " + payload.LastName)
	if name == "" {
		name = payload.ID
	}

	existing, err := h.userRepo.GetByClerkSubject(r.Context(), payload.ID)
	if err != nil {
		return apperrors.Internal(err)
	}

	if existing != nil {
		existing.Name = name
		existing.Email = email
		existing.ClerkSubject = payload.ID
		if err := h.userRepo.Update(r.Context(), existing); err != nil {
			return err
		}
		h.log.Info("webhook: updated user",
			zap.Int("user_id", existing.ID),
			zap.String("clerk_subject", payload.ID),
		)
		return nil
	}

	user := &domain.User{
		Name:         name,
		Email:        email,
		ClerkSubject: payload.ID,
		Role:         domain.RolePlayer,
	}
	if err := h.userRepo.Create(r.Context(), user); err != nil {
		return err
	}
	h.log.Info("webhook: created user",
		zap.Int("user_id", user.ID),
		zap.String("clerk_subject", payload.ID),
	)
	return nil
}

// verifySvixSignature validates the Svix HMAC-SHA256 signature on the request.
//
// The signed payload is "{svix-id}.{svix-timestamp}.{body}".
// The secret is base64-decoded from the "whsec_<base64>" format.
func (h *WebhookHandler) verifySvixSignature(r *http.Request, body []byte) error {
	msgID := r.Header.Get("svix-id")
	msgTimestamp := r.Header.Get("svix-timestamp")
	msgSignature := r.Header.Get("svix-signature")

	if msgID == "" || msgTimestamp == "" || msgSignature == "" {
		return errors.New("missing svix headers")
	}

	ts, err := strconv.ParseInt(msgTimestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid svix-timestamp: %w", err)
	}
	if age := time.Since(time.Unix(ts, 0)).Abs(); age > 5*time.Minute {
		return fmt.Errorf("webhook timestamp too old: %s", age)
	}

	secretBase64 := strings.TrimPrefix(h.webhookSecret, "whsec_")
	secretBytes, err := base64.StdEncoding.DecodeString(secretBase64)
	if err != nil {
		return fmt.Errorf("could not decode webhook secret: %w", err)
	}

	toSign := fmt.Sprintf("%s.%s.%s", msgID, msgTimestamp, string(body))
	mac := hmac.New(sha256.New, secretBytes)
	mac.Write([]byte(toSign))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	// The header may contain multiple space-separated "v1,<sig>" entries.
	for _, sig := range strings.Fields(msgSignature) {
		if parts := strings.SplitN(sig, ",", 2); len(parts) == 2 && parts[0] == "v1" {
			if hmac.Equal([]byte(parts[1]), []byte(expected)) {
				return nil
			}
		}
	}
	return errors.New("no valid svix signature found")
}
