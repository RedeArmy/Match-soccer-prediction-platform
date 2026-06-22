package dispatcher

import (
	"fmt"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

func buildAccountWelcomeContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	var p notification.AccountWelcomePayload
	if err := entry.DecodePayload(&p); err != nil {
		return userContent{}, err
	}
	return userContent{
		title: localeStr("Welcome to Kiniela!", "¡Bienvenido a Kiniela!", locale),
		body: localeStr(
			fmt.Sprintf("Hi %s! Your account is ready. Start predicting now.", p.UserName),
			fmt.Sprintf("¡Hola %s! Tu cuenta está lista. Empieza a predecir ahora.", p.UserName),
			locale,
		),
		actionURL: urlGroupsMe,
	}, nil
}

func buildAccountBalanceCreditedContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	return buildBalanceMovementContent(entry, locale,
		"Balance credited", "Saldo acreditado",
		"%s has been added to your account. New balance: %s.",
		"%s ha sido añadido a tu cuenta. Nuevo saldo: %s.",
	)
}

func buildAccountBalanceDebitedContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	return buildBalanceMovementContent(entry, locale,
		"Balance debited", "Saldo debitado",
		"%s has been deducted from your account. New balance: %s.",
		"%s ha sido deducido de tu cuenta. Nuevo saldo: %s.",
	)
}

func buildBalanceMovementContent(entry *notification.OutboxEntry, locale Locale, titleEN, titleES, bodyFmtEN, bodyFmtES string) (userContent, error) {
	var p notification.AccountBalancePayload
	if err := entry.DecodePayload(&p); err != nil {
		return userContent{}, err
	}
	return userContent{
		title: localeStr(titleEN, titleES, locale),
		body: localeStr(
			fmt.Sprintf(bodyFmtEN, formatCents(p.AmountCents, p.Currency), formatCents(p.BalanceAfter, p.Currency)),
			fmt.Sprintf(bodyFmtES, formatCents(p.AmountCents, p.Currency), formatCents(p.BalanceAfter, p.Currency)),
			locale,
		),
		actionURL: urlBalance,
	}, nil
}

func buildAccountLowBalanceContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	var p notification.AccountBalancePayload
	if err := entry.DecodePayload(&p); err != nil {
		return userContent{}, err
	}
	return userContent{
		title: localeStr("Low balance alert", "Alerta de saldo bajo", locale),
		body: localeStr(
			fmt.Sprintf("Your balance is %s. Top up to continue participating.", formatCents(p.BalanceAfter, p.Currency)),
			fmt.Sprintf("Tu saldo es %s. Recarga para seguir participando.", formatCents(p.BalanceAfter, p.Currency)),
			locale,
		),
		actionURL: urlBalance,
	}, nil
}
