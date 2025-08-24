package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
}

type OrdersRepo struct {
	Pool      DB
	qTimeout  time.Duration
	txTimeout time.Duration
}

func NewOrdersRepo(pool *pgxpool.Pool) *OrdersRepo {
	return &OrdersRepo{
		Pool:      pool,
		qTimeout:  2 * time.Second,
		txTimeout: 5 * time.Second,
	}
}

func NewOrdersRepoWith(pool *pgxpool.Pool, qTimeout, txTimeout time.Duration) *OrdersRepo {
	return &OrdersRepo{
		Pool:      pool,
		qTimeout:  qTimeout,
		txTimeout: txTimeout,
	}
}

func (r *OrdersRepo) withQ(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, r.qTimeout)
}
func (r *OrdersRepo) withTx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, r.txTimeout)
}

func (r *OrdersRepo) GetOrder(ctx context.Context, uid string) (Order, error) {
	if uid == "" || len(uid) > maxUIDLen {
		return Order{}, ErrBadUID
	}

	o, err := r.getOrderHeader(ctx, uid)
	if err != nil {
		return Order{}, err
	}
	if o.Payment, err = r.getPayment(ctx, uid); err != nil {
		return Order{}, err
	}
	if o.Delivery, err = r.getDelivery(ctx, uid); err != nil {
		return Order{}, err
	}
	if o.Items, err = r.getItems(ctx, uid); err != nil {
		return Order{}, err
	}
	return o, nil
}

func (r *OrdersRepo) ListRecentOrderUIDs(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		return []string{}, nil
	}
	ctxT, cancel := r.withQ(ctx)
	defer cancel()

	rows, err := r.Pool.Query(ctxT, `
        SELECT order_uid
        FROM orders
        ORDER BY date_created DESC
        LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("listRecent query: %w", err)
	}
	defer rows.Close()

	uids := make([]string, 0, limit)
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("listRecent scan: %w", err)
		}
		uids = append(uids, uid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listRecent rows: %w", err)
	}
	return uids, nil
}

func (r *OrdersRepo) UpsertOrder(ctx context.Context, o Order) (err error) {
	return r.upsertOrderBatch(ctx, o)
}

func (r *OrdersRepo) Ping(ctx context.Context) error {
	ctxT, cancel := r.withQ(ctx)
	defer cancel()
	var x int
	if err := r.Pool.QueryRow(ctxT, "select 1").Scan(&x); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	return nil
}
