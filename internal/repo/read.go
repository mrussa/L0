package repo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (r *OrdersRepo) getOrderHeader(ctx context.Context, uid string) (Order, error) {
	var o Order
	ctxT, cancel := r.withQ(ctx)
	defer cancel()

	err := r.Pool.QueryRow(ctxT, qOrder, uid).Scan(
		&o.OrderUID, &o.TrackNumber, &o.Entry, &o.Locale, &o.InternalSignature,
		&o.CustomerID, &o.DeliveryService, &o.ShardKey, &o.SMID, &o.DateCreated, &o.OofShard,
	)
	if errorsIsNoRows(err) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("getOrderHeader: %w", err)
	}
	return o, nil
}

func (r *OrdersRepo) getDelivery(ctx context.Context, uid string) (Delivery, error) {
	var d Delivery
	ctxT, cancel := r.withQ(ctx)
	defer cancel()

	err := r.Pool.QueryRow(ctxT, qDelivery, uid).Scan(
		&d.Name, &d.Phone, &d.Zip, &d.City, &d.Address, &d.Region, &d.Email,
	)
	if errorsIsNoRows(err) {
		return Delivery{}, fmt.Errorf("%w: delivery missing", ErrInconsistent)
	}
	if err != nil {
		return Delivery{}, fmt.Errorf("getDelivery: %w", err)
	}
	return d, nil
}

func (r *OrdersRepo) getPayment(ctx context.Context, uid string) (Payment, error) {
	var p Payment
	ctxT, cancel := r.withQ(ctx)
	defer cancel()

	err := r.Pool.QueryRow(ctxT, qPayment, uid).Scan(
		&p.TransactionID, &p.RequestID, &p.Currency, &p.Provider, &p.Amount,
		&p.PaymentDT, &p.Bank, &p.DeliveryCost, &p.GoodsTotal, &p.CustomFee,
	)
	if errorsIsNoRows(err) {
		return Payment{}, fmt.Errorf("%w: payment missing", ErrInconsistent)
	}
	if err != nil {
		return Payment{}, fmt.Errorf("getPayment: %w", err)
	}
	return p, nil
}

func (r *OrdersRepo) getItems(ctx context.Context, uid string) ([]Item, error) {
	ctxT, cancel := r.withQ(ctx)
	defer cancel()

	rows, err := r.Pool.Query(ctxT, qItems, uid)
	if err != nil {
		return nil, fmt.Errorf("getItems query: %w", err)
	}
	defer rows.Close()

	items := make([]Item, 0, defaultItemsCap)
	for rows.Next() {
		var it Item
		var id int64
		if err := rows.Scan(
			&id, &it.ChrtID, &it.TrackNumber, &it.Price, &it.RID, &it.Name,
			&it.Sale, &it.Size, &it.TotalPrice, &it.NmID, &it.Brand, &it.Status,
		); err != nil {
			return nil, fmt.Errorf("getItems scan: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("getItems rows: %w", err)
	}
	return items, nil
}

func errorsIsNoRows(err error) bool { return err == pgx.ErrNoRows }
