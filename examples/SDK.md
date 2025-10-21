# Unified SDK

The Unified SDK provides a developer-friendly interface for interacting with event-sourced services. It combines all commands and queries into a single client that only requires a transport.

## Quick Start

```go
import (
    accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
    natspkg "github.com/plaenen/eventstore/pkg/nats"
    "github.com/plaenen/eventstore/pkg/eventsourcing"
)

// Create transport
transport, _ := natspkg.NewTransport(&natspkg.TransportConfig{
    TransportConfig: eventsourcing.DefaultTransportConfig(),
    URL:             "nats://localhost:4222",
    Name:            "my-client",
})
defer transport.Close()

// Create SDK - only needs transport!
sdk := accountv1.NewAccountSDK(transport)

// Use the SDK
resp, err := sdk.OpenAccount(ctx, &accountv1.OpenAccountCommand{
    AccountId:      "acc-123",
    OwnerName:      "John Doe",
    InitialBalance: "1000.00",
})
```

## Benefits

### 1. Single Import
Instead of managing separate command and query clients:
```go
// Old way - separate clients
commandClient := accountv1.NewAccountClient(transport)
queryClient := accountv1.NewAccountClient(transport)

// New way - unified SDK
sdk := accountv1.NewAccountSDK(transport)
```

### 2. Only Requires Transport
No need to manage multiple transports or connections:
```go
// Create once
transport := natspkg.NewTransport(config)

// Use everywhere
sdk := accountv1.NewAccountSDK(transport)
```

### 3. Type-Safe Methods
All operations are type-safe with auto-completion support:
```go
// Commands
sdk.OpenAccount(ctx, &accountv1.OpenAccountCommand{...})
sdk.Deposit(ctx, &accountv1.DepositCommand{...})
sdk.Withdraw(ctx, &accountv1.WithdrawCommand{...})
sdk.CloseAccount(ctx, &accountv1.CloseAccountCommand{...})

// Queries
sdk.GetAccount(ctx, &accountv1.GetAccountRequest{...})
sdk.GetAccountBalance(ctx, &accountv1.GetAccountBalanceRequest{...})
sdk.GetAccountHistory(ctx, &accountv1.GetAccountHistoryRequest{...})
sdk.ListAccounts(ctx, &accountv1.ListAccountsRequest{...})
```

### 4. Clean Error Handling
Consistent error handling across all operations:
```go
resp, appErr := sdk.Deposit(ctx, &accountv1.DepositCommand{
    AccountId: "acc-123",
    Amount:    "100.00",
})
if appErr != nil {
    log.Printf("Error: [%s] %s", appErr.Code, appErr.Message)
    return
}
```

## Complete Example

See [unified_sdk_demo.go](./unified_sdk_demo.go) for a complete working example.

```bash
go run unified_sdk_demo.go
```

## API Reference

### Account SDK

#### Constructor

```go
func NewAccountSDK(transport eventsourcing.Transport) *AccountSDK
```

Creates a new unified SDK for the Account service.

**Parameters:**
- `transport`: Any transport implementation (NATS, HTTP, etc.)

**Returns:**
- `*AccountSDK`: Ready-to-use SDK instance

#### Commands

##### OpenAccount
```go
func (s *AccountSDK) OpenAccount(ctx context.Context, cmd *OpenAccountCommand) (*OpenAccountResponse, *eventsourcing.AppError)
```

Opens a new account.

**Fields:**
- `AccountId`: Unique identifier for the account
- `OwnerName`: Name of the account owner
- `InitialBalance`: Starting balance (decimal string, e.g., "1000.00")

##### Deposit
```go
func (s *AccountSDK) Deposit(ctx context.Context, cmd *DepositCommand) (*DepositResponse, *eventsourcing.AppError)
```

Deposits money into an account.

**Fields:**
- `AccountId`: Account to deposit to
- `Amount`: Amount to deposit (decimal string)

##### Withdraw
```go
func (s *AccountSDK) Withdraw(ctx context.Context, cmd *WithdrawCommand) (*WithdrawResponse, *eventsourcing.AppError)
```

Withdraws money from an account.

**Fields:**
- `AccountId`: Account to withdraw from
- `Amount`: Amount to withdraw (decimal string)

**Errors:**
- `INSUFFICIENT_FUNDS`: Not enough balance
- `ACCOUNT_CLOSED`: Account is closed

##### CloseAccount
```go
func (s *AccountSDK) CloseAccount(ctx context.Context, cmd *CloseAccountCommand) (*CloseAccountResponse, *eventsourcing.AppError)
```

Closes an account.

**Fields:**
- `AccountId`: Account to close

#### Queries

##### GetAccount
```go
func (s *AccountSDK) GetAccount(ctx context.Context, query *GetAccountRequest) (*AccountView, *eventsourcing.AppError)
```

Retrieves full account details.

**Returns:**
- `AccountView` with fields: `AccountId`, `OwnerName`, `Balance`, `Status`, `Version`

##### GetAccountBalance
```go
func (s *AccountSDK) GetAccountBalance(ctx context.Context, query *GetAccountBalanceRequest) (*BalanceView, *eventsourcing.AppError)
```

Retrieves just the account balance (lightweight query).

**Returns:**
- `BalanceView` with `Balance` field

##### GetAccountHistory
```go
func (s *AccountSDK) GetAccountHistory(ctx context.Context, query *GetAccountHistoryRequest) (*AccountHistoryResponse, *eventsourcing.AppError)
```

Retrieves transaction history for an account.

**Returns:**
- `AccountHistoryResponse` with `Transactions` array

##### ListAccounts
```go
func (s *AccountSDK) ListAccounts(ctx context.Context, query *ListAccountsRequest) (*ListAccountsResponse, *eventsourcing.AppError)
```

Lists all accounts.

**Returns:**
- `ListAccountsResponse` with `Accounts` array

#### Utility Methods

##### Transport
```go
func (s *AccountSDK) Transport() eventsourcing.Transport
```

Returns the underlying transport. Useful for advanced use cases.

##### Close
```go
func (s *AccountSDK) Close() error
```

Closes the underlying transport connection.

## Error Handling

All SDK methods return `*eventsourcing.AppError` instead of `error`. This provides structured error information:

```go
type AppError struct {
    Code     string  // Error code (e.g., "INSUFFICIENT_FUNDS")
    Message  string  // Human-readable message
    Solution string  // Optional suggested solution
}
```

### Common Error Codes

- `TRANSPORT_ERROR`: Network or communication error
- `INVALID_RESPONSE`: Malformed response from server
- `INSUFFICIENT_FUNDS`: Not enough balance for withdrawal
- `ACCOUNT_CLOSED`: Operation on closed account
- `INVALID_ACCOUNT_ID`: Account ID is missing or invalid
- `INVALID_AMOUNT`: Amount is invalid or negative

### Error Handling Pattern

```go
resp, appErr := sdk.Deposit(ctx, cmd)
if appErr != nil {
    switch appErr.Code {
    case "INSUFFICIENT_FUNDS":
        // Handle insufficient funds
    case "ACCOUNT_CLOSED":
        // Handle closed account
    default:
        // Handle other errors
        log.Printf("Error: [%s] %s", appErr.Code, appErr.Message)
    }
    return
}

// Success - use resp
fmt.Printf("New balance: %s\n", resp.NewBalance)
```

## Context & Timeouts

All SDK methods accept a `context.Context` for timeout and cancellation:

```go
// With timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

resp, err := sdk.GetAccount(ctx, &accountv1.GetAccountRequest{
    AccountId: "acc-123",
})
```

## Best Practices

### 1. Reuse the SDK Instance
Create one SDK instance per transport and reuse it:

```go
// ✅ Good - reuse SDK
sdk := accountv1.NewAccountSDK(transport)
for _, accountID := range accounts {
    sdk.GetAccount(ctx, &accountv1.GetAccountRequest{AccountId: accountID})
}

// ❌ Bad - creating new SDK for each call
for _, accountID := range accounts {
    sdk := accountv1.NewAccountSDK(transport)  // Wasteful!
    sdk.GetAccount(ctx, &accountv1.GetAccountRequest{AccountId: accountID})
}
```

### 2. Use Context for Timeouts
Always provide appropriate timeouts:

```go
// ✅ Good - with timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
resp, err := sdk.Deposit(ctx, cmd)

// ❌ Bad - no timeout
resp, err := sdk.Deposit(context.Background(), cmd)
```

### 3. Handle Errors Appropriately
Check error codes for business logic:

```go
// ✅ Good - specific error handling
resp, appErr := sdk.Withdraw(ctx, cmd)
if appErr != nil {
    if appErr.Code == "INSUFFICIENT_FUNDS" {
        return fmt.Errorf("not enough balance")
    }
    return fmt.Errorf("withdraw failed: %s", appErr.Message)
}

// ❌ Bad - ignoring error codes
resp, appErr := sdk.Withdraw(ctx, cmd)
if appErr != nil {
    panic(appErr.Message)  // Don't panic, handle gracefully
}
```

### 4. Close Transport When Done
Ensure proper cleanup:

```go
transport, _ := natspkg.NewTransport(config)
defer transport.Close()  // ✅ Good

sdk := accountv1.NewAccountSDK(transport)
// Use sdk...

// Or use SDK's convenience method
defer sdk.Close()  // ✅ Also good
```

## Advanced Usage

### Custom Transport Configuration

```go
config := &natspkg.TransportConfig{
    TransportConfig: &eventsourcing.TransportConfig{
        Timeout:              30 * time.Second,
        MaxReconnectAttempts: 5,
        ReconnectWait:        2 * time.Second,
        MaxRetries:           3,
    },
    URL:  "nats://localhost:4222",
    Name: "my-advanced-client",
}

transport, _ := natspkg.NewTransport(config)
sdk := accountv1.NewAccountSDK(transport)
```

### Multiple Services

If you have multiple services, create separate SDK instances:

```go
// Account service
accountSDK := accountv1.NewAccountSDK(transport)

// User service (when implemented)
// userSDK := userv1.NewUserSDK(transport)

// Same transport, different SDKs
```

### Testing

For testing, you can use in-memory transport or mock the SDK:

```go
// In tests
transport := &MockTransport{...}
sdk := accountv1.NewAccountSDK(transport)

// Test your code that uses the SDK
```

## Migration Guide

If you're using the old client approach, here's how to migrate:

### Before (Old Way)
```go
client := accountv1.NewAccountClient(transport)
resp, err := client.Deposit(ctx, &accountv1.DepositCommand{...})
```

### After (Unified SDK)
```go
sdk := accountv1.NewAccountSDK(transport)
resp, err := sdk.Deposit(ctx, &accountv1.DepositCommand{...})
```

The API is identical - just replace `NewAccountClient` with `NewAccountSDK`!

## Troubleshooting

### "Transport error"
- Check that the NATS server is running
- Verify the NATS URL is correct
- Check network connectivity

### "Invalid response"
- Ensure server and client are using compatible protobuf definitions
- Check server logs for errors

### "Timeout"
- Increase context timeout
- Check server performance
- Verify network latency

## Support

For issues or questions:
- Check the [examples](./unified_sdk_demo.go)
- Review error codes and messages
- Check server logs
- Ensure versions match between client and server
