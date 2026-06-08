package handler

import (
	"context"
	"fmt"

	"github.com/rede/world-cup-quiniela/pkg/recurrente"
)

// recurrenteCheckoutAdapter adapts pkg/recurrente.Client to the
// RecurrenteCheckoutCreator interface consumed by PaymentIntentHandler.
type recurrenteCheckoutAdapter struct {
	client *recurrente.Client
}

// NewRecurrenteCheckoutAdapter wraps a recurrente.Client as a RecurrenteCheckoutCreator.
func NewRecurrenteCheckoutAdapter(client *recurrente.Client) RecurrenteCheckoutCreator {
	return &recurrenteCheckoutAdapter{client: client}
}

func (a *recurrenteCheckoutAdapter) CreateCheckout(
	ctx context.Context,
	userID, amountCents int,
	currency, reference, successURL, cancelURL string,
) (string, error) {
	resp, err := a.client.CreateCheckout(ctx, recurrente.CheckoutRequest{
		Items: []recurrente.CheckoutItem{{
			Name:          "Depósito World Cup Quiniela",
			AmountInCents: amountCents,
			Currency:      currency,
			Quantity:      1,
		}},
		SuccessURL: successURL,
		CancelURL:  cancelURL,
		Metadata: map[string]any{
			"wcq_user_id":   userID,
			"wcq_reference": reference,
		},
	})
	if err != nil {
		return "", fmt.Errorf("recurrente adapter: %w", err)
	}
	return resp.CheckoutURL, nil
}

var _ RecurrenteCheckoutCreator = (*recurrenteCheckoutAdapter)(nil)
