package domain

// Audit action strings written to the audit_log table. Using constants rather
// than inline strings prevents typos from creating silent mismatches between
// the writer and any downstream query that filters by action.
const (
	AuditActionMatchCreated         = "match.created"
	AuditActionMatchStarted         = "match.started"
	AuditActionMatchResultSet       = "match.result_set"
	AuditActionMatchResultCorrected = "match.result_corrected"
	AuditActionMatchCancelled       = "match.cancelled"
	AuditActionTiebreakerQuestion   = "tiebreaker.question_set"
	AuditActionTiebreakerResult     = "tiebreaker.result_confirmed"
	AuditActionSlotConfirmed        = "tournament.slot_confirmed"
	AuditActionGroupDeleted         = "admin_group.deleted"
	AuditActionMemberRemoved        = "admin_group.member_removed"
	AuditActionGroupSettingsUpdated = "admin_group.settings_updated"
	AuditActionOwnershipTransferred = "admin_group.ownership_transferred"
	AuditActionUserBanned           = "admin_user.banned"
	AuditActionUserUnbanned         = "admin_user.unbanned"
	AuditActionAdminSetRole         = "admin_user.role_changed"
	AuditActionPaymentCreated       = "payment.created"
	AuditActionPaymentValidated     = "payment.validated"
	AuditActionPaymentRejected      = "payment.rejected"
	AuditActionJoinApproved         = "group.join_approved"
	AuditActionGroupRenamed         = "group.renamed"
	AuditActionParamUpdated         = "param.updated"
	AuditActionConflictAcknowledged = "conflict.acknowledged"
	AuditActionConflictAutoResolved = "conflict.auto_resolved"
	AuditActionMemberBulkRemoved    = "admin_group.member_bulk_removed"
	AuditActionGroupBulkDeleted     = "admin_group.bulk_deleted"
	AuditActionLeaderboardRefreshed = "admin_group.leaderboard_refreshed"
	AuditActionPrizesDistributed    = "admin_group.prizes_distributed"
	AuditActionScoringRuleUpdated   = "scoring_rule.updated"
	AuditActionExtraRuleUpdated     = "extra_rule.updated"

	// Balance and payment actions.
	AuditActionBankTransferUploaded = "bank_transfer.uploaded"
	AuditActionBankTransferApproved = "bank_transfer.approved"
	AuditActionBankTransferRejected = "bank_transfer.rejected"
	AuditActionBalanceCredited      = "balance.credited"
	AuditActionBalanceDebited       = "balance.debited"
	AuditActionWithdrawalRequested  = "withdrawal.requested"
	AuditActionWithdrawalApproved   = "withdrawal.approved"
	AuditActionWithdrawalRejected   = "withdrawal.rejected"
	AuditActionWithdrawalProcessed  = "withdrawal.processed"
	AuditActionWebhookPaymentCredit = "webhook.payment_credited"
	// AuditActionWebhookAmountMismatch is written when a payment webhook's
	// declared amount/user/currency differs from the server-authoritative
	// payment_intents row. The credit itself always uses the intent's own
	// values (never the webhook's), so this is not a financial-integrity
	// event — it is a signal worth an operator's attention, since it can
	// indicate a forged or replayed webhook payload.
	AuditActionWebhookAmountMismatch = "webhook.amount_mismatch"

	// Exchange rate actions.
	AuditActionFXRateOverride = "fx_rate.override"
)
