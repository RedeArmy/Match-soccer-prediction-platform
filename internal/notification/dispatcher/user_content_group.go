package dispatcher

import (
	"fmt"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

func buildGroupJoinRequestedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.GroupJoinPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("New join request", "Nueva solicitud de unión", locale),
		body: localeStr(
			fmt.Sprintf("Someone has requested to join %s. Review and approve or reject the request.", p.QuinielaName),
			fmt.Sprintf("Alguien ha solicitado unirse a %s. Revisa y aprueba o rechaza la solicitud.", p.QuinielaName),
			locale,
		),
		actionURL: fmt.Sprintf(urlGroupMembers, p.QuinielaID),
	}
}

func buildGroupJoinApprovedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.GroupJoinPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Group join approved", "Solicitud de grupo aprobada", locale),
		body: localeStr(
			fmt.Sprintf("You have been approved to join %s.", p.QuinielaName),
			fmt.Sprintf("Has sido aprobado para unirte a %s.", p.QuinielaName),
			locale,
		),
		actionURL: fmt.Sprintf("/api/v1/groups/%d", p.QuinielaID),
	}
}

func buildGroupJoinRejectedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.GroupJoinPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Group join request rejected", "Solicitud de grupo rechazada", locale),
		body: localeStr(
			fmt.Sprintf("Your request to join %s was not approved.", p.QuinielaName),
			fmt.Sprintf("Tu solicitud para unirte a %s no fue aprobada.", p.QuinielaName),
			locale,
		),
		actionURL: urlGroupsMe,
	}
}

func buildGroupDisbandedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.GroupDisbandedPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Group disbanded", "Grupo disuelto", locale),
		body: localeStr(
			fmt.Sprintf("The group %s has been disbanded.", p.QuinielaName),
			fmt.Sprintf("El grupo %s ha sido disuelto.", p.QuinielaName),
			locale,
		),
		actionURL: urlGroupsMe,
	}
}

func buildGroupDeadline24hContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.GroupDeadlinePayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Group deadline in 24 hours", "Límite de grupo en 24 horas", locale),
		body: localeStr(
			fmt.Sprintf("The prediction window for %s closes in 24 hours.", p.QuinielaName),
			fmt.Sprintf("La ventana de predicciones para %s cierra en 24 horas.", p.QuinielaName),
			locale,
		),
		actionURL: fmt.Sprintf("/api/v1/groups/%d", p.QuinielaID),
	}
}

func buildGroupLeaderboardMilestoneContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.GroupLeaderboardMilestonePayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Leaderboard milestone", "Hito en el marcador", locale),
		body: localeStr(
			fmt.Sprintf("You are now ranked #%d in %s with %d points.", p.NewRank, p.QuinielaName, p.TotalPoints),
			fmt.Sprintf("Ahora estás en el puesto #%d en %s con %d puntos.", p.NewRank, p.QuinielaName, p.TotalPoints),
			locale,
		),
		actionURL: fmt.Sprintf("/api/v1/groups/%d/leaderboard", p.QuinielaID),
	}
}

func buildGroupMemberJoinedContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.GroupJoinPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("New member joined your group", "Nuevo miembro en tu grupo", locale),
		body: localeStr(
			fmt.Sprintf("A new member has joined %s.", p.QuinielaName),
			fmt.Sprintf("Un nuevo miembro se ha unido a %s.", p.QuinielaName),
			locale,
		),
		actionURL: fmt.Sprintf(urlGroupMembers, p.QuinielaID),
	}
}

func buildGroupMemberLeftContent(entry *notification.OutboxEntry, locale Locale) userContent {
	var p notification.GroupJoinPayload
	_ = entry.DecodePayload(&p)
	return userContent{
		title: localeStr("Member left the group", "Miembro abandonó el grupo", locale),
		body: localeStr(
			fmt.Sprintf("A member has left %s.", p.QuinielaName),
			fmt.Sprintf("Un miembro ha abandonado %s.", p.QuinielaName),
			locale,
		),
		actionURL: fmt.Sprintf(urlGroupMembers, p.QuinielaID),
	}
}
