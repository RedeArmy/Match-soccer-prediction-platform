package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	svix "github.com/svix/svix-webhooks/go"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// WebhookHandler handles incoming Clerk webhook events.
type WebhookHandler struct {
	userRepo         repository.UserRepository
	verifier         *svix.Webhook // nil means skip OR reject — see skipVerification
	skipVerification bool          // true only when WCQ_CLERK_WEBHOOKSECRET is intentionally absent (dev)
	log              *zap.Logger
}

// NewWebhookHandler constructs a WebhookHandler.
//
// webhookSecret is the "whsec_<base64>" value from the Clerk webhook dashboard.
// When empty, signature verification is skipped and a warning is logged —
// acceptable for local development only. Startup validation must reject this
// configuration outside development.
// When the secret is present but malformed, all webhook requests are rejected
// with 400 to prevent accepting unverified payloads silently.
func NewWebhookHandler(userRepo repository.UserRepository, webhookSecret string, log *zap.Logger) *WebhookHandler {
	h := &WebhookHandler{userRepo: userRepo, log: log}
	if webhookSecret == "" {
		log.Warn("WebhookHandler: WCQ_CLERK_WEBHOOKSECRET is not set — webhook signature verification is DISABLED; do not use in production")
		h.skipVerification = true
		return h
	}
	wh, err := svix.NewWebhook(webhookSecret)
	if err != nil {
		log.Error("WebhookHandler: invalid webhook secret format — all webhook requests will be rejected", zap.Error(err))
		return h // verifier=nil, skipVerification=false → always rejects
	}
	h.verifier = wh
	return h
}

// clerkEmailAddress is the email entry inside a Clerk user object.
type clerkEmailAddress struct {
	ID           string `json:"id"`
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

	if !h.skipVerification {
		if h.verifier == nil {
			// Secret was provided but is malformed; reject all requests.
			writeJSON(w, http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{Code: "bad_request", Message: "invalid webhook signature"},
			})
			return
		}
		if err := h.verifier.Verify(body, r.Header); err != nil {
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

// primaryEmail resolves the primary email address from the Clerk payload.
// It matches PrimaryEmailAddressID against the ID field of each address entry.
// Falls back to the first entry when no match is found (e.g. transient Clerk
// inconsistency) so user creation never fails on a missing primary pointer.
func primaryEmail(addrs []clerkEmailAddress, primaryID string) string {
	for _, a := range addrs {
		if a.ID == primaryID {
			return a.EmailAddress
		}
	}
	if len(addrs) > 0 {
		return addrs[0].EmailAddress
	}
	return ""
}

// upsertUser creates or updates an internal User from a Clerk user payload.
func (h *WebhookHandler) upsertUser(r *http.Request, data json.RawMessage) error {
	var payload clerkUserPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return apperrors.Validation("could not parse user data in webhook payload")
	}

	email := primaryEmail(payload.EmailAddresses, payload.PrimaryEmailID)
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
