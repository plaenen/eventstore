package projections

import (
	"context"
	"database/sql"

	accountv1 "github.com/plaenen/eventstore/examples/pb/account/v1"
	"github.com/plaenen/eventstore/pkg/eventsourcing"
	"google.golang.org/protobuf/proto"
)

// AccountViewProjection maintains a denormalized view of account data
type AccountViewProjection struct {
	db *sql.DB
}

// NewAccountViewProjection creates a new account view projection
func NewAccountViewProjection(db *sql.DB) *AccountViewProjection {
	return &AccountViewProjection{db: db}
}

// Name returns the projection name
func (p *AccountViewProjection) Name() string {
	return "account_view"
}

// Handle processes events and updates the projection
func (p *AccountViewProjection) Handle(ctx context.Context, envelope *eventsourcing.EventEnvelope) error {
	switch envelope.EventType {
	case "accountv1.AccountOpenedEvent":
		var evt accountv1.AccountOpenedEvent
		if err := proto.Unmarshal(envelope.Data, &evt); err != nil {
			return err
		}
		return p.handleAccountOpened(&evt)

	case "accountv1.MoneyDepositedEvent":
		var evt accountv1.MoneyDepositedEvent
		if err := proto.Unmarshal(envelope.Data, &evt); err != nil {
			return err
		}
		return p.handleMoneyDeposited(&evt)

	case "accountv1.MoneyWithdrawnEvent":
		var evt accountv1.MoneyWithdrawnEvent
		if err := proto.Unmarshal(envelope.Data, &evt); err != nil {
			return err
		}
		return p.handleMoneyWithdrawn(&evt)

	case "accountv1.AccountClosedEvent":
		var evt accountv1.AccountClosedEvent
		if err := proto.Unmarshal(envelope.Data, &evt); err != nil {
			return err
		}
		return p.handleAccountClosed(&evt)
	}

	return nil
}

// Reset clears all projection data
func (p *AccountViewProjection) Reset(ctx context.Context) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM account_view`)
	return err
}

func (p *AccountViewProjection) handleAccountOpened(evt *accountv1.AccountOpenedEvent) error {
	_, err := p.db.Exec(`
		INSERT INTO account_view (account_id, owner_name, balance, status)
		VALUES (?, ?, ?, ?)
	`, evt.AccountId, evt.OwnerName, evt.InitialBalance, "OPEN")
	return err
}

func (p *AccountViewProjection) handleMoneyDeposited(evt *accountv1.MoneyDepositedEvent) error {
	_, err := p.db.Exec(`
		UPDATE account_view SET balance = ? WHERE account_id = ?
	`, evt.NewBalance, evt.AccountId)
	return err
}

func (p *AccountViewProjection) handleMoneyWithdrawn(evt *accountv1.MoneyWithdrawnEvent) error {
	_, err := p.db.Exec(`
		UPDATE account_view SET balance = ? WHERE account_id = ?
	`, evt.NewBalance, evt.AccountId)
	return err
}

func (p *AccountViewProjection) handleAccountClosed(evt *accountv1.AccountClosedEvent) error {
	_, err := p.db.Exec(`
		UPDATE account_view SET status = ? WHERE account_id = ?
	`, "CLOSED", evt.AccountId)
	return err
}
