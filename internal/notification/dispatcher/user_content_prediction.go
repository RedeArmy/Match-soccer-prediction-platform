package dispatcher

import (
	"fmt"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

func buildPredictionConfirmedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.PredictionConfirmedPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Prediction confirmed", "Predicción confirmada", locale),
		body: localeStr(
			fmt.Sprintf("Your prediction for %s vs %s has been recorded.", p.HomeTeam, p.AwayTeam),
			fmt.Sprintf("Tu predicción para %s vs %s ha sido registrada.", p.HomeTeam, p.AwayTeam),
			locale,
		),
		actionURL: "/api/v1/predictions/me",
	}
}

func buildPredictionDeadlineApproachContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.PredictionDeadlinePayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Prediction deadline approaching", "Límite de predicción se acerca", locale),
		body: localeStr(
			fmt.Sprintf("%s vs %s kicks off in %d minutes — submit your prediction now.", p.HomeTeam, p.AwayTeam, p.MinutesLeft),
			fmt.Sprintf("%s vs %s empieza en %d minutos — envía tu predicción ahora.", p.HomeTeam, p.AwayTeam, p.MinutesLeft),
			locale,
		),
		actionURL: fmt.Sprintf(urlMatchDetail, p.MatchID),
	}
}

func buildPredictionMissingReminderContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.PredictionDeadlinePayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Missing prediction reminder", "Recordatorio de predicción pendiente", locale),
		body: localeStr(
			fmt.Sprintf("You haven't predicted %s vs %s yet. Deadline is in %d minutes.", p.HomeTeam, p.AwayTeam, p.MinutesLeft),
			fmt.Sprintf("Aún no has predicho %s vs %s. El límite cierra en %d minutos.", p.HomeTeam, p.AwayTeam, p.MinutesLeft),
			locale,
		),
		actionURL: fmt.Sprintf(urlMatchDetail, p.MatchID),
	}
}

func buildPredictionLockedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.PredictionLockedPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Predictions locked", "Predicciones cerradas", locale),
		body: localeStr(
			fmt.Sprintf("Predictions for %s vs %s are now locked.", p.HomeTeam, p.AwayTeam),
			fmt.Sprintf("Las predicciones para %s vs %s ya están cerradas.", p.HomeTeam, p.AwayTeam),
			locale,
		),
		actionURL: fmt.Sprintf(urlMatchDetail, p.MatchID),
	}
}

func buildPredictionScoredContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.PredictionScoredPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Match scored", "Partido puntuado", locale),
		body: localeStr(
			fmt.Sprintf("%s vs %s finished %d-%d. You earned %d points.", p.HomeTeam, p.AwayTeam, p.HomeScore, p.AwayScore, p.PointsEarned),
			fmt.Sprintf("%s vs %s terminó %d-%d. Ganaste %d puntos.", p.HomeTeam, p.AwayTeam, p.HomeScore, p.AwayScore, p.PointsEarned),
			locale,
		),
		actionURL: "/api/v1/predictions/me",
	}
}

func buildMatchResultEnteredContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.MatchEventPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Match result entered", "Resultado registrado", locale),
		body: localeStr(
			fmt.Sprintf("The result for %s vs %s has been recorded.", p.HomeTeam, p.AwayTeam),
			fmt.Sprintf("El resultado de %s vs %s ha sido registrado.", p.HomeTeam, p.AwayTeam),
			locale,
		),
		actionURL: fmt.Sprintf(urlMatchDetail, p.MatchID),
	}
}

func buildMatchPostponedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.MatchEventPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Match postponed", "Partido aplazado", locale),
		body: localeStr(
			fmt.Sprintf("%s vs %s has been postponed.", p.HomeTeam, p.AwayTeam),
			fmt.Sprintf("%s vs %s ha sido aplazado.", p.HomeTeam, p.AwayTeam),
			locale,
		),
		actionURL: fmt.Sprintf(urlMatchDetail, p.MatchID),
	}
}

func buildMatchCancelledContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.MatchEventPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Match cancelled", "Partido cancelado", locale),
		body: localeStr(
			fmt.Sprintf("%s vs %s has been cancelled.", p.HomeTeam, p.AwayTeam),
			fmt.Sprintf("%s vs %s ha sido cancelado.", p.HomeTeam, p.AwayTeam),
			locale,
		),
		actionURL: fmt.Sprintf(urlMatchDetail, p.MatchID),
	}
}
