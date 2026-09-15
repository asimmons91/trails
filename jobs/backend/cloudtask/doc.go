// Package cloudtask implements a jobs.Backend backed by Google Cloud Tasks
// (https://docs.cloud.google.com/tasks/docs). Enqueue creates a Cloud Task
// that Cloud Tasks later pushes back into an HTTP endpoint this package
// mounts via trails.Spur; Cloud Tasks itself durably owns queueing,
// scheduling (via each task's ScheduleTime), and delivery retries.
//
// A small amount of local state, stored via the trails Pack ORM (see
// Migration), covers what Cloud Tasks has no concept of: a per-job attempt
// counter independent of the Cloud Tasks queue's own RetryConfig, and a
// concurrency-window guard for jobs.Enqueued's ConcurrencyKey/Limit/Duration
// fields. Because execution is push-driven rather than polled, that guard
// (claim.go) has to hold correctly under many concurrent HTTP requests
// racing over a possibly brand-new concurrency key, which a dedicated
// per-key lock row (slot.go) exists to make safe.
//
// Periodic cleanup of old rows is a self-rescheduling Cloud Tasks chain
// (schedule.go/cleanup.go) rather than an in-process goroutine: the host
// app is assumed to run as a Cloud Run service that can scale to zero
// between requests, so nothing about this backend depends on a long-lived
// process. A lightweight check on every Enqueue call bootstraps or heals
// the chain if it was never started or has gone stale.
//
// # What the host app must provision
//
//   - A Cloud Tasks queue per logical jobs.Enqueued.Queue name this backend
//     is configured to use (see Config and WithQueueName) — this package
//     never creates a queue itself.
//   - A service account (Config.ServiceAccountEmail) Cloud Tasks mints an
//     OIDC token for on every push, which the push handler verifies against
//     Config.Audience.
//   - Config.BaseURL: the publicly reachable scheme+host+mount-prefix Cloud
//     Tasks calls back into, matching wherever this Spur is actually
//     mounted in the host's router.
//   - Running Migration against the app's database before using this
//     backend (see Migration; not self-registered, the host decides when).
package cloudtask
