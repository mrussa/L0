package repo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (r *OrdersRepo) upsertOrderBatch(ctx context.Context, o Order) (err error) {
	if o.OrderUID == "" || len(o.OrderUID) > maxUIDLen {
		return ErrBadUID
	}
	if o.Payment.Amount < 0 {
		return fmt.Errorf("%w: negative amount", ErrInconsistent)
	}
	o.DateCreated = o.DateCreated.UTC()

	ctxT, cancel := r.withTx(ctx)
	defer cancel()

	tx, err := r.Pool.BeginTx(ctxT, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctxT)
			panic(p)
		}
	}()

	var b pgx.Batch
	b.Queue(qUpsertOrder,
		o.OrderUID, o.TrackNumber, o.Entry, o.Locale, o.InternalSignature, o.CustomerID,
		o.DeliveryService, o.ShardKey, o.SMID, o.DateCreated, o.OofShard,
	)
	b.Queue(qUpsertPayment,
		o.OrderUID, o.Payment.TransactionID, o.Payment.RequestID, o.Payment.Currency,
		o.Payment.Provider, o.Payment.Amount, o.Payment.PaymentDT, o.Payment.Bank,
		o.Payment.DeliveryCost, o.Payment.GoodsTotal, o.Payment.CustomFee,
	)
	b.Queue(qUpsertDelivery,
		o.OrderUID, o.Delivery.Name, o.Delivery.Phone, o.Delivery.Zip, o.Delivery.City,
		o.Delivery.Address, o.Delivery.Region, o.Delivery.Email,
	)
	b.Queue(qDeleteItems, o.OrderUID)
	for _, it := range o.Items {
		b.Queue(qInsertItem,
			o.OrderUID, it.ChrtID, it.TrackNumber, it.Price, it.RID, it.Name,
			it.Sale, it.Size, it.TotalPrice, it.NmID, it.Brand, it.Status,
		)
	}

	br := tx.SendBatch(ctxT, &b)

	steps := 4 + len(o.Items)
	for i := 0; i < steps; i++ {
		if _, execErr := br.Exec(); execErr != nil {
			_ = br.Close()
			_ = tx.Rollback(ctxT)
			return fmt.Errorf("batch step %d: %w", i, execErr)
		}
	}

	if errClose := br.Close(); errClose != nil {
		_ = tx.Rollback(ctxT)
		return fmt.Errorf("batch close: %w", errClose)
	}

	if cErr := tx.Commit(ctxT); cErr != nil {
		return fmt.Errorf("commit: %w", cErr)
	}

	return nil
}
