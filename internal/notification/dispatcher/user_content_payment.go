package dispatcher

import (
	"fmt"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

func buildPaymentConfirmedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.PaymentPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Payment confirmed", "Pago confirmado", locale),
		body: localeStr(
			fmt.Sprintf("Your payment of %s has been confirmed.", formatCents(p.AmountCents, p.Currency)),
			fmt.Sprintf("Tu pago de %s ha sido confirmado.", formatCents(p.AmountCents, p.Currency)),
			locale,
		),
		actionURL: urlBalance,
	}
}

func buildPaymentFailedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.PaymentPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Payment failed", "Pago fallido", locale),
		body: localeStr(
			fmt.Sprintf("Your payment of %s could not be processed. %s", formatCents(p.AmountCents, p.Currency), p.Reason),
			fmt.Sprintf("Tu pago de %s no pudo procesarse. %s", formatCents(p.AmountCents, p.Currency), p.Reason),
			locale,
		),
		actionURL: "/api/v1/payment-intents",
	}
}

func buildPaymentBankTransferSubmittedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.BankTransferPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Bank transfer proof submitted", "Comprobante de transferencia enviado", locale),
		body: localeStr(
			fmt.Sprintf("Your transfer proof for %s has been submitted and is awaiting review.", formatCents(p.AmountCents, p.Currency)),
			fmt.Sprintf("Tu comprobante de transferencia por %s ha sido enviado y está pendiente de revisión.", formatCents(p.AmountCents, p.Currency)),
			locale,
		),
		actionURL: "/api/v1/bank-transfers",
	}
}

func buildPaymentBankTransferApprovedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.BankTransferPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Bank transfer approved", "Transferencia bancaria aprobada", locale),
		body: localeStr(
			fmt.Sprintf("%s has been credited to your account.", formatCents(p.AmountCents, p.Currency)),
			fmt.Sprintf("%s ha sido acreditado a tu cuenta.", formatCents(p.AmountCents, p.Currency)),
			locale,
		),
		actionURL: urlBalance,
	}
}

func buildPaymentBankTransferRejectedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.BankTransferPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Bank transfer rejected", "Transferencia bancaria rechazada", locale),
		body: localeStr(
			fmt.Sprintf("Your transfer proof for %s was rejected. %s", formatCents(p.AmountCents, p.Currency), p.Notes),
			fmt.Sprintf("Tu comprobante de transferencia por %s fue rechazado. %s", formatCents(p.AmountCents, p.Currency), p.Notes),
			locale,
		),
		actionURL: "/api/v1/bank-transfers",
	}
}

func buildPaymentPendingTimeoutContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.PaymentPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Payment expired", "Pago expirado", locale),
		body: localeStr(
			fmt.Sprintf("Your payment of %s has expired without confirmation. Please try again.", formatCents(p.AmountCents, p.Currency)),
			fmt.Sprintf("Tu pago de %s ha expirado sin confirmación. Por favor, inténtalo de nuevo.", formatCents(p.AmountCents, p.Currency)),
			locale,
		),
		actionURL: "/api/v1/payment-intents",
	}
}

func buildWithdrawalRequestedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.WithdrawalPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Withdrawal requested", "Retiro solicitado", locale),
		body: localeStr(
			fmt.Sprintf("Your withdrawal of %s is pending admin approval.", formatCents(p.AmountCents, p.Currency)),
			fmt.Sprintf("Tu retiro de %s está pendiente de aprobación del administrador.", formatCents(p.AmountCents, p.Currency)),
			locale,
		),
		actionURL: urlWithdrawals,
	}
}

func buildWithdrawalApprovedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.WithdrawalPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Withdrawal approved", "Retiro aprobado", locale),
		body: localeStr(
			fmt.Sprintf("Your withdrawal of %s has been approved.", formatCents(p.AmountCents, p.Currency)),
			fmt.Sprintf("Tu retiro de %s ha sido aprobado.", formatCents(p.AmountCents, p.Currency)),
			locale,
		),
		actionURL: urlWithdrawals,
	}
}

func buildWithdrawalRejectedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.WithdrawalPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Withdrawal rejected", "Retiro rechazado", locale),
		body: localeStr(
			fmt.Sprintf("Your withdrawal of %s was rejected. %s", formatCents(p.AmountCents, p.Currency), p.Notes),
			fmt.Sprintf("Tu retiro de %s fue rechazado. %s", formatCents(p.AmountCents, p.Currency), p.Notes),
			locale,
		),
		actionURL: urlWithdrawals,
	}
}

func buildWithdrawalCompletedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.WithdrawalPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Withdrawal completed", "Retiro completado", locale),
		body: localeStr(
			fmt.Sprintf("Your withdrawal of %s has been processed successfully.", formatCents(p.AmountCents, p.Currency)),
			fmt.Sprintf("Tu retiro de %s ha sido procesado exitosamente.", formatCents(p.AmountCents, p.Currency)),
			locale,
		),
		actionURL: urlBalance,
	}
}

func buildWithdrawalFailedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.WithdrawalPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Withdrawal failed", "Retiro fallido", locale),
		body: localeStr(
			fmt.Sprintf("Your withdrawal of %s could not be completed. Please contact support.", formatCents(p.AmountCents, p.Currency)),
			fmt.Sprintf("Tu retiro de %s no pudo completarse. Por favor, contacta al soporte.", formatCents(p.AmountCents, p.Currency)),
			locale,
		),
		actionURL: urlWithdrawals,
	}
}

func buildWithdrawalProcessingContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.WithdrawalPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Withdrawal being processed", "Retiro en proceso", locale),
		body: localeStr(
			fmt.Sprintf("Your withdrawal of %s is now being processed. Funds will be transferred shortly.", formatCents(p.AmountCents, p.Currency)),
			fmt.Sprintf("Tu retiro de %s está siendo procesado. Los fondos serán transferidos pronto.", formatCents(p.AmountCents, p.Currency)),
			locale,
		),
		actionURL: urlWithdrawals,
	}
}

func buildWithdrawalPendingTimeoutContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.WithdrawalPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Withdrawal request expired", "Solicitud de retiro expirada", locale),
		body: localeStr(
			fmt.Sprintf("Your withdrawal of %s has expired without admin action. Please submit a new request or contact support.", formatCents(p.AmountCents, p.Currency)),
			fmt.Sprintf("Tu retiro de %s ha expirado sin acción del administrador. Envía una nueva solicitud o contacta al soporte.", formatCents(p.AmountCents, p.Currency)),
			locale,
		),
		actionURL: urlWithdrawals,
	}
}
