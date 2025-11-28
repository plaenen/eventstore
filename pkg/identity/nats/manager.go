package nats

import (
	"context"
	"fmt"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/plaenen/eventstore/pkg/errorx"
	"github.com/plaenen/eventstore/pkg/runner"
)

// AccountManager manages NATS Accounts and Users using Decentralized Auth (JWTs).
type AccountManager struct {
	operatorSeed string
	keyStore     KeyStore
	nc           *nats.Conn
	logger       runner.Logger
}

// NewAccountManager creates a new AccountManager.
func NewAccountManager(operatorSeed string, keyStore KeyStore, nc *nats.Conn, logger runner.Logger) *AccountManager {
	return &AccountManager{
		operatorSeed: operatorSeed,
		keyStore:     keyStore,
		nc:           nc,
		logger:       logger,
	}
}

// CreateAccount creates a new NATS Account for a tenant, signs it with the Operator key,
// and publishes it to the NATS Resolver.
func (m *AccountManager) CreateAccount(ctx context.Context, tenantID string) (string, error) {
	// 1. Create Account Keypair
	accKp, err := nkeys.CreateAccount()
	if err != nil {
		return "", errorx.Wrap(err, "failed to create account keypair")
	}

	accPub, err := accKp.PublicKey()
	if err != nil {
		return "", errorx.Wrap(err, "failed to get account public key")
	}

	accSeed, err := accKp.Seed()
	if err != nil {
		return "", errorx.Wrap(err, "failed to get account seed")
	}

	// 2. Create Account Claims
	claims := jwt.NewAccountClaims(accPub)
	claims.Name = tenantID
	claims.Limits.JetStreamLimits = jwt.JetStreamLimits{
		MemoryStorage: -1, // Unlimited
		DiskStorage:   -1, // Unlimited
		Streams:       -1, // Unlimited
		Consumer:      -1, // Unlimited
	}

	// Set expiry if needed, for now we use default (never expires or long lived)
	// claims.Expires = time.Now().AddDate(1, 0, 0).Unix()

	// 3. Sign with Operator Key
	opKp, err := nkeys.FromSeed([]byte(m.operatorSeed))
	if err != nil {
		return "", errorx.Wrap(err, "invalid operator seed")
	}

	accJWT, err := claims.Encode(opKp)
	if err != nil {
		return "", errorx.Wrap(err, "failed to sign account JWT")
	}

	// 4. Store Account Seed
	if err := m.keyStore.SaveSeed(ctx, tenantID, accSeed); err != nil {
		return "", errorx.Wrap(err, "failed to save account seed")
	}

	// 5. Publish to Resolver
	// Subject: $SYS.REQ.ACCOUNT.<AccountPubKey>
	subject := fmt.Sprintf("$SYS.REQ.ACCOUNT.%s", accPub)
	if err := m.nc.Publish(subject, []byte(accJWT)); err != nil {
		return "", errorx.Wrap(err, "failed to publish account JWT to resolver")
	}

	m.logger.Info("created nats account", "tenant_id", tenantID, "account_pub", accPub)

	return accJWT, nil
}

// CreateUser creates a new NATS User for a tenant, signed by the Tenant's Account key.
// Returns the JWT and the full Creds file content.
func (m *AccountManager) CreateUser(ctx context.Context, tenantID, userID string) (string, []byte, error) {
	// 1. Get Account Seed
	accSeed, err := m.keyStore.GetSeed(ctx, tenantID)
	if err != nil {
		return "", nil, errorx.Wrapf(err, "failed to get account seed for tenant %s", tenantID)
	}

	accKp, err := nkeys.FromSeed(accSeed)
	if err != nil {
		return "", nil, errorx.Wrap(err, "invalid account seed")
	}

	accPub, err := accKp.PublicKey()
	if err != nil {
		return "", nil, errorx.Wrap(err, "failed to get account public key")
	}

	// 2. Create User Keypair
	userKp, err := nkeys.CreateUser()
	if err != nil {
		return "", nil, errorx.Wrap(err, "failed to create user keypair")
	}

	userPub, err := userKp.PublicKey()
	if err != nil {
		return "", nil, errorx.Wrap(err, "failed to get user public key")
	}

	userSeed, err := userKp.Seed()
	if err != nil {
		return "", nil, errorx.Wrap(err, "failed to get user seed")
	}

	// 3. Create User Claims
	claims := jwt.NewUserClaims(userPub)
	claims.Name = userID
	claims.IssuerAccount = accPub

	// Permissions
	// Separate subjects for Events (EventStore) and Commands (CQRS)
	// Events: events.{tenant}.>
	// Commands: commands.{tenant}.>

	pubAllow := []string{
		fmt.Sprintf("events.%s.>", tenantID),
		fmt.Sprintf("commands.%s.>", tenantID),
		"_INBOX.>",
		// Allow JetStream API for own stream
		fmt.Sprintf("$JS.API.CONSUMER.*.EVENTS_%s.>", tenantID),
		fmt.Sprintf("$JS.ACK.EVENTS_%s.>", tenantID),
		fmt.Sprintf("$JS.API.STREAM.INFO.EVENTS_%s", tenantID),
	}

	subAllow := []string{
		fmt.Sprintf("events.%s.>", tenantID),
		fmt.Sprintf("commands.%s.>", tenantID),
		"_INBOX.>",
	}

	claims.Pub.Allow = pubAllow
	claims.Sub.Allow = subAllow

	// 4. Sign with Account Key
	userJWT, err := claims.Encode(accKp)
	if err != nil {
		return "", nil, errorx.Wrap(err, "failed to sign user JWT")
	}

	// 5. Generate Creds
	creds, err := jwt.FormatUserConfig(userJWT, userSeed)
	if err != nil {
		return "", nil, errorx.Wrap(err, "failed to format user creds")
	}

	m.logger.Info("created nats user", "tenant_id", tenantID, "user_id", userID, "user_pub", userPub)

	return userJWT, creds, nil
}
