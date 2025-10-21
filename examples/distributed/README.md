# Distributed Architecture Example

This example demonstrates the **thin client pattern** for microservices architecture where:

1. **API Gateway** (Command Sender) - Thin client with NO database
2. **Command Handler Service** - Has event store and processes commands
3. **NATS** - Distributed command bus

## Architecture

```
┌─────────────────┐                  ┌──────────────────────┐
│  API Gateway    │                  │  Handler Service     │
│  (Port 8080)    │                  │  (Background)        │
│                 │                  │                      │
│  SDK Client     │                  │  SDK Client          │
│  Role: Sender   │  ─── NATS ────>  │  Role: Handler       │
│  NO DATABASE    │    Commands      │  + Event Store (DB)  │
│                 │                  │  + Command Handlers  │
│  Just sends     │                  │  + Business Logic    │
│  commands       │                  │                      │
└─────────────────┘                  └──────────────────────┘
```

## Running the Example

### Terminal 1: Start NATS Server
```bash
# Option 1: Docker
docker run -p 4222:4222 nats:latest

# Option 2: Native
nats-server
```

### Terminal 2: Start Command Handler Service
```bash
cd examples/distributed
go run ./cmd/handler
```

Output:
```
🔧 Command Handler Service Starting...
✅ Connected to NATS at nats://localhost:4222
✅ Event store initialized: ./data/handler.db
✅ Registered command handlers
⏳ Listening for commands...
```

### Terminal 3: Start API Gateway
```bash
cd examples/distributed
go run ./cmd/gateway
```

Output:
```
🌐 API Gateway Starting...
✅ Connected to NATS at nats://localhost:4222
✅ Thin client (no database) - Role: CommandSender
🚀 HTTP Server listening on :8080
```

### Terminal 4: Send Commands
```bash
# Open account
curl -X POST http://localhost:8080/accounts \
  -H "Content-Type: application/json" \
  -d '{
    "account_id": "acc-001",
    "owner_name": "Alice",
    "initial_balance": "1000.00"
  }'

# Deposit
curl -X POST http://localhost:8080/accounts/acc-001/deposit \
  -H "Content-Type: application/json" \
  -d '{
    "amount": "500.00"
  }'

# Withdraw
curl -X POST http://localhost:8080/accounts/acc-001/withdraw \
  -H "Content-Type: application/json" \
  -d '{
    "amount": "200.00"
  }'
```

## Key Differences

### API Gateway (Thin Client)
```go
// NO DATABASE!
client, _ := unifiedsdk.New(
    unifiedsdk.WithMode(sdk.ProductionMode),
    unifiedsdk.WithRole(sdk.RoleCommandSender),  // ← Thin client
    unifiedsdk.WithNATSURL("nats://localhost:4222"),
    // NO WithSQLiteDSN - not needed!
)

// Just send commands to NATS
client.Account.OpenAccount(ctx, cmd, principalID)
```

### Handler Service (Full Stack)
```go
// HAS DATABASE
client, _ := unifiedsdk.New(
    unifiedsdk.WithMode(sdk.ProductionMode),
    unifiedsdk.WithRole(sdk.RoleCommandHandler),  // ← Handler
    unifiedsdk.WithNATSURL("nats://localhost:4222"),
    unifiedsdk.WithSQLiteDSN("./data/handler.db"),  // ← Event store
)

// Register command handlers
client.Client().RegisterCommandHandler("account.v1.OpenAccountCommand", handler)
```

## Benefits

✅ **True Microservices** - Services can scale independently
✅ **Thin API Gateways** - No database overhead on frontend
✅ **Separation of Concerns** - API layer vs. business logic layer
✅ **Horizontal Scaling** - Multiple gateways, multiple handlers
✅ **Resource Efficiency** - Database only where needed

## Scaling Pattern

### Multiple API Gateways
```bash
# Terminal 1: Gateway on port 8080
PORT=8080 go run ./cmd/gateway

# Terminal 2: Gateway on port 8081
PORT=8081 go run ./cmd/gateway

# Load balancer distributes across both
```

### Multiple Command Handlers
```bash
# Terminal 1: Handler with DB shard 1
DB_PATH=./data/handler1.db go run ./cmd/handler

# Terminal 2: Handler with DB shard 2
DB_PATH=./data/handler2.db go run ./cmd/handler

# NATS distributes commands across handlers
```

## Production Deployment

```yaml
# kubernetes/api-gateway.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-gateway
spec:
  replicas: 3  # Multiple thin clients
  template:
    spec:
      containers:
      - name: gateway
        image: myapp/gateway:latest
        env:
        - name: ROLE
          value: "sender"  # Thin client
        - name: NATS_URL
          value: "nats://nats-cluster:4222"
```

```yaml
# kubernetes/command-handler.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: command-handler
spec:
  replicas: 2  # Multiple handlers
  template:
    spec:
      containers:
      - name: handler
        image: myapp/handler:latest
        env:
        - name: ROLE
          value: "handler"
        - name: NATS_URL
          value: "nats://nats-cluster:4222"
        - name: DB_PATH
          value: "/data/events.db"
        volumeMounts:
        - name: data
          mountPath: /data
```
