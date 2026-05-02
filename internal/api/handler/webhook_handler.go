package handler

import (
	"encoding/json"
	"io"
	"net/http"

	svix "github.com/svix/svix-webhooks/go"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// WebhookHandler handles incoming Clerk webhook events.
type WebhookHandler struct {
	syncer           service.ClerkUserSyncer
	verifier         *svix.Webhook // nil means skip OR reject - see skipVerification
	skipVerification bool          // true only when WCQ_CLERK_WEBHOOKSECRET is intentionally absent (dev)
	log              *zap.Logger
}

// NewWebhookHandler constructs a WebhookHandler.
//
// webhookSecret is the "whsec_<base64>" value from the Clerk webhook dashboard.
// When empty, signature verification is skipped and a warning is logged -
// acceptable for local development only. Startup validation must reject this
// configuration outside development.
// When the secret is present but malformed, all webhook requests are rejected
// with 400 to prevent accepting unverified payloads silently.
func NewWebhookHandler(syncer service.ClerkUserSyncer, webhookSecret string, log *zap.Logger) *WebhookHandler {
	h := &WebhookHandler{syncer: syncer, log: log}
	if webhookSecret == "" {
		log.Warn("WebhookHandler: WCQ_CLERK_WEBHOOKSECRET is not set - webhook signature verification is DISABLED; do not use in production")
		h.skipVerification = true
		return h
	}
	wh, err := svix.NewWebhook(webhookSecret)
	if err != nil {
		log.Error("WebhookHandler: invalid webhook secret format - all webhook requests will be rejected", zap.Error(err))
		return h // verifier=nil, skipVerification=false -> always rejects
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
		writeError(w, r, h.log, apperrors.Internal(err))
		return
	}

	if !h.skipVerification {
		if h.verifier == nil {
			// Secret was provided but is malformed; reject all requests.
			writeError(w, r, h.log, apperrors.BadRequest("invalid webhook configuration"))
			return
		}
		if err := h.verifier.Verify(body, r.Header); err != nil {
			h.log.Warn("webhook signature verification failed", zap.Error(err))
			writeError(w, r, h.log, apperrors.BadRequest("invalid webhook signature"))
			return
		}
	}

	var event clerkWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeError(w, r, h.log, apperrors.Validation("could not parse webhook payload"))
		return
	}

	switch event.Type {
	case "user.created", "user.updated":
		if err := h.syncUser(r, event.Data); err != nil {
			writeError(w, r, h.log, err)
			return
		}
	default:
		// Unknown event types are acknowledged without processing so Clerk
		// does not retry them indefinitely.
		h.log.Debug("webhook: ignoring unknown event type", zap.String("type", event.Type))
	}

	w.WriteHeader(http.StatusNoContent)
}

// syncUser parses the Clerk user payload and delegates persistence to the
// ClerkUserSyncer service. All business logic (email resolution, name
// normalisation, create-or-update) lives in the service layer.
func (h *WebhookHandler) syncUser(r *http.Request, data json.RawMessage) error {
	var payload clerkUserPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return apperrors.Validation("could not parse user data in webhook payload")
	}

	emails := make([]service.ClerkEmail, len(payload.EmailAddresses))
	for i, a := range payload.EmailAddresses {
		emails[i] = service.ClerkEmail{ID: a.ID, Address: a.EmailAddress}
	}

	return h.syncer.Upsert(
		r.Context(),
		payload.ID,
		payload.FirstName,
		payload.LastName,
		payload.PrimaryEmailID,
		emails,
	)
}
