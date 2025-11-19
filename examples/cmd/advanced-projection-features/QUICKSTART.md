# Quick Start Guide

Get up and running in 2 minutes!

## 1. Start NATS Server

```bash
# Option A: Using Docker (recommended)
make nats-up

# Option B: Using local nats-server
nats-server -js
```

## 2. Run a Scenario

```bash
# Using make (easiest)
make basic
make rebuild
make monitor

# Using run script
./run.sh basic
./run.sh rebuild

# Direct go run
go run main.go basic
go run main.go rebuild
```

## 3. What to Try

### First Time? Start Here
```bash
make basic
```
Shows basic projection with checkpoint tracking.

### Want to See the Optimization?
```bash
make rebuild
```
Demonstrates no duplicate processing during rebuilds.

### Want to See It All?
```bash
make all
```
Runs all 5 scenarios back-to-back.

## Scenarios at a Glance

| Scenario | Command | What It Shows | Time |
|----------|---------|---------------|------|
| **basic** | `make basic` | Checkpoint tracking, consumer names | ~5s |
| **rebuild** | `make rebuild` | Rebuild optimization, no duplicates | ~10s |
| **interrupted** | `make interrupted` | Interrupted rebuild detection | ~5s |
| **monitor** | `make monitor` | Real-time checkpoint monitoring | 10s |
| **concurrent** | `make concurrent` | Events during rebuild (core scenario) | ~15s |

## Monitoring

### View NATS Consumer

```bash
nats consumer info DEMO_EVENTS projection_account-balance
```

### View Checkpoint

```sql
sqlite3 demo_projections.db "SELECT * FROM projection_checkpoints"
```

### View Projection Data

```sql
sqlite3 demo_projections.db "SELECT * FROM account_balances LIMIT 10"
```

## Clean Up

```bash
# Clean databases
make clean

# Stop NATS
make nats-down
```

## Next Steps

1. ✅ Run `make basic` to see it work
2. 📚 Read [README.md](./README.md) for detailed explanation
3. 🔍 Check [REBUILD_OPTIMIZATION.md](../../../REBUILD_OPTIMIZATION.md) for technical details
4. 🛠️ Adapt the code to your own projections

## Common Issues

**NATS not running?**
```bash
make nats-up
```

**Consumer not found?**
```bash
# Wait a few seconds after starting projection
# Or check: nats consumer ls DEMO_EVENTS
```

**Database locked?**
```bash
make clean
```

That's it! You're ready to explore advanced projection features. 🚀
