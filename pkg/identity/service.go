package identity

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/plaenen/eventstore/pkg/errorx"
	"github.com/plaenen/eventstore/pkg/identity/nats"
	identityv1 "github.com/plaenen/eventstore/pkg/identity/v1"
	"github.com/plaenen/eventstore/pkg/identity/v1/identityv1connect"
	"github.com/plaenen/eventstore/pkg/runner"
)

var _ identityv1connect.IdentityServiceHandler = (*Service)(nil)

// Service implements identityv1connect.IdentityServiceHandler.
type Service struct {
	accountManager *nats.AccountManager
	logger         runner.Logger
}

// NewService creates a new Identity Service.
func NewService(accountManager *nats.AccountManager, logger runner.Logger) *Service {
	return &Service{
		accountManager: accountManager,
		logger:         logger,
	}
}

// ExchangeToken exchanges a third-party token for NATS credentials.
func (s *Service) ExchangeToken(
	ctx context.Context,
	req *connect.Request[identityv1.ExchangeTokenRequest],
) (*connect.Response[identityv1.ExchangeTokenResponse], error) {
	// 1. Validate Request
	if req.Msg.TenantId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errorx.ErrInvalidArgument)
	}
	if req.Msg.UserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errorx.ErrInvalidArgument)
	}
	// In a real implementation, we would validate req.Msg.Token here.
	// For now, we assume the token is valid if present (or just ignore it for this demo).

	// 2. Create NATS User
	// This will also create the Account if it doesn't exist (if we added that logic to AccountManager,
	// but currently AccountManager.CreateUser requires the account seed to exist).
	// So we assume the Account has been provisioned.
	// TODO: Auto-provision account if missing? Or return error?
	// For now, let's try to create the user.

	jwt, creds, err := s.accountManager.CreateUser(ctx, req.Msg.TenantId, req.Msg.UserId)
	if err != nil {
		s.logger.Error("failed to create nats user", "error", err, "tenant_id", req.Msg.TenantId, "user_id", req.Msg.UserId)
		// If account doesn't exist, we might want to create it on the fly for "Onboarding" flow.
		// But strictly speaking, CreateUser fails if account seed is missing.
		return nil, connect.NewError(connect.CodeInternal, errorx.ErrInternal)
	}

	// 3. Return Response
	res := connect.NewResponse(&identityv1.ExchangeTokenResponse{
		Jwt:   jwt,
		Creds: string(creds),
		// Seed is embedded in creds, but we can extract it if needed.
		// The proto has a 'seed' field, but nkeys/jwt doesn't easily give just the seed string from creds
		// without parsing. The 'creds' file is usually what clients need.
		// We'll leave 'seed' empty or parse it if strictly required.
		ExpiresAt: time.Now().Add(24 * time.Hour).Unix(), // Mock expiration
	})

	return res, nil
}
