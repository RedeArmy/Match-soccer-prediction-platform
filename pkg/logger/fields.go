package logger

// TODO: define reusable zap.Field constructors for fields logged across
// multiple packages (e.g. UserID, MatchID, RequestID, Latency).
//
// Centralising field definitions prevents teams from using inconsistent
// key names ("user_id" vs "userId" vs "uid") across log lines, which breaks
// log-aggregation queries and dashboards that filter on specific field names.
