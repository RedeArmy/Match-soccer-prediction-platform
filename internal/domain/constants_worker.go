package domain

// Default values for worker and system-purge system parameters.
const (
	// Worker: leaderboard snapshot generation.
	// 4 concurrent snapshots is sized for a single shared-CPU machine with 256 MB RAM.
	// Each goroutine holds pgx row buffers while executing ranking queries; 16 goroutines
	// at peak can exhaust the heap on a memory-constrained instance. Raise via system
	// param worker.snapshot_concurrency when running on a larger instance.
	DefaultWorkerSnapshotConcurrency = 4   // worker.snapshot_concurrency
	DefaultWorkerSnapshotRetryBaseMs = 100 // worker.snapshot_retry_base_ms
	DefaultWorkerSnapshotMaxAttempts = 3   // worker.snapshot_max_attempts

	// Worker: background maintenance jobs.
	DefaultWorkerDLQMonitorIntervalSec = 300 // worker.dlq_monitor_interval_sec (5 min)
	DefaultWorkerPurgeIntervalHours    = 24  // worker.purge_interval_hours

	// Soft-delete retention.
	DefaultPurgeRetentionDays = 30 // system.purge_retention_days

	// Leaderboard snapshot retention: number of most-recent snapshots to keep per
	// quiniela. The daily purge job deletes every snapshot beyond this count,
	// bounding table growth to (active_quinielas × keep_latest_count) rows.
	// Five snapshots cover the last five match results — sufficient for trend
	// display — while staying well below the 6 400-row worst case for 64 matches
	// across 100 quinielas.
	DefaultSnapshotKeepLatestCount = 5 // snapshot.keep_latest_count
)

// Worker and system-purge system parameter keys.
const (
	// Worker params (all is_runtime=FALSE: worker restart required).
	// ParamKeyWorkerSnapshotConcurrency caps concurrent quiniela snapshots per event.
	ParamKeyWorkerSnapshotConcurrency = "worker.snapshot_concurrency"
	// ParamKeyWorkerSnapshotRetryBaseMs is the initial snapshot retry backoff in ms;
	// doubles on each subsequent attempt (exponential).
	ParamKeyWorkerSnapshotRetryBaseMs = "worker.snapshot_retry_base_ms"
	// ParamKeyWorkerSnapshotMaxAttempts is the maximum snapshot write attempts per
	// quiniela per match event.
	ParamKeyWorkerSnapshotMaxAttempts = "worker.snapshot_max_attempts"
	// ParamKeyWorkerDLQMonitorIntervalSec is the seconds between DLQ size log events.
	ParamKeyWorkerDLQMonitorIntervalSec = "worker.dlq_monitor_interval_sec"
	// ParamKeyWorkerPurgeIntervalHours is the hours between permanent purge runs.
	ParamKeyWorkerPurgeIntervalHours = "worker.purge_interval_hours"

	// ParamKeyPurgeRetentionDays is the age in days after which soft-deleted
	// users and quinielas are permanently removed by the worker purge goroutine.
	// is_runtime=FALSE: changing the value requires a worker restart.
	ParamKeyPurgeRetentionDays = "system.purge_retention_days"

	// ParamKeySnapshotKeepLatestCount is the number of most-recent leaderboard
	// snapshots to retain per quiniela. The daily purge job deletes every snapshot
	// beyond this count. is_runtime=FALSE: worker restart required.
	ParamKeySnapshotKeepLatestCount = "snapshot.keep_latest_count"
)
