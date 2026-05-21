-- Seed initial notification template content from the compiled Go defaults.
--
-- Operators may edit any row via the admin API:
--   PUT /api/v1/admin/notification-templates/{event_type}/{locale}
--   POST /api/v1/admin/notification-templates/{event_type}/{locale}/preview
--
-- Delete a row to revert that event/locale back to the compiled Go fallback
-- without requiring a redeployment.
--
-- Template syntax: Go text/template.  Available functions:
--   formatCents .amount_cents .currency  →  "50.00 GTQ"
--   int .quiniela_id                     →  int64 safe for URL path segments
--
-- Idempotent: ON CONFLICT DO NOTHING is safe on re-run.
INSERT INTO notification_templates (event_type, locale, title_tmpl, body_tmpl, action_url_tmpl) VALUES

-- ── Predictions ───────────────────────────────────────────────────────────────

('prediction.confirmed', 'en',
 'Prediction confirmed',
 $$Your prediction for {{.home_team}} vs {{.away_team}} has been recorded.$$,
 '/api/v1/predictions/me'),

('prediction.confirmed', 'es',
 'Predicción confirmada',
 $$Tu predicción para {{.home_team}} vs {{.away_team}} ha sido registrada.$$,
 '/api/v1/predictions/me'),

('prediction.deadline_approaching', 'en',
 'Prediction deadline approaching',
 $${{.home_team}} vs {{.away_team}} kicks off in {{int .minutes_left}} minutes — submit your prediction now.$$,
 $$/api/v1/matches/{{int .match_id}}$$),

('prediction.deadline_approaching', 'es',
 'Límite de predicción se acerca',
 $${{.home_team}} vs {{.away_team}} empieza en {{int .minutes_left}} minutos — envía tu predicción ahora.$$,
 $$/api/v1/matches/{{int .match_id}}$$),

('prediction.missing_reminder', 'en',
 'Missing prediction reminder',
 $$You haven''t predicted {{.home_team}} vs {{.away_team}} yet. Deadline is in {{int .minutes_left}} minutes.$$,
 $$/api/v1/matches/{{int .match_id}}$$),

('prediction.missing_reminder', 'es',
 'Recordatorio de predicción pendiente',
 $$Aún no has predicho {{.home_team}} vs {{.away_team}}. El límite cierra en {{int .minutes_left}} minutos.$$,
 $$/api/v1/matches/{{int .match_id}}$$),

('prediction.locked', 'en',
 'Predictions locked',
 $$Predictions for {{.home_team}} vs {{.away_team}} are now locked.$$,
 $$/api/v1/matches/{{int .match_id}}$$),

('prediction.locked', 'es',
 'Predicciones cerradas',
 $$Las predicciones para {{.home_team}} vs {{.away_team}} ya están cerradas.$$,
 $$/api/v1/matches/{{int .match_id}}$$),

('prediction.scored', 'en',
 'Match scored',
 $${{.home_team}} vs {{.away_team}} finished {{int .home_score}}-{{int .away_score}}. You earned {{int .points_earned}} points.$$,
 '/api/v1/predictions/me'),

('prediction.scored', 'es',
 'Partido puntuado',
 $${{.home_team}} vs {{.away_team}} terminó {{int .home_score}}-{{int .away_score}}. Ganaste {{int .points_earned}} puntos.$$,
 '/api/v1/predictions/me'),

-- ── Match events ──────────────────────────────────────────────────────────────

('match.result_entered', 'en',
 'Match result entered',
 $$The result for {{.home_team}} vs {{.away_team}} has been recorded.$$,
 $$/api/v1/matches/{{int .match_id}}$$),

('match.result_entered', 'es',
 'Resultado registrado',
 $$El resultado de {{.home_team}} vs {{.away_team}} ha sido registrado.$$,
 $$/api/v1/matches/{{int .match_id}}$$),

('match.postponed', 'en',
 'Match postponed',
 $${{.home_team}} vs {{.away_team}} has been postponed.$$,
 $$/api/v1/matches/{{int .match_id}}$$),

('match.postponed', 'es',
 'Partido aplazado',
 $${{.home_team}} vs {{.away_team}} ha sido aplazado.$$,
 $$/api/v1/matches/{{int .match_id}}$$),

('match.cancelled', 'en',
 'Match cancelled',
 $${{.home_team}} vs {{.away_team}} has been cancelled.$$,
 $$/api/v1/matches/{{int .match_id}}$$),

('match.cancelled', 'es',
 'Partido cancelado',
 $${{.home_team}} vs {{.away_team}} ha sido cancelado.$$,
 $$/api/v1/matches/{{int .match_id}}$$),

-- ── Group events ──────────────────────────────────────────────────────────────

('group.join_requested', 'en',
 'New join request',
 $$Someone has requested to join {{.quiniela_name}}. Review and approve or reject the request.$$,
 $$/api/v1/groups/{{int .quiniela_id}}/members$$),

('group.join_requested', 'es',
 'Nueva solicitud de unión',
 $$Alguien ha solicitado unirse a {{.quiniela_name}}. Revisa y aprueba o rechaza la solicitud.$$,
 $$/api/v1/groups/{{int .quiniela_id}}/members$$),

('group.join_approved', 'en',
 'Group join approved',
 $$You have been approved to join {{.quiniela_name}}.$$,
 $$/api/v1/groups/{{int .quiniela_id}}$$),

('group.join_approved', 'es',
 'Solicitud de grupo aprobada',
 $$Has sido aprobado para unirte a {{.quiniela_name}}.$$,
 $$/api/v1/groups/{{int .quiniela_id}}$$),

('group.join_rejected', 'en',
 'Group join request rejected',
 $$Your request to join {{.quiniela_name}} was not approved.$$,
 '/api/v1/groups/me'),

('group.join_rejected', 'es',
 'Solicitud de grupo rechazada',
 $$Tu solicitud para unirte a {{.quiniela_name}} no fue aprobada.$$,
 '/api/v1/groups/me'),

('group.disbanded', 'en',
 'Group disbanded',
 $$The group {{.quiniela_name}} has been disbanded.$$,
 '/api/v1/groups/me'),

('group.disbanded', 'es',
 'Grupo disuelto',
 $$El grupo {{.quiniela_name}} ha sido disuelto.$$,
 '/api/v1/groups/me'),

('group.deadline_24h', 'en',
 'Group deadline in 24 hours',
 $$The prediction window for {{.quiniela_name}} closes in 24 hours.$$,
 $$/api/v1/groups/{{int .quiniela_id}}$$),

('group.deadline_24h', 'es',
 'Límite de grupo en 24 horas',
 $$La ventana de predicciones para {{.quiniela_name}} cierra en 24 horas.$$,
 $$/api/v1/groups/{{int .quiniela_id}}$$),

('group.leaderboard_milestone', 'en',
 'Leaderboard milestone',
 $$You are now ranked #{{int .new_rank}} in {{.quiniela_name}} with {{int .total_points}} points.$$,
 $$/api/v1/groups/{{int .quiniela_id}}/leaderboard$$),

('group.leaderboard_milestone', 'es',
 'Hito en el marcador',
 $$Ahora estás en el puesto #{{int .new_rank}} en {{.quiniela_name}} con {{int .total_points}} puntos.$$,
 $$/api/v1/groups/{{int .quiniela_id}}/leaderboard$$),

('group.member_joined', 'en',
 'New member joined your group',
 $$A new member has joined {{.quiniela_name}}.$$,
 $$/api/v1/groups/{{int .quiniela_id}}/members$$),

('group.member_joined', 'es',
 'Nuevo miembro en tu grupo',
 $$Un nuevo miembro se ha unido a {{.quiniela_name}}.$$,
 $$/api/v1/groups/{{int .quiniela_id}}/members$$),

('group.member_left', 'en',
 'Member left the group',
 $$A member has left {{.quiniela_name}}.$$,
 $$/api/v1/groups/{{int .quiniela_id}}/members$$),

('group.member_left', 'es',
 'Miembro abandonó el grupo',
 $$Un miembro ha abandonado {{.quiniela_name}}.$$,
 $$/api/v1/groups/{{int .quiniela_id}}/members$$),

-- ── Payment events ────────────────────────────────────────────────────────────

('payment.confirmed', 'en',
 'Payment confirmed',
 $$Your payment of {{formatCents .amount_cents .currency}} has been confirmed.$$,
 '/api/v1/users/me/balance'),

('payment.confirmed', 'es',
 'Pago confirmado',
 $$Tu pago de {{formatCents .amount_cents .currency}} ha sido confirmado.$$,
 '/api/v1/users/me/balance'),

('payment.failed', 'en',
 'Payment failed',
 $$Your payment of {{formatCents .amount_cents .currency}} could not be processed. {{.reason}}$$,
 '/api/v1/payment-intents'),

('payment.failed', 'es',
 'Pago fallido',
 $$Tu pago de {{formatCents .amount_cents .currency}} no pudo procesarse. {{.reason}}$$,
 '/api/v1/payment-intents'),

('payment.bank_transfer_submitted', 'en',
 'Bank transfer proof submitted',
 $$Your transfer proof for {{formatCents .amount_cents .currency}} has been submitted and is awaiting review.$$,
 '/api/v1/bank-transfers'),

('payment.bank_transfer_submitted', 'es',
 'Comprobante de transferencia enviado',
 $$Tu comprobante de transferencia por {{formatCents .amount_cents .currency}} ha sido enviado y está pendiente de revisión.$$,
 '/api/v1/bank-transfers'),

('payment.bank_transfer_approved', 'en',
 'Bank transfer approved',
 $${{formatCents .amount_cents .currency}} has been credited to your account.$$,
 '/api/v1/users/me/balance'),

('payment.bank_transfer_approved', 'es',
 'Transferencia bancaria aprobada',
 $${{formatCents .amount_cents .currency}} ha sido acreditado a tu cuenta.$$,
 '/api/v1/users/me/balance'),

('payment.bank_transfer_rejected', 'en',
 'Bank transfer rejected',
 $$Your transfer proof for {{formatCents .amount_cents .currency}} was rejected. {{.notes}}$$,
 '/api/v1/bank-transfers'),

('payment.bank_transfer_rejected', 'es',
 'Transferencia bancaria rechazada',
 $$Tu comprobante de transferencia por {{formatCents .amount_cents .currency}} fue rechazado. {{.notes}}$$,
 '/api/v1/bank-transfers'),

('payment.pending_timeout', 'en',
 'Payment expired',
 $$Your payment of {{formatCents .amount_cents .currency}} has expired without confirmation. Please try again.$$,
 '/api/v1/payment-intents'),

('payment.pending_timeout', 'es',
 'Pago expirado',
 $$Tu pago de {{formatCents .amount_cents .currency}} ha expirado sin confirmación. Por favor, inténtalo de nuevo.$$,
 '/api/v1/payment-intents'),

-- ── Withdrawal events ─────────────────────────────────────────────────────────

('withdrawal.requested', 'en',
 'Withdrawal requested',
 $$Your withdrawal of {{formatCents .amount_cents .currency}} is pending admin approval.$$,
 '/api/v1/withdrawals'),

('withdrawal.requested', 'es',
 'Retiro solicitado',
 $$Tu retiro de {{formatCents .amount_cents .currency}} está pendiente de aprobación del administrador.$$,
 '/api/v1/withdrawals'),

('withdrawal.approved', 'en',
 'Withdrawal approved',
 $$Your withdrawal of {{formatCents .amount_cents .currency}} has been approved.$$,
 '/api/v1/withdrawals'),

('withdrawal.approved', 'es',
 'Retiro aprobado',
 $$Tu retiro de {{formatCents .amount_cents .currency}} ha sido aprobado.$$,
 '/api/v1/withdrawals'),

('withdrawal.rejected', 'en',
 'Withdrawal rejected',
 $$Your withdrawal of {{formatCents .amount_cents .currency}} was rejected. {{.notes}}$$,
 '/api/v1/withdrawals'),

('withdrawal.rejected', 'es',
 'Retiro rechazado',
 $$Tu retiro de {{formatCents .amount_cents .currency}} fue rechazado. {{.notes}}$$,
 '/api/v1/withdrawals'),

('withdrawal.processing', 'en',
 'Withdrawal being processed',
 $$Your withdrawal of {{formatCents .amount_cents .currency}} is now being processed. Funds will be transferred shortly.$$,
 '/api/v1/withdrawals'),

('withdrawal.processing', 'es',
 'Retiro en proceso',
 $$Tu retiro de {{formatCents .amount_cents .currency}} está siendo procesado. Los fondos serán transferidos pronto.$$,
 '/api/v1/withdrawals'),

('withdrawal.completed', 'en',
 'Withdrawal completed',
 $$Your withdrawal of {{formatCents .amount_cents .currency}} has been processed successfully.$$,
 '/api/v1/users/me/balance'),

('withdrawal.completed', 'es',
 'Retiro completado',
 $$Tu retiro de {{formatCents .amount_cents .currency}} ha sido procesado exitosamente.$$,
 '/api/v1/users/me/balance'),

('withdrawal.failed', 'en',
 'Withdrawal failed',
 $$Your withdrawal of {{formatCents .amount_cents .currency}} could not be completed. Please contact support.$$,
 '/api/v1/withdrawals'),

('withdrawal.failed', 'es',
 'Retiro fallido',
 $$Tu retiro de {{formatCents .amount_cents .currency}} no pudo completarse. Por favor, contacta al soporte.$$,
 '/api/v1/withdrawals'),

('withdrawal.pending_timeout', 'en',
 'Withdrawal request expired',
 $$Your withdrawal of {{formatCents .amount_cents .currency}} has expired without admin action. Please submit a new request or contact support.$$,
 '/api/v1/withdrawals'),

('withdrawal.pending_timeout', 'es',
 'Solicitud de retiro expirada',
 $$Tu retiro de {{formatCents .amount_cents .currency}} ha expirado sin acción del administrador. Envía una nueva solicitud o contacta al soporte.$$,
 '/api/v1/withdrawals'),

-- ── Account events ────────────────────────────────────────────────────────────

('account.welcome', 'en',
 'Welcome to World Cup Quiniela!',
 $$Hi {{.user_name}}! Your account is ready. Start predicting now.$$,
 '/api/v1/groups/me'),

('account.welcome', 'es',
 '¡Bienvenido a World Cup Quiniela!',
 $$¡Hola {{.user_name}}! Tu cuenta está lista. Empieza a predecir ahora.$$,
 '/api/v1/groups/me'),

('account.balance_credited', 'en',
 'Balance credited',
 $${{formatCents .amount_cents .currency}} has been added to your account. New balance: {{formatCents .balance_after .currency}}.$$,
 '/api/v1/users/me/balance'),

('account.balance_credited', 'es',
 'Saldo acreditado',
 $${{formatCents .amount_cents .currency}} ha sido añadido a tu cuenta. Nuevo saldo: {{formatCents .balance_after .currency}}.$$,
 '/api/v1/users/me/balance'),

('account.balance_debited', 'en',
 'Balance debited',
 $${{formatCents .amount_cents .currency}} has been deducted from your account. New balance: {{formatCents .balance_after .currency}}.$$,
 '/api/v1/users/me/balance'),

('account.balance_debited', 'es',
 'Saldo debitado',
 $${{formatCents .amount_cents .currency}} ha sido deducido de tu cuenta. Nuevo saldo: {{formatCents .balance_after .currency}}.$$,
 '/api/v1/users/me/balance'),

('account.low_balance', 'en',
 'Low balance alert',
 $$Your balance is {{formatCents .balance_after .currency}}. Top up to continue participating.$$,
 '/api/v1/users/me/balance'),

('account.low_balance', 'es',
 'Alerta de saldo bajo',
 $$Tu saldo es {{formatCents .balance_after .currency}}. Recarga para seguir participando.$$,
 '/api/v1/users/me/balance')

ON CONFLICT (event_type, locale) DO NOTHING;
