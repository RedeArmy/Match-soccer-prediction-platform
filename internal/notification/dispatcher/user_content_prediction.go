package dispatcher

import (
	"fmt"
	"html"
	"html/template"
	"strings"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

// teamNamesES maps English FIFA team names to their Spanish equivalents.
// Names not present in the map are returned unchanged (many are identical).
var teamNamesES = map[string]string{
	// UEFA
	"Germany":        "Alemania",
	"France":         "Francia",
	"Spain":          "España",
	"England":        "Inglaterra",
	"Netherlands":    "Países Bajos",
	"Belgium":        "Bélgica",
	"Italy":          "Italia",
	"Poland":         "Polonia",
	"Switzerland":    "Suiza",
	"Croatia":        "Croacia",
	"Denmark":        "Dinamarca",
	"Turkey":         "Turquía",
	"Scotland":       "Escocia",
	"Hungary":        "Hungría",
	"Czech Republic": "República Checa",
	"Romania":        "Rumanía",
	"Slovakia":       "Eslovaquia",
	"Slovenia":       "Eslovenia",
	"Norway":         "Noruega",
	"Sweden":         "Suecia",
	"Finland":        "Finlandia",
	"Greece":         "Grecia",
	"Albania":        "Albania",
	"Serbia":         "Serbia",
	"Austria":        "Austria",
	"Portugal":       "Portugal",
	// CONMEBOL
	"Brazil": "Brasil",
	"Peru":   "Perú",
	// CONCACAF
	"United States": "Estados Unidos",
	"USA":           "Estados Unidos",
	"Canada":        "Canadá",
	"Mexico":        "México",
	"Panama":        "Panamá",
	// CAF
	"Morocco":      "Marruecos",
	"Cameroon":     "Camerún",
	"Egypt":        "Egipto",
	"South Africa": "Sudáfrica",
	"Ivory Coast":  "Costa de Marfil",
	"DR Congo":     "RD Congo",
	"Algeria":      "Argelia",
	"Tunisia":      "Túnez",
	"Mali":         "Malí",
	"Kenya":        "Kenia",
	"Ethiopia":     "Etiopía",
	// AFC
	"Japan":        "Japón",
	"South Korea":  "Corea del Sur",
	"Iran":         "Irán",
	"Saudi Arabia": "Arabia Saudita",
	"Uzbekistan":   "Uzbekistán",
	"Qatar":        "Catar",
	"Iraq":         "Irak",
	"Jordan":       "Jordania",
	"Bahrain":      "Baréin",
	// OFC
	"New Zealand": "Nueva Zelanda",
}

// translateTeamName returns the Spanish name for a team when locale is Spanish,
// falling back to the original name if no translation is registered.
func translateTeamName(name string, locale Locale) string {
	if locale != LocaleES {
		return name
	}
	if es, ok := teamNamesES[name]; ok {
		return es
	}
	return name
}

func buildPredictionConfirmedContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	var p notification.PredictionConfirmedPayload
	if err := entry.DecodePayload(&p); err != nil {
		return userContent{}, err
	}
	home := translateTeamName(p.HomeTeam, locale)
	away := translateTeamName(p.AwayTeam, locale)
	return userContent{
		title: localeStr("Prediction confirmed", "Predicción confirmada", locale),
		body: localeStr(
			fmt.Sprintf("Your prediction for %s vs %s has been recorded.", home, away),
			fmt.Sprintf("Tu predicción para %s vs %s ha sido registrada.", home, away),
			locale,
		),
		actionURL: urlDashboard,
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
	home := translateTeamName(p.HomeTeam, locale)
	away := translateTeamName(p.AwayTeam, locale)
	return userContent{
		title: localeStr(enTitle, esTitle, locale),
		body: localeStr(
			fmt.Sprintf(enBodyFmt, home, away, p.MinutesLeft),
			fmt.Sprintf(esBodyFmt, home, away, p.MinutesLeft),
			locale,
		),
		actionURL: urlDashboard,
	}, nil
}

func buildPredictionLockedContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	var p notification.PredictionLockedPayload
	if err := entry.DecodePayload(&p); err != nil {
		return userContent{}, err
	}
	home := translateTeamName(p.HomeTeam, locale)
	away := translateTeamName(p.AwayTeam, locale)
	return userContent{
		title: localeStr("Predictions locked", "Predicciones cerradas", locale),
		body: localeStr(
			fmt.Sprintf("Predictions for %s vs %s are now locked.", home, away),
			fmt.Sprintf("Las predicciones para %s vs %s ya están cerradas.", home, away),
			locale,
		),
		actionURL: urlDashboard,
	}, nil
}

func buildPredictionScoredContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	var p notification.PredictionScoredPayload
	if err := entry.DecodePayload(&p); err != nil {
		return userContent{}, err
	}
	home := translateTeamName(p.HomeTeam, locale)
	away := translateTeamName(p.AwayTeam, locale)
	return userContent{
		title: localeStr("Match scored", "Partido puntuado", locale),
		body: localeStr(
			fmt.Sprintf("%s vs %s finished %d-%d. You earned %d points.", home, away, p.HomeScore, p.AwayScore, p.PointsEarned),
			fmt.Sprintf("%s vs %s terminó %d-%d. Ganaste %d puntos.", home, away, p.HomeScore, p.AwayScore, p.PointsEarned),
			locale,
		),
		actionURL: urlDashboard,
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
	home := translateTeamName(p.HomeTeam, locale)
	away := translateTeamName(p.AwayTeam, locale)
	return userContent{
		title: localeStr(enTitle, esTitle, locale),
		body: localeStr(
			fmt.Sprintf(enBodyFmt, home, away),
			fmt.Sprintf(esBodyFmt, home, away),
			locale,
		),
		actionURL: urlDashboard,
	}, nil
}

func buildPredictionDailyReminderContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	var p notification.PredictionDailyReminderPayload
	if err := entry.DecodePayload(&p); err != nil {
		return userContent{}, err
	}

	count := len(p.Matches)
	var matchListHTML template.HTML
	if count > 0 {
		var sb strings.Builder
		sb.WriteString(`<ul style="padding-left:20px;color:#444;line-height:2.2;margin-top:12px">`)
		for _, m := range p.Matches {
			home := html.EscapeString(translateTeamName(m.HomeTeam, locale))
			away := html.EscapeString(translateTeamName(m.AwayTeam, locale))
			kickoff := m.KickoffAt.UTC().Format("15:04")
			fmt.Fprintf(&sb, "<li><strong>%s vs %s</strong> &mdash; %s UTC</li>", home, away, kickoff)
		}
		sb.WriteString("</ul>")
		matchListHTML = template.HTML(sb.String()) //nolint:gosec // G203: all dynamic values are sanitised with html.EscapeString before insertion
	}

	return userContent{
		title: localeStr(
			"Daily prediction reminder",
			"Recordatorio de predicciones del día",
			locale,
		),
		body: localeStr(
			fmt.Sprintf("You have %d match(es) without a prediction today. Submit before kick-off:", count),
			fmt.Sprintf("Tienes %d partido(s) sin predecir hoy. Envía tu predicción antes del inicio:", count),
			locale,
		),
		actionURL:     urlDashboard,
		matchListHTML: matchListHTML,
	}, nil
}
