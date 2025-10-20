package middleware

import (
	"context"
	"fmt"

	"github.com/plaenen/eventsourcing/pkg/eventsourcing"
)

// Authorizer defines the interface for authorization checks.
type Authorizer interface {
	// Authorize checks if the principal is authorized to execute the command.
	Authorize(ctx context.Context, principalID string, commandType string, command interface{}) error
}

// AuthorizationMiddleware enforces authorization for commands.
func AuthorizationMiddleware(authorizer Authorizer) eventsourcing.CommandMiddleware {
	return func(next eventsourcing.CommandHandler) eventsourcing.CommandHandler {
		return eventsourcing.CommandHandlerFunc(func(ctx context.Context, cmd *eventsourcing.CommandEnvelope) ([]*eventsourcing.Event, error) {
			commandType := cmd.Metadata.Custom["command_type"]
			principalID := cmd.Metadata.PrincipalID

			// Check authorization
			if err := authorizer.Authorize(ctx, principalID, commandType, cmd.Command); err != nil {
				return nil, fmt.Errorf("authorization failed: %w", err)
			}

			return next.Handle(ctx, cmd)
		})
	}
}

// RoleBasedAuthorizer implements simple role-based authorization.
type RoleBasedAuthorizer struct {
	// commandRoles maps command types to required roles
	commandRoles map[string][]string
	// principalRoles provides roles for a principal
	principalRoles func(ctx context.Context, principalID string) ([]string, error)
}

// NewRoleBasedAuthorizer creates a role-based authorizer.
func NewRoleBasedAuthorizer(
	commandRoles map[string][]string,
	principalRoles func(ctx context.Context, principalID string) ([]string, error),
) *RoleBasedAuthorizer {
	return &RoleBasedAuthorizer{
		commandRoles:   commandRoles,
		principalRoles: principalRoles,
	}
}

func (a *RoleBasedAuthorizer) Authorize(ctx context.Context, principalID string, commandType string, command interface{}) error {
	// Get required roles for command
	requiredRoles, exists := a.commandRoles[commandType]
	if !exists || len(requiredRoles) == 0 {
		// No authorization required
		return nil
	}

	// Get principal roles
	principalRolesList, err := a.principalRoles(ctx, principalID)
	if err != nil {
		return fmt.Errorf("failed to get principal roles: %w", err)
	}

	// Check if principal has any required role
	principalRolesMap := make(map[string]bool)
	for _, role := range principalRolesList {
		principalRolesMap[role] = true
	}

	for _, requiredRole := range requiredRoles {
		if principalRolesMap[requiredRole] {
			return nil // Authorized
		}
	}

	return fmt.Errorf("principal %s lacks required role for command %s (required: %v)", principalID, commandType, requiredRoles)
}
