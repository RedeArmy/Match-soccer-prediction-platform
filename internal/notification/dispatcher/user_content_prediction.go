package dispatcher

import (
	"fmt"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

func buildPredictionConfirmedContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	var p notification.PredictionConfirmedPayload
	if err := entry.DecodePayload(&p); err != nil {
		return userContent{}, err
	}
	return userContent{
		title: localeStr("Prediction confirmed", "Predicción confirmada", locale),
		body: localeStr(
			fmt.Sprintf("Your prediction for %s vs %s has been recorded.", p.HomeTeam, p.AwayTeam),
			fmt.Sprintf("Tu predicción para %s vs %s ha sido registrada.", p.HomeTeam, p.AwayTeam),
			locale,
		),
		actionURL: "/api/v1/predictions/me",
	}, nil
}

func buildPredictionDeadlineApproachContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	return buildPredictionDeadlineContent(entry, locale,
		"Prediction deadline approaching", "Límite de predicción se acerca",
		"%s vs %s kicks off in %d minutes — submit your prediction now.",
		"%s vs %s empieza en %d minutos — envía tu predicción ahora.",
	)
}

func buildPredictionMissingReminderContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	return buildPredictionDeadlineContent(entry, locale,
		"Missing prediction reminder", "Recordatorio de predicción pendiente",
		"You haven't predicted %s vs %s yet. Deadline is in %d minutes.",
		"Aún no has predicho %s vs %s. El límite cierra en %d minutos.",
	)
}

// buildPredictionDeadlineContent decodes a PredictionDeadlinePayload and
// constructs notification content from the given locale strings.
// enBodyFmt and esBodyFmt must supply three verbs: %s (HomeTeam), %s (AwayTeam), %d (MinutesLeft).
func buildPredictionDeadlineContent(entry *notification.OutboxEntry, locale Locale, enTitle, esTitle, enBodyFmt, esBodyFmt string) (userContent, error) {
	var p notification.PredictionDeadlinePayload
	if err := entry.DecodePayload(&p); err != nil {
		return userContent{}, err
	}
	return userContent{
		title: localeStr(enTitle, esTitle, locale),
		body: localeStr(
			fmt.Sprintf(enBodyFmt, p.HomeTeam, p.AwayTeam, p.MinutesLeft),
			fmt.Sprintf(esBodyFmt, p.HomeTeam, p.AwayTeam, p.MinutesLeft),
			locale,
		),
		actionURL: fmt.Sprintf(urlMatchDetail, p.MatchID),
	}, nil
}

func buildPredictionLockedContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	var p notification.PredictionLockedPayload
	if err := entry.DecodePayload(&p); err != nil {
		return userContent{}, err
	}
	return userContent{
		title: localeStr("Predictions locked", "Predicciones cerradas", locale),
		body: localeStr(
			fmt.Sprintf("Predictions for %s vs %s are now locked.", p.HomeTeam, p.AwayTeam),
			fmt.Sprintf("Las predicciones para %s vs %s ya están cerradas.", p.HomeTeam, p.AwayTeam),
			locale,
		),
		actionURL: fmt.Sprintf(urlMatchDetail, p.MatchID),
	}, nil
}

func buildPredictionScoredContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	var p notification.PredictionScoredPayload
	if err := entry.DecodePayload(&p); err != nil {
		return userContent{}, err
	}
	return userContent{
		title: localeStr("Match scored", "Partido puntuado", locale),
		body: localeStr(
			fmt.Sprintf("%s vs %s finished %d-%d. You earned %d points.", p.HomeTeam, p.AwayTeam, p.HomeScore, p.AwayScore, p.PointsEarned),
			fmt.Sprintf("%s vs %s terminó %d-%d. Ganaste %d puntos.", p.HomeTeam, p.AwayTeam, p.HomeScore, p.AwayScore, p.PointsEarned),
			locale,
		),
		actionURL: "/api/v1/predictions/me",
	}, nil
}

func buildMatchResultEnteredContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	return buildMatchEventContent(entry, locale,
		"Match result entered", "Resultado registrado",
		"The result for %s vs %s has been recorded.",
		"El resultado de %s vs %s ha sido registrado.",
	)
}

func buildMatchPostponedContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	return buildMatchEventContent(entry, locale,
		"Match postponed", "Partido aplazado",
		"%s vs %s has been postponed.",
		"%s vs %s ha sido aplazado.",
	)
}

func buildMatchCancelledContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	return buildMatchEventContent(entry, locale,
		"Match cancelled", "Partido cancelado",
		"%s vs %s has been cancelled.",
		"%s vs %s ha sido cancelado.",
	)
}

// buildMatchEventContent decodes a MatchEventPayload and constructs notification
// content from the given locale strings.
// enBodyFmt and esBodyFmt must supply two verbs: %s (HomeTeam) and %s (AwayTeam).
func buildMatchEventContent(entry *notification.OutboxEntry, locale Locale, enTitle, esTitle, enBodyFmt, esBodyFmt string) (userContent, error) {
	var p notification.MatchEventPayload
	if err := entry.DecodePayload(&p); err != nil {
		return userContent{}, err
	}
	return userContent{
		title: localeStr(enTitle, esTitle, locale),
		body: localeStr(
			fmt.Sprintf(enBodyFmt, p.HomeTeam, p.AwayTeam),
			fmt.Sprintf(esBodyFmt, p.HomeTeam, p.AwayTeam),
			locale,
		),
		actionURL: fmt.Sprintf(urlMatchDetail, p.MatchID),
	}, nil
}
