// Package notify turns a monitoring alert into a message that reaches a human.
//
// # The shape of the thing
//
// A caller (the monitoring service, an operator pressing "test") hands in an
// Alert. Three steps follow, and they are deliberately separated:
//
//  1. Deduplication (dedupe.go). Decide answers one question: should this
//     observation produce a message at all? A disk sitting at 92% for six
//     hours is one incident, not seventy-two messages. Decide is a pure
//     function over the previous state, so the policy can be tested without a
//     database and audited without reading SQL.
//
//  2. Rendering (template.go). The alert becomes a subject and a body that
//     carry what an operator needs in order to act: which server, which
//     resource, the measured value, the threshold it crossed, and a link
//     straight to the panel page for that server. Rendering happens once, at
//     enqueue time, so editing a template does not change the wording of a
//     message that is already in flight.
//
//  3. Delivery (dispatcher.go, and the senders). Nothing is sent on the
//     request path. The rendered message is written to the
//     notification_deliveries outbox and a background dispatcher picks it up,
//     retries with exponential backoff, and dead-letters after a bounded
//     number of attempts.
//
// # Why the outbox is in Postgres and not in the Redis job queue
//
// internal/job has an asynq queue, and it was measured before this package was
// written rather than assumed to work. Two things were found. Its worker
// ignores the Redis address, password and database passed to NewQueueManager
// and hardcodes localhost:6379 db 0, so a panel configured against any other
// Redis enqueues into one server and consumes from another - the task sits
// pending forever with a worker running. And its notification handler
// unmarshals the payload, logs a line, sleeps 500ms and returns nil: a task
// routed to it is reported successful having sent nothing.
//
// Even with both fixed, an alert that exists only in Redis is an alert that a
// Redis restart deletes without trace. Alerting is trusted, and a trusted
// channel that silently drops is worse than no channel at all, so the queue of
// record here is the database. The dispatcher is a plain background loop over
// that table: it survives a panel restart, it is visible to an operator over
// the API, and it needs nothing beyond the connection the panel already holds.
//
// # Secrets
//
// Channel configuration holds SMTP passwords and bot tokens. Three rules hold
// throughout this package, and each has a test that reads the actual output
// rather than trusting the intent:
//
//   - Secret values never reach a log field. The dispatcher logs the channel's
//     name and type, never its config.
//   - Secret values never reach an error string. Every sender scrubs its own
//     credentials out of the errors it returns, and the dispatcher scrubs
//     again from the channel config before anything is logged or written to
//     notification_deliveries.last_error. The Telegram bot token is the reason
//     the second pass exists: it lives in the request URL, so any transport
//     error carries it.
//   - Secret values never reach an API response. RedactConfig replaces them on
//     every read path, and MergeConfig makes sure a client that writes a
//     redacted config back does not overwrite the stored credential with the
//     placeholder.
package notify
