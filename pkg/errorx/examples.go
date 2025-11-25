package errorx

// This file contains examples of proper error handling patterns.
// These are not compiled - they serve as documentation.

/*

// ============================================================================
// Example 1: Repository Implementation
// ============================================================================

type AccountRepository struct {
	store EventStore
	db    *sql.DB
}

func (r *AccountRepository) Load(aggregateID string) (*Account, error) {
	// Validate input (APPLICATION ERROR)
	if aggregateID == "" {
		return nil, fmt.Errorf("aggregate_id: %w", ErrInvalidArgument)
	}

	// Query database (SYSTEM ERROR if database fails)
	events, err := r.store.LoadEvents(aggregateID)
	if err != nil {
		// Check if it's a "not found" (APPLICATION ERROR)
		if errors.Is(err, ErrNotFound) {
			return nil, NewNotFoundError("Aggregate", aggregateID)
		}
		// Otherwise it's a SYSTEM ERROR (db failure, network issue, etc.)
		return nil, fmt.Errorf("failed to load events from store: %w", err)
	}

	// Empty event stream means aggregate doesn't exist (APPLICATION ERROR)
	if len(events) == 0 {
		return nil, NewNotFoundError("Aggregate", aggregateID)
	}

	// Rebuild aggregate from events
	aggregate := NewAccount(aggregateID)
	for _, event := range events {
		if err := aggregate.Apply(event); err != nil {
			// Event application failure is a SYSTEM ERROR (data corruption)
			return nil, fmt.Errorf("%w: failed to apply event: %v", ErrDataCorruption, err)
		}
	}

	return aggregate, nil
}

func (r *AccountRepository) Save(aggregate *Account) error {
	uncommittedEvents := aggregate.GetUncommittedEvents()
	if len(uncommittedEvents) == 0 {
		return nil // No changes to save
	}

	// Attempt to append events with version check
	err := r.store.AppendEvents(
		aggregate.AggregateID(),
		aggregate.Version(),
		uncommittedEvents,
	)
	if err != nil {
		// Check for optimistic locking conflict (APPLICATION ERROR - retryable)
		if errors.Is(err, ErrConcurrencyConflict) {
			return NewConflictError(
				aggregate.AggregateID(),
				aggregate.Version(),
				-1, // actual version unknown
			)
		}

		// Check for unique constraint violation (APPLICATION ERROR)
		if errors.Is(err, ErrUniqueConstraintViolation) {
			// err should already have details
			return err
		}

		// Otherwise it's a SYSTEM ERROR (database failure, network, etc.)
		return fmt.Errorf("failed to append events: %w", err)
	}

	aggregate.MarkEventsAsCommitted()
	return nil
}

// ============================================================================
// Example 2: Command Handler
// ============================================================================

func (h *AccountHandler) OpenAccount(
	ctx context.Context,
	cmd *OpenAccountCommand,
) (*OpenAccountResponse, error) {
	// Validate command (APPLICATION ERROR)
	if cmd.AccountId == "" {
		return nil, fmt.Errorf("account_id: %w", ErrInvalidArgument)
	}
	if cmd.OwnerName == "" {
		return nil, fmt.Errorf("owner_name: %w", ErrInvalidArgument)
	}

	balance, err := decimal.NewFromString(cmd.InitialBalance)
	if err != nil || balance.IsNegative() {
		return nil, NewValidationError(
			"initial_balance",
			cmd.InitialBalance,
			"must be a non-negative number",
		)
	}

	// Create aggregate
	agg := domain.NewAccount(cmd.AccountId)

	// Emit event
	event := &AccountOpenedEvent{
		AccountId:      cmd.AccountId,
		OwnerName:      cmd.OwnerName,
		InitialBalance: cmd.InitialBalance,
		Timestamp:      time.Now().Unix(),
	}

	if err := agg.ApplyAccountOpenedEvent(event); err != nil {
		// Event application failure in NEW aggregate is a programming error (SYSTEM)
		return nil, fmt.Errorf("%w: failed to apply event: %v", ErrInternal, err)
	}

	// Save aggregate
	if err := h.repo.Save(agg); err != nil {
		// Check for APPLICATION errors first
		if errors.Is(err, ErrUniqueConstraintViolation) {
			// Already exists - return APPLICATION error
			return nil, fmt.Errorf("account %s: %w", cmd.AccountId, ErrAlreadyExists)
		}

		// Otherwise it's a SYSTEM error
		// Log with full context (for debugging)
		logger.Error("failed to save aggregate",
			"aggregate_id", cmd.AccountId,
			"error", err,
		)
		// Return sanitized error to client
		return nil, ErrInternal
	}

	return &OpenAccountResponse{
		AccountId: cmd.AccountId,
		Version:   agg.Version(),
	}, nil
}

// ============================================================================
// Example 3: Error Handling at API/Transport Layer
// ============================================================================

func (s *Server) HandleCommand(ctx context.Context, req *CommandRequest) (*CommandResponse, error) {
	// Call handler
	response, err := s.handler.Execute(ctx, req)
	if err != nil {
		// Classify the error
		if IsApplicationError(err) {
			// APPLICATION ERROR - return to client with details
			return nil, ConvertToProtocolError(err)
		}

		if IsSystemError(err) {
			// SYSTEM ERROR - log and sanitize
			logger.Error("system error during command execution",
				"command", req.CommandType,
				"error", err,
			)
			// Return generic error to client (security - don't leak internals)
			return nil, protocol.ErrInternal("an internal error occurred")
		}

		// Unknown error - treat as SYSTEM ERROR
		logger.Error("unexpected error during command execution",
			"command", req.CommandType,
			"error", err,
		)
		return nil, protocol.ErrInternal("an internal error occurred")
	}

	return response, nil
}

func ConvertToProtocolError(err error) error {
	// Map application errors to protocol errors
	switch {
	case errors.Is(err, ErrNotFound):
		return protocol.ErrNotFound(err.Error())
	case errors.Is(err, ErrAlreadyExists):
		return protocol.ErrAlreadyExists(err.Error())
	case errors.Is(err, ErrConflict):
		return protocol.ErrConflict(err.Error())
	case errors.Is(err, ErrInvalidArgument):
		return protocol.ErrInvalidArgument(err.Error())
	case errors.Is(err, ErrPermissionDenied):
		return protocol.ErrPermissionDenied(err.Error())
	case errors.Is(err, ErrUnauthenticated):
		return protocol.ErrUnauthenticated(err.Error())
	case errors.Is(err, ErrResourceExhausted):
		return protocol.ErrResourceExhausted(err.Error())
	default:
		return protocol.ErrInternal("an error occurred")
	}
}

// ============================================================================
// Example 4: Retry Logic with Retryable Errors
// ============================================================================

func (h *AccountHandler) Deposit(
	ctx context.Context,
	cmd *DepositCommand,
) (*DepositResponse, error) {
	const maxRetries = 3

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		response, err := h.tryDeposit(ctx, cmd)
		if err == nil {
			return response, nil
		}

		lastErr = err

		// Check if error is retryable
		if !IsRetryable(err) {
			// Not retryable - return immediately
			return nil, err
		}

		// It's retryable (concurrency conflict, timeout, etc.)
		if attempt < maxRetries-1 {
			// Wait before retry (exponential backoff)
			backoff := time.Duration(1<<attempt) * 100 * time.Millisecond
			time.Sleep(backoff)
			continue
		}
	}

	// Max retries exceeded
	return nil, fmt.Errorf("operation failed after %d retries: %w", maxRetries, lastErr)
}

func (h *AccountHandler) tryDeposit(
	ctx context.Context,
	cmd *DepositCommand,
) (*DepositResponse, error) {
	// Load aggregate
	agg, err := h.repo.Load(cmd.AccountId)
	if err != nil {
		return nil, err // Propagate error (APPLICATION or SYSTEM)
	}

	// Emit event
	event := &MoneyDepositedEvent{
		AccountId: cmd.AccountId,
		Amount:    cmd.Amount,
		Timestamp: time.Now().Unix(),
	}

	if err := agg.ApplyMoneyDepositedEvent(event); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInternal, err)
	}

	// Save (might fail with concurrency conflict - retryable)
	if err := h.repo.Save(agg); err != nil {
		return nil, err // Propagate (will be retried if retryable)
	}

	return &DepositResponse{
		NewBalance: agg.Balance,
		Version:    agg.Version(),
	}, nil
}

// ============================================================================
// Example 5: Query Handler with Proper Error Handling
// ============================================================================

func (h *AccountQueryHandler) GetAccount(
	ctx context.Context,
	query *GetAccountRequest,
) (*AccountView, error) {
	// Validate query (APPLICATION ERROR)
	if query.AccountId == "" {
		return nil, fmt.Errorf("account_id: %w", ErrInvalidArgument)
	}

	// Load aggregate
	agg, err := h.repo.Load(query.AccountId)
	if err != nil {
		// Check if it's "not found" (APPLICATION ERROR)
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrAggregateNotFound) {
			return nil, NewNotFoundError("Account", query.AccountId)
		}
		// Otherwise it's a SYSTEM ERROR
		logger.Error("failed to load account",
			"account_id", query.AccountId,
			"error", err,
		)
		return nil, ErrInternal
	}

	// Convert to view
	return &AccountView{
		AccountId: agg.AccountId,
		OwnerName: agg.OwnerName,
		Balance:   agg.Balance,
		Status:    agg.Status,
		Version:   agg.Version(),
	}, nil
}

// ============================================================================
// Summary: Error Handling Decision Tree
// ============================================================================

/*

When you encounter an error, ask:

1. Is it an EXPECTED error (business logic, validation, not found)?
   YES -> Return APPLICATION error with helpful message
   - Use sentinel errors (ErrNotFound, ErrInvalidArgument, etc.)
   - Include context (field names, IDs, etc.)
   - Can be returned directly to clients
   - Example: return NewNotFoundError("Account", id)

2. Is it a RETRYABLE error (concurrency, timeout)?
   YES -> Implement retry logic
   - Use IsRetryable(err) to check
   - Implement exponential backoff
   - Limit retry attempts
   - Example: Optimistic locking conflicts

3. Is it an UNEXPECTED error (database failure, network issue)?
   YES -> Return SYSTEM error with sanitization
   - Log full error with context
   - Return generic error to client (security)
   - Monitor and alert
   - Example: return ErrInternal after logging

Decision Matrix:

┌─────────────────────┬──────────────────┬────────────────────┬─────────────────┐
│ Error Type          │ Classification   │ Action             │ Client Response │
├─────────────────────┼──────────────────┼────────────────────┼─────────────────┤
│ Not Found           │ APPLICATION      │ Return with ID     │ 404 + details   │
│ Already Exists      │ APPLICATION      │ Return with ID     │ 409 + details   │
│ Invalid Input       │ APPLICATION      │ Return with field  │ 400 + details   │
│ Version Conflict    │ APPLICATION      │ Retry or return    │ 409 + details   │
│ Permission Denied   │ APPLICATION      │ Return             │ 403 + message   │
│ Database Failure    │ SYSTEM           │ Log + sanitize     │ 500 + generic   │
│ Network Timeout     │ SYSTEM           │ Log + retry/fail   │ 503 + generic   │
│ Data Corruption     │ SYSTEM           │ Log + alert        │ 500 + generic   │
└─────────────────────┴──────────────────┴────────────────────┴─────────────────┘

*/
