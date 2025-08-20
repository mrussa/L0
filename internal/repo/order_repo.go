package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OrdersRepo struct {
	Pool *pgxpool.Pool
}

func NewOrdersRepo(pool *pgxpool.Pool) *OrdersRepo {
	return &OrdersRepo{Pool: pool}
}

var (
	ErrNotFound     = errors.New("order not found")
	ErrBadUID       = errors.New("bad order_uid")
	ErrInconsistent = errors.New("inconsistent data")
)

func with2s(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 2*time.Second)
}

const (
	qOrder = `SELECT order_uid, track_number, entry, locale, internal_signature, customer_id,
                     delivery_service, shardkey, sm_id, date_created, oof_shard
              FROM orders WHERE order_uid = $1`
	qDelivery = `SELECT name, phone, zip, city, address, region, email
                 FROM order_delivery WHERE order_uid = $1`
	qPayment = `SELECT transaction_id, request_id, currency, provider, amount, payment_dt,
                       bank, delivery_cost, goods_total, custom_fee
                FROM order_payment WHERE order_uid = $1`
	qItems = `SELECT id, chrt_id, track_number, price, rid, name, sale, size,
                     total_price, nm_id, brand, status
              FROM order_items WHERE order_uid = $1 ORDER BY id`
)

const (
	maxUIDLen       = 100
	defaultItemsCap = 8
)

func (r *OrdersRepo) getOrderHeader(ctx context.Context, uid string) (Order, error) {
	var o Order
	ctxT, cancel := with2s(ctx)
	defer cancel()

	err := r.Pool.QueryRow(ctxT, qOrder, uid).Scan(
		&o.OrderUID, &o.TrackNumber, &o.Entry, &o.Locale, &o.InternalSignature,
		&o.CustomerID, &o.DeliveryService, &o.ShardKey, &o.SMID, &o.DateCreated, &o.OofShard,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, err
	}
	return o, nil
}

func (r *OrdersRepo) getDelivery(ctx context.Context, uid string) (Delivery, error) {
	var d Delivery
	ctxT, cancel := with2s(ctx)
	defer cancel()

	err := r.Pool.QueryRow(ctxT, qDelivery, uid).Scan(
		&d.Name, &d.Phone, &d.Zip, &d.City, &d.Address, &d.Region, &d.Email,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, fmt.Errorf("%w: no delivery for order", ErrInconsistent)
	}
	return d, err
}

func (r *OrdersRepo) getPayment(ctx context.Context, uid string) (Payment, error) {
	var p Payment
	ctxT, cancel := with2s(ctx)
	defer cancel()

	err := r.Pool.QueryRow(ctxT, qPayment, uid).Scan(
		&p.TransactionID, &p.RequestID, &p.Currency, &p.Provider, &p.Amount,
		&p.PaymentDT, &p.Bank, &p.DeliveryCost, &p.GoodsTotal, &p.CustomFee,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, fmt.Errorf("%w: no payment for order", ErrInconsistent)
	}
	return p, err
}

func (r *OrdersRepo) getItems(ctx context.Context, uid string) ([]Item, error) {
	ctxT, cancel := with2s(ctx)
	defer cancel()

	rows, err := r.Pool.Query(ctxT, qItems, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Item, 0, defaultItemsCap)

	for rows.Next() {
		var it Item
		var tmpID int64
		if err := rows.Scan(
			&tmpID, &it.ChrtID, &it.TrackNumber, &it.Price, &it.RID, &it.Name,
			&it.Sale, &it.Size, &it.TotalPrice, &it.NmID, &it.Brand, &it.Status,
		); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
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
