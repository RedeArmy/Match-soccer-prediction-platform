package dispatcher

import (
	"fmt"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

func buildAccountWelcomeContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.AccountWelcomePayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Welcome to World Cup Quiniela!", "¡Bienvenido a World Cup Quiniela!", locale),
		body: localeStr(
			fmt.Sprintf("Hi %s! Your account is ready. Start predicting now.", p.UserName),
			fmt.Sprintf("¡Hola %s! Tu cuenta está lista. Empieza a predecir ahora.", p.UserName),
			locale,
		),
		actionURL: urlGroupsMe,
	}
}

func buildAccountBalanceCreditedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.AccountBalancePayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Balance credited", "Saldo acreditado", locale),
		body: localeStr(
			fmt.Sprintf("%s has been added to your account. New balance: %s.", formatCents(p.AmountCents, p.Currency), formatCents(p.BalanceAfter, p.Currency)),
			fmt.Sprintf("%s ha sido añadido a tu cuenta. Nuevo saldo: %s.", formatCents(p.AmountCents, p.Currency), formatCents(p.BalanceAfter, p.Currency)),
			locale,
		),
		actionURL: urlBalance,
	}
}

func buildAccountBalanceDebitedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.AccountBalancePayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Balance debited", "Saldo debitado", locale),
		body: localeStr(
			fmt.Sprintf("%s has been deducted from your account. New balance: %s.", formatCents(p.AmountCents, p.Currency), formatCents(p.BalanceAfter, p.Currency)),
			fmt.Sprintf("%s ha sido deducido de tu cuenta. Nuevo saldo: %s.", formatCents(p.AmountCents, p.Currency), formatCents(p.BalanceAfter, p.Currency)),
			locale,
		),
		actionURL: urlBalance,
	}
}

func buildAccountLowBalanceContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.AccountBalancePayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Low balance alert", "Alerta de saldo bajo", locale),
		body: localeStr(
			fmt.Sprintf("Your balance is %s. Top up to continue participating.", formatCents(p.BalanceAfter, p.Currency)),
			fmt.Sprintf("Tu saldo es %s. Recarga para seguir participando.", formatCents(p.BalanceAfter, p.Currency)),
			locale,
		),
		actionURL: urlBalance,
	}
}
