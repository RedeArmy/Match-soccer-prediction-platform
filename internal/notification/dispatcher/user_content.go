package dispatcher

import "github.com/rede/world-cup-quiniela/internal/notification"

// Action URL path constants used by content builder functions.
// Defined once here to prevent silent drift when the API route changes.
const (
	urlMatchDetail  = "/api/v1/matches/%d"
	urlGroupsMe     = "/api/v1/groups/me"
	urlBalance      = "/api/v1/users/me/balance"
	urlWithdrawals  = "/api/v1/withdrawals"
	urlGroupMembers = "/api/v1/groups/%d/members"
)

// contentBuilderFunc renders the notification content for a specific event type.
type contentBuilderFunc func(entry *notification.OutboxEntry, locale Locale) userContent

// contentRegistry maps each event type to its content builder function.
// Lookups are O(1); adding a new event type requires one registry entry and
// one builder function — no changes to resolveUserContent are needed.
var contentRegistry map[notification.EventType]contentBuilderFunc

func init() {
	contentRegistry = map[notification.EventType]contentBuilderFunc{
		// Predictions
		notification.EventPredictionConfirmed:        buildPredictionConfirmedContent,
		notification.EventPredictionDeadlineApproach: buildPredictionDeadlineApproachContent,
		notification.EventPredictionMissingReminder:  buildPredictionMissingReminderContent,
		notification.EventPredictionLocked:           buildPredictionLockedContent,
		notification.EventPredictionScored:           buildPredictionScoredContent,
		// Matches
		notification.EventMatchResultEntered: buildMatchResultEnteredContent,
		notification.EventMatchPostponed:     buildMatchPostponedContent,
		notification.EventMatchCancelled:     buildMatchCancelledContent,
		// Groups
		notification.EventGroupJoinRequested:        buildGroupJoinRequestedContent,
		notification.EventGroupJoinApproved:         buildGroupJoinApprovedContent,
		notification.EventGroupJoinRejected:         buildGroupJoinRejectedContent,
		notification.EventGroupDisbanded:            buildGroupDisbandedContent,
		notification.EventGroupDeadline24h:          buildGroupDeadline24hContent,
		notification.EventGroupLeaderboardMilestone: buildGroupLeaderboardMilestoneContent,
		notification.EventGroupMemberJoined:         buildGroupMemberJoinedContent,
		notification.EventGroupMemberLeft:           buildGroupMemberLeftContent,
		// Payments
		notification.EventPaymentConfirmed:             buildPaymentConfirmedContent,
		notification.EventPaymentFailed:                buildPaymentFailedContent,
		notification.EventPaymentBankTransferSubmitted: buildPaymentBankTransferSubmittedContent,
		notification.EventPaymentBankTransferApproved:  buildPaymentBankTransferApprovedContent,
		notification.EventPaymentBankTransferRejected:  buildPaymentBankTransferRejectedContent,
		notification.EventPaymentPendingTimeout:        buildPaymentPendingTimeoutContent,
		// Withdrawals
		notification.EventWithdrawalRequested:     buildWithdrawalRequestedContent,
		notification.EventWithdrawalApproved:      buildWithdrawalApprovedContent,
		notification.EventWithdrawalRejected:      buildWithdrawalRejectedContent,
		notification.EventWithdrawalCompleted:     buildWithdrawalCompletedContent,
		notification.EventWithdrawalFailed:        buildWithdrawalFailedContent,
		notification.EventWithdrawalProcessing:    buildWithdrawalProcessingContent,
		notification.EventWithdrawalPendingTimeout: buildWithdrawalPendingTimeoutContent,
		// Account
		notification.EventAccountWelcome:        buildAccountWelcomeContent,
		notification.EventAccountBalanceCredited: buildAccountBalanceCreditedContent,
		notification.EventAccountBalanceDebited:  buildAccountBalanceDebitedContent,
		notification.EventAccountLowBalance:      buildAccountLowBalanceContent,
	}
}

// resolveUserContent maps an outbox entry to its title/body/actionURL for the
// given locale via a registry lookup. CC=2: one map lookup, no switch.
func resolveUserContent(entry *notification.OutboxEntry, locale Locale) userContent {
	if build, ok := contentRegistry[entry.EventType]; ok {
		return build(entry, locale)
	}
	return defaultUserContent(locale)
}

// buildUserContent returns the rendered notification content for the given
// event. The locale field is attached here so builder functions do not need to
// set it individually.
func buildUserContent(entry *notification.OutboxEntry, locale Locale) userContent {
	c := resolveUserContent(entry, locale)
	c.locale = locale
	return c
}

// defaultUserContent is the fallback for event types not registered in contentRegistry.
func defaultUserContent(locale Locale) userContent {
	return userContent{
		title: localeStr("New notification", "Nueva notificación", locale),
		body:  localeStr("You have a new notification. Open the app for details.", "Tienes una nueva notificación. Abre la aplicación para más detalles.", locale),
	}
}
