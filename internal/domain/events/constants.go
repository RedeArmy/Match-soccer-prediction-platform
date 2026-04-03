package events

// TODO: define event type constants used to identify event payloads on the bus.
// Using typed constants rather than raw strings prevents typos from causing
// silent routing failures where a publisher emits "MatchFinshed" and the
// subscriber listens for "MatchFinished", with no compile-time or runtime error.
