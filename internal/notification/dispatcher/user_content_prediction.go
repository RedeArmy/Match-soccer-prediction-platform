package dispatcher

import (
	"fmt"
	"html"
	"html/template"
	"strings"
	"time"

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

func buildDailySummaryContent(entry *notification.OutboxEntry, locale Locale) (userContent, error) {
	var p notification.DailySummaryPayload
	if err := entry.DecodePayload(&p); err != nil {
		return userContent{}, err
	}

	home := translateTeamName
	away := translateTeamName

	// Build the match-results table with inline styles (email-client safe).
	var sb strings.Builder

	// Introductory sentence.
	if locale == LocaleES {
		fmt.Fprintf(&sb, `<p style="color:#444;margin:0 0 16px">Resultados de la jornada del <strong>%s</strong>:</p>`, p.MatchDate)
	} else {
		fmt.Fprintf(&sb, `<p style="color:#444;margin:0 0 16px">Here are your results for <strong>%s</strong>:</p>`, p.MatchDate)
	}

	// Table header
	if locale == LocaleES {
		sb.WriteString(`<table style="width:100%;border-collapse:collapse;font-size:14px;margin-bottom:24px">`)
		sb.WriteString(`<thead><tr style="background:#1a1a2e;color:#fff">`)
		sb.WriteString(`<th style="padding:10px 12px;text-align:left;font-weight:600">Partido</th>`)
		sb.WriteString(`<th style="padding:10px 12px;text-align:center;font-weight:600">Resultado</th>`)
		sb.WriteString(`<th style="padding:10px 12px;text-align:center;font-weight:600">Tu predicción</th>`)
		sb.WriteString(`<th style="padding:10px 12px;text-align:center;font-weight:600">Puntos</th>`)
		sb.WriteString(`</tr></thead><tbody>`)
	} else {
		sb.WriteString(`<table style="width:100%;border-collapse:collapse;font-size:14px;margin-bottom:24px">`)
		sb.WriteString(`<thead><tr style="background:#1a1a2e;color:#fff">`)
		sb.WriteString(`<th style="padding:10px 12px;text-align:left;font-weight:600">Match</th>`)
		sb.WriteString(`<th style="padding:10px 12px;text-align:center;font-weight:600">Result</th>`)
		sb.WriteString(`<th style="padding:10px 12px;text-align:center;font-weight:600">Your prediction</th>`)
		sb.WriteString(`<th style="padding:10px 12px;text-align:center;font-weight:600">Points</th>`)
		sb.WriteString(`</tr></thead><tbody>`)
	}

	for i, m := range p.Matches {
		rowBg := "#fff"
		if i%2 == 1 {
			rowBg = "#f8f9fa"
		}
		h := html.EscapeString(home(m.HomeTeam, locale))
		aw := html.EscapeString(away(m.AwayTeam, locale))
		kickoff := m.KickoffAt.UTC().Format("15:04")
		match := fmt.Sprintf(`%s vs %s <span style="color:#888;font-size:12px">(%s UTC)</span>`, h, aw, kickoff)
		result := fmt.Sprintf(`<strong>%d – %d</strong>`, m.HomeScore, m.AwayScore)

		var predCell string
		var pointsBadge string
		if m.PredHome != nil && m.PredAway != nil {
			predCell = fmt.Sprintf(`%d – %d`, *m.PredHome, *m.PredAway)
			badgeColor := "#e74c3c"
			if m.PointsEarned >= 5 {
				badgeColor = "#27ae60" // exact score
			} else if m.PointsEarned >= 3 {
				badgeColor = "#f39c12" // outcome/diff
			} else if m.PointsEarned > 0 {
				badgeColor = "#e67e22"
			}
			pointsBadge = fmt.Sprintf(`<span style="display:inline-block;background:%s;color:#fff;border-radius:12px;padding:2px 10px;font-weight:700;font-size:13px">+%d</span>`, badgeColor, m.PointsEarned)
		} else {
			if locale == LocaleES {
				predCell = `<span style="color:#bbb">Sin predicción</span>`
			} else {
				predCell = `<span style="color:#bbb">No prediction</span>`
			}
			pointsBadge = `<span style="color:#bbb">—</span>`
		}

		fmt.Fprintf(&sb,
			`<tr style="background:%s;border-bottom:1px solid #eee">
  <td style="padding:10px 12px;color:#333">%s</td>
  <td style="padding:10px 12px;text-align:center;color:#333">%s</td>
  <td style="padding:10px 12px;text-align:center;color:#333">%s</td>
  <td style="padding:10px 12px;text-align:center">%s</td>
</tr>`,
			rowBg, match, result, predCell, pointsBadge,
		)
	}

	// Totals row
	sb.WriteString(`<tr style="background:#1a1a2e;color:#fff;font-weight:700">`)
	if locale == LocaleES {
		fmt.Fprintf(&sb, `<td colspan="3" style="padding:10px 12px">Puntos de hoy</td>`)
		fmt.Fprintf(&sb, `<td style="padding:10px 12px;text-align:center;font-size:16px">+%d</td>`, p.PointsToday)
	} else {
		fmt.Fprintf(&sb, `<td colspan="3" style="padding:10px 12px">Today's points</td>`)
		fmt.Fprintf(&sb, `<td style="padding:10px 12px;text-align:center;font-size:16px">+%d</td>`, p.PointsToday)
	}
	sb.WriteString(`</tr>`)
	sb.WriteString(`</tbody></table>`)

	// Encouraging message (rotated by day-of-week for variety)
	encouragingES := []string{
		"¡Sigue así! Cada partido es una nueva oportunidad de subir en el marcador.",
		"¡Buen trabajo hoy! Mañana hay más partidos — no olvides tus predicciones.",
		"¡El torneo apenas empieza! Sigue prediciendo para escalar posiciones.",
		"¡Cada punto cuenta! Mantén el ritmo y sigue compitiendo fuerte.",
		"¡Ánimo! Los mejores predictores no se rinden — prepárate para la siguiente jornada.",
		"¡Genial! Revisa el marcador y planea tu estrategia para los próximos partidos.",
		"¡Vas por buen camino! Sigue prediciendo y demuestra tu conocimiento futbolero.",
	}
	encouragingEN := []string{
		"Keep it up! Every match is a new chance to climb the leaderboard.",
		"Great day! More matches tomorrow — don't forget your predictions.",
		"The tournament is just getting started! Keep predicting to move up the ranks.",
		"Every point counts! Stay consistent and keep competing.",
		"Don't give up! The best predictors never quit — get ready for the next round.",
		"Nice work! Check the leaderboard and plan your strategy for upcoming matches.",
		"You're on the right track! Keep predicting and show off your football knowledge.",
	}
	dayIdx := int(time.Now().Weekday())
	var encourageMsg string
	if locale == LocaleES {
		encourageMsg = encouragingES[dayIdx%len(encouragingES)]
	} else {
		encourageMsg = encouragingEN[dayIdx%len(encouragingEN)]
	}
	fmt.Fprintf(&sb, `<p style="color:#555;font-style:italic;margin:0">%s</p>`, html.EscapeString(encourageMsg))

	subject := localeStr(
		fmt.Sprintf("Your daily summary — %s | +%d pts today", p.MatchDate, p.PointsToday),
		fmt.Sprintf("Resumen del día — %s | +%d pts hoy", p.MatchDate, p.PointsToday),
		locale,
	)
	headline := localeStr(
		fmt.Sprintf("Match day summary — %s", p.MatchDate),
		fmt.Sprintf("Resumen de la jornada — %s", p.MatchDate),
		locale,
	)

	pushBody := localeStr(
		fmt.Sprintf("+%d pts today", p.PointsToday),
		fmt.Sprintf("+%d pts hoy", p.PointsToday),
		locale,
	)
	return userContent{
		title:         headline,
		emailSubject:  subject,
		body:          pushBody,
		matchListHTML: template.HTML(sb.String()), //nolint:gosec // G203: all dynamic values escaped via html.EscapeString
		actionURL:     urlDashboard,
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
