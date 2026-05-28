package repository

import (
	"context"
	"database/sql"

	"github.com/fivecode/plotty/core/constants"
	"github.com/fivecode/plotty/core/models"
	"github.com/fivecode/plotty/core/named_errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetBalance(ctx context.Context, userID uint64) (int, error) {
	var balance int
	err := r.pool.QueryRow(ctx, `SELECT ai_credits FROM users WHERE id = $1`, userID).Scan(&balance)
	return balance, err
}

func (r *Repository) DeductCredits(ctx context.Context, userID uint64, amount int, jobType string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`UPDATE users SET ai_credits = ai_credits - $2 WHERE id = $1 AND ai_credits >= $2`,
		userID, amount,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return named_errors.ErrInsufficientCredits
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO credit_transactions (id, user_id, amount, type, description, status)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		uuid.New(), userID, -amount, constants.CreditTxTypeUsage, "AI: "+jobType, constants.CreditTxStatusCompleted,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) RefundCredits(ctx context.Context, userID uint64, amount int, jobType string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`UPDATE users SET ai_credits = ai_credits + $2 WHERE id = $1`,
		userID, amount,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO credit_transactions (id, user_id, amount, type, description, status)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		uuid.New(), userID, amount, "refund", "Возврат AI: "+jobType, constants.CreditTxStatusCompleted,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) AddCredits(ctx context.Context, userID uint64, amount int, paymentLabel string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`INSERT INTO credit_transactions (id, user_id, amount, type, payment_label, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (payment_label) DO NOTHING`,
		uuid.New(), userID, amount, constants.CreditTxTypePurchase, paymentLabel, constants.CreditTxStatusCompleted,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}

	_, err = tx.Exec(ctx,
		`UPDATE users SET ai_credits = ai_credits + $2 WHERE id = $1`,
		userID, amount,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) GetTransactions(ctx context.Context, userID uint64) ([]models.CreditTransaction, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, amount, type, description, payment_label, status, created_at
		 FROM credit_transactions WHERE user_id = $1
		 ORDER BY created_at DESC LIMIT 50`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []models.CreditTransaction
	for rows.Next() {
		var t models.CreditTransaction
		var desc, label sql.NullString
		if err := rows.Scan(&t.ID, &t.UserID, &t.Amount, &t.Type, &desc, &label, &t.Status, &t.CreatedAt); err != nil {
			return nil, err
		}
		if desc.Valid {
			t.Description = &desc.String
		}
		if label.Valid {
			t.PaymentLabel = &label.String
		}
		txs = append(txs, t)
	}
	return txs, rows.Err()
}
