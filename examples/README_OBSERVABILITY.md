# Running the SQLite Observability Demo

## Quick Start

```bash
# From the examples directory
cd examples

# Build the demo
go build -o bankaccount_sqlite_obs_demo bankaccount_sqlite_observability_demo.go

# Run it
./bankaccount_sqlite_obs_demo
```

## What It Does

The demo will:

1. Start an embedded NATS server on port 4222
2. Create two SQLite database files:
   - **`eventstore.db`** - All domain events and snapshots
   - **`observability.db`** - All traces and metrics
3. Execute several banking transactions
4. Query and display observability data
5. Wait 12 seconds for metrics to be exported
6. Show you how to query the databases

## Expected Output

You'll see output like:

```
=== Bank Account SQLite Observability Demo ===
Single-binary application with file-based storage

This demo creates two SQLite database files:
  • eventstore.db - Event sourcing data (aggregates, events)
  • observability.db - Traces and metrics

1️⃣  Starting embedded NATS server...
   ✅ Embedded NATS server ready

2️⃣  Setting up SQLite for observability...
   📁 Observability database: ./observability.db
   ✅ SQLite observability initialized

3️⃣  Setting up SQLite event store...
   📁 Event store database: ./eventstore.db
   ✅ Event store ready

... (transactions execute) ...

9️⃣  Waiting for metrics export...
   ✅ Metrics exported to SQLite

🔟 Querying observability data from SQLite...
   📊 Recent traces:
      Found X traces

   🔗 Spans for operations:
      Found X spans

   📈 Metric summary:
      ...

🎉 Demo Complete!
```

## Database Files Created

After running, you'll have two SQLite files in the `examples` directory:

```bash
ls -lh *.db
# -rw-r--r--  1 user  staff   XXK  eventstore.db
# -rw-r--r--  1 user  staff   XXK  observability.db
```

## Querying the Databases

### Event Store Queries

```bash
# View all events
sqlite3 eventstore.db "SELECT aggregate_id, event_type, version FROM events"

# View events for specific account
sqlite3 eventstore.db "
  SELECT event_type, version, timestamp
  FROM events
  WHERE aggregate_id = 'acc-sqlite-demo-789'
  ORDER BY version
"

# Count events by type
sqlite3 eventstore.db "
  SELECT event_type, COUNT(*) as count
  FROM events
  GROUP BY event_type
"
```

### Observability Queries

```bash
# View all traces
sqlite3 observability.db "SELECT trace_id, created_at FROM otel_traces LIMIT 10"

# View spans with durations
sqlite3 observability.db "
  SELECT
    name,
    (end_time - start_time)/1000000 as duration_ms
  FROM otel_spans
  ORDER BY duration_ms DESC
  LIMIT 10
"

# Count operations
sqlite3 observability.db "
  SELECT name, COUNT(*) as count
  FROM otel_spans
  GROUP BY name
"

# View metrics
sqlite3 observability.db "
  SELECT name, type, COUNT(*) as data_points
  FROM otel_metrics
  GROUP BY name, type
"
```

### Pretty Output

For better formatted output, use SQLite's column mode:

```bash
sqlite3 observability.db << EOF
.headers on
.mode column
.width 40 15 10

SELECT
  name,
  COUNT(*) as span_count,
  ROUND(AVG((end_time - start_time)/1000000), 2) as avg_ms
FROM otel_spans
GROUP BY name;
EOF
```

## Cleanup

To remove the database files:

```bash
rm -f eventstore.db observability.db
```

## Alternative: Run Without Building

```bash
# Run directly with go run (slower startup)
go run bankaccount_sqlite_observability_demo.go
```

## Troubleshooting

### Port Already in Use

If you see an error about port 4222 being in use:

```bash
# Find what's using the port
lsof -i :4222

# Kill it if needed
kill -9 <PID>
```

### Database Locked

If you get "database is locked" errors:

```bash
# Make sure no other processes are using the databases
lsof eventstore.db observability.db

# Or just remove and recreate them
rm -f *.db
```

### Permission Denied

If you can't write database files:

```bash
# Make sure the examples directory is writable
ls -ld .

# Run from a different directory
cd /tmp
/path/to/bankaccount_sqlite_obs_demo
```

## What's Next?

After running the demo, try:

1. **Query the databases** - Use the SQL examples above
2. **Inspect the schema** - Run `.schema` in sqlite3
3. **Write custom queries** - Analyze performance patterns
4. **Keep the databases** - Use them for historical analysis
5. **Build your own app** - Use the demo as a template

See the main [OBSERVABILITY.md](OBSERVABILITY.md) for complete documentation.
