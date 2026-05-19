// Package scheduler implements periodic notification jobs for the World Cup
// Quiniela platform.
//
// Jobs are registered with a schedule spec (interval, daily, or weekly) and
// executed by a single Scheduler goroutine.  An injectable Clock makes the
// scheduler fully deterministic in tests without any real-time sleeps.
//
// Every job writes one or more outbox events; the outbox worker then claims
// and dispatches them through the normal reliability path (retry, DLQ).
//
// Leader election (election.LeaderElection.TryAcquire) ensures that only one
// replica fires jobs in a multi-replica deployment.  If TryAcquire returns
// false the tick is silently skipped.
package scheduler

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// Clock is an injectable time source.  Production code uses RealClock; tests
// inject a stub to drive the scheduler forward without real-time delays.
type Clock interface {
	Now() time.Time
}

// RealClock delegates to the standard library.
type RealClock struct{}

// Now returns time.Now().
func (RealClock) Now() time.Time { return time.Now() }

// LeaderElector reports whether this replica should fire jobs right now.
// The production implementation uses a distributed lease (e.g. pg advisory
// lock or Redis NX key).  Tests supply a stub that always returns true.
type LeaderElector interface {
	TryAcquire(ctx context.Context) bool
}

// alwaysLeader is a no-op elector used when no elector is configured.
type alwaysLeader struct{}

func (alwaysLeader) TryAcquire(_ context.Context) bool { return true }

// scheduleKind classifies the firing cadence for a job.
type scheduleKind int

const (
	kindInterval scheduleKind = iota // fire every N duration
	kindDaily                        // fire once per day at a fixed time-of-day
	kindWeekly                       // fire once per week on a fixed weekday + time-of-day
)

// jobSpec describes when a job fires.
type jobSpec struct {
	kind     scheduleKind
	interval time.Duration // kindInterval
	hour     int           // kindDaily / kindWeekly: hour in [0, 23]
	minute   int           // kindDaily / kindWeekly: minute in [0, 59]
	weekday  time.Weekday  // kindWeekly
}

// job pairs a schedule specification with the function to run.
type job struct {
	name    string
	spec    jobSpec
	lastRun time.Time
	fn      func(ctx context.Context) error
}

// Scheduler runs periodic notification jobs.  Construct with New and start
// with Run.
type Scheduler struct {
	jobs    []*job
	clock   Clock
	elector LeaderElector
	tick    time.Duration // resolution of the internal ticker
	loc     *time.Location
	log     *zap.Logger
}

// Config bundles constructor arguments for Scheduler.
type Config struct {
	Clock    Clock          // nil → RealClock
	Elector  LeaderElector  // nil → always leader
	Tick     time.Duration  // nil → 30s
	Location *time.Location // nil → UTC
	Log      *zap.Logger
}

// New constructs a Scheduler.
func New(cfg Config) *Scheduler {
	if cfg.Clock == nil {
		cfg.Clock = RealClock{}
	}
	if cfg.Elector == nil {
		cfg.Elector = alwaysLeader{}
	}
	if cfg.Tick <= 0 {
		cfg.Tick = 30 * time.Second
	}
	if cfg.Location == nil {
		cfg.Location = time.UTC
	}
	return &Scheduler{
		clock:   cfg.Clock,
		elector: cfg.Elector,
		tick:    cfg.Tick,
		loc:     cfg.Location,
		log:     cfg.Log,
	}
}

// RegisterInterval adds a job that fires every interval duration.
func (s *Scheduler) RegisterInterval(name string, interval time.Duration, fn func(context.Context) error) {
	s.jobs = append(s.jobs, &job{
		name: name,
		spec: jobSpec{kind: kindInterval, interval: interval},
		fn:   fn,
	})
}

// RegisterDaily adds a job that fires once per day at hour:minute in the
// configured location.
func (s *Scheduler) RegisterDaily(name string, hour, minute int, fn func(context.Context) error) {
	s.jobs = append(s.jobs, &job{
		name: name,
		spec: jobSpec{kind: kindDaily, hour: hour, minute: minute},
		fn:   fn,
	})
}

// RegisterWeekly adds a job that fires once per week on the given weekday
// at hour:minute in the configured location.
func (s *Scheduler) RegisterWeekly(name string, weekday time.Weekday, hour, minute int, fn func(context.Context) error) {
	s.jobs = append(s.jobs, &job{
		name: name,
		spec: jobSpec{kind: kindWeekly, weekday: weekday, hour: hour, minute: minute},
		fn:   fn,
	})
}

// Run blocks until ctx is cancelled, evaluating registered jobs every s.tick.
// It logs but swallows per-job errors so a single failure does not stop the
// scheduler.
func (s *Scheduler) Run(ctx context.Context) {
	s.log.Info("notification scheduler started",
		zap.Duration("tick", s.tick),
		zap.Int("jobs", len(s.jobs)),
	)
	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.log.Info("notification scheduler stopped")
			return
		case t := <-ticker.C:
			s.runDue(ctx, t)
		}
	}
}

// RunWithTick is the test hook: it evaluates all due jobs for the given
// synthetic now value without any real timers.
func (s *Scheduler) RunWithTick(ctx context.Context, now time.Time) {
	s.runDue(ctx, now)
}

func (s *Scheduler) runDue(ctx context.Context, now time.Time) {
	if !s.elector.TryAcquire(ctx) {
		return
	}
	for _, j := range s.jobs {
		if s.shouldRun(j, now) {
			j.lastRun = now
			if err := j.fn(ctx); err != nil {
				s.log.Warn("scheduler: job failed",
					zap.String("job", j.name),
					zap.Error(err),
				)
			}
		}
	}
}

// shouldRun returns true when job j is due at time now.
func (s *Scheduler) shouldRun(j *job, now time.Time) bool {
	switch j.spec.kind {
	case kindInterval:
		return j.lastRun.IsZero() || now.Sub(j.lastRun) >= j.spec.interval

	case kindDaily:
		local := now.In(s.loc)
		if j.lastRun.IsZero() || now.Sub(j.lastRun) >= 23*time.Hour {
			return local.Hour() == j.spec.hour && local.Minute() == j.spec.minute
		}
		return false

	case kindWeekly:
		local := now.In(s.loc)
		if j.lastRun.IsZero() || now.Sub(j.lastRun) >= 6*24*time.Hour {
			return local.Weekday() == j.spec.weekday &&
				local.Hour() == j.spec.hour &&
				local.Minute() == j.spec.minute
		}
		return false
	}
	return false
}
