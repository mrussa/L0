package repo

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func tNow() time.Time {
	return time.Date(2021, 11, 26, 6, 22, 19, 0, time.UTC)
}

func sampleOrder() Order {
	return Order{
		OrderUID:          "uid-1",
		TrackNumber:       "TRK",
		Entry:             "WBIL",
		Locale:            "en",
		InternalSignature: "",
		CustomerID:        "cust",
		DeliveryService:   "meest",
		ShardKey:          "9",
		SMID:              99,
		DateCreated:       tNow(),
		OofShard:          "1",
		Delivery: Delivery{
			Name:    "Name",
			Phone:   "+100000",
			Zip:     "000000",
			City:    "City",
			Address: "Addr",
			Region:  "Region",
			Email:   "n@example.com",
		},
		Payment: Payment{
			TransactionID: "tx-1",
			RequestID:     "",
			Currency:      "USD",
			Provider:      "wbpay",
			Amount:        1817,
			PaymentDT:     1637907727,
			Bank:          "alpha",
			DeliveryCost:  1500,
			GoodsTotal:    317,
			CustomFee:     0,
		},
		Items: []Item{
			{
				ChrtID:      1,
				TrackNumber: "TRK",
				Price:       317,
				RID:         "rid1",
				Name:        "Item1",
				Sale:        0,
				Size:        "0",
				TotalPrice:  317,
				NmID:        100,
				Brand:       "Brand",
				Status:      200,
			},
			{
				ChrtID:      2,
				TrackNumber: "TRK",
				Price:       100,
				RID:         "rid2",
				Name:        "Item2",
				Sale:        0,
				Size:        "0",
				TotalPrice:  100,
				NmID:        200,
				Brand:       "Brand",
				Status:      200,
			},
		},
	}
}

func Test_NewOrdersRepo_DefaultTimeouts_Work(t *testing.T) {
	r := NewOrdersRepo(nil)

	ctxQ, cancelQ := r.withQ(context.Background())
	defer cancelQ()
	dlQ, okQ := ctxQ.Deadline()
	require.True(t, okQ)
	require.WithinDuration(t, time.Now().Add(2*time.Second), dlQ, 200*time.Millisecond)

	ctxT, cancelT := r.withTx(context.Background())
	defer cancelT()
	dlT, okT := ctxT.Deadline()
	require.True(t, okT)
	require.WithinDuration(t, time.Now().Add(5*time.Second), dlT, 200*time.Millisecond)
}

func Test_NewOrdersRepoWith_CustomTimeouts(t *testing.T) {
	r := NewOrdersRepoWith(nil, 1500*time.Millisecond, 3*time.Second)

	ctxQ, cancelQ := r.withQ(context.Background())
	defer cancelQ()
	dlQ, _ := ctxQ.Deadline()
	require.WithinDuration(t, time.Now().Add(1500*time.Millisecond), dlQ, 200*time.Millisecond)

	ctxT, cancelT := r.withTx(context.Background())
	defer cancelT()
	dlT, _ := ctxT.Deadline()
	require.WithinDuration(t, time.Now().Add(3*time.Second), dlT, 200*time.Millisecond)
}

func Test_errorsIsNoRows(t *testing.T) {
	require.True(t, errorsIsNoRows(pgx.ErrNoRows))
	require.False(t, errorsIsNoRows(errors.New("x")))
}

func Test_getOrderHeader_Success(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()

	uid := "uid-1"
	rows := pgxmock.NewRows([]string{
		"order_uid", "track_number", "entry", "locale", "internal_signature", "customer_id",
		"delivery_service", "shardkey", "sm_id", "date_created", "oof_shard",
	}).AddRow(uid, "TRK", "WBIL", "en", "", "cust", "meest", "9", int32(99), tNow(), "1")

	mock.ExpectQuery(regexp.QuoteMeta(qOrder)).WithArgs(uid).WillReturnRows(rows)

	r := &OrdersRepo{Pool: mock, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	got, err := r.getOrderHeader(context.Background(), uid)
	require.NoError(t, err)
	require.Equal(t, uid, got.OrderUID)
	require.Equal(t, int32(99), got.SMID)
	require.Equal(t, tNow(), got.DateCreated)
	require.NoError(t, mock.ExpectationsWereMet())
}

func Test_getOrderHeader_NotFound_And_OtherErrorWrapped(t *testing.T) {
	m1, _ := pgxmock.NewPool()
	defer m1.Close()
	m1.ExpectQuery(regexp.QuoteMeta(qOrder)).WithArgs("nope").WillReturnError(pgx.ErrNoRows)
	r1 := &OrdersRepo{Pool: m1, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err := r1.getOrderHeader(context.Background(), "nope")
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, m1.ExpectationsWereMet())

	m2, _ := pgxmock.NewPool()
	defer m2.Close()
	m2.ExpectQuery(regexp.QuoteMeta(qOrder)).WithArgs("bad").WillReturnError(errors.New("boom"))
	r2 := &OrdersRepo{Pool: m2, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err = r2.getOrderHeader(context.Background(), "bad")
	require.Error(t, err)
	require.ErrorContains(t, err, "getOrderHeader")
	require.ErrorContains(t, err, "boom")
	require.NoError(t, m2.ExpectationsWereMet())
}

func Test_getDelivery_Success_NoRows_OtherWrapped(t *testing.T) {
	uid := "uid-1"

	m1, _ := pgxmock.NewPool()
	defer m1.Close()
	rows := pgxmock.NewRows([]string{
		"name", "phone", "zip", "city", "address", "region", "email",
	}).AddRow("Name", "+1", "0", "City", "Addr", "Region", "e@mail")
	m1.ExpectQuery(regexp.QuoteMeta(qDelivery)).WithArgs(uid).WillReturnRows(rows)
	r1 := &OrdersRepo{Pool: m1, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	d, err := r1.getDelivery(context.Background(), uid)
	require.NoError(t, err)
	require.Equal(t, "Name", d.Name)
	require.NoError(t, m1.ExpectationsWereMet())

	m2, _ := pgxmock.NewPool()
	defer m2.Close()
	m2.ExpectQuery(regexp.QuoteMeta(qDelivery)).WithArgs(uid).WillReturnError(pgx.ErrNoRows)
	r2 := &OrdersRepo{Pool: m2, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err = r2.getDelivery(context.Background(), uid)
	require.ErrorIs(t, err, ErrInconsistent)
	require.ErrorContains(t, err, "delivery missing")
	require.NoError(t, m2.ExpectationsWereMet())

	m3, _ := pgxmock.NewPool()
	defer m3.Close()
	m3.ExpectQuery(regexp.QuoteMeta(qDelivery)).WithArgs(uid).WillReturnError(errors.New("boom"))
	r3 := &OrdersRepo{Pool: m3, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err = r3.getDelivery(context.Background(), uid)
	require.ErrorContains(t, err, "getDelivery")
	require.ErrorContains(t, err, "boom")
	require.NoError(t, m3.ExpectationsWereMet())
}

func Test_getPayment_Success_NoRows_OtherWrapped(t *testing.T) {
	uid := "uid-1"

	m1, _ := pgxmock.NewPool()
	defer m1.Close()
	rows := pgxmock.NewRows([]string{
		"transaction_id", "request_id", "currency", "provider", "amount", "payment_dt",
		"bank", "delivery_cost", "goods_total", "custom_fee",
	}).AddRow("tx", "", "USD", "wbpay", int32(10), int64(123), "alpha", int32(1), int32(2), int32(0))
	m1.ExpectQuery(regexp.QuoteMeta(qPayment)).WithArgs(uid).WillReturnRows(rows)
	r1 := &OrdersRepo{Pool: m1, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	p, err := r1.getPayment(context.Background(), uid)
	require.NoError(t, err)
	require.Equal(t, "tx", p.TransactionID)
	require.NoError(t, m1.ExpectationsWereMet())

	m2, _ := pgxmock.NewPool()
	defer m2.Close()
	m2.ExpectQuery(regexp.QuoteMeta(qPayment)).WithArgs(uid).WillReturnError(pgx.ErrNoRows)
	r2 := &OrdersRepo{Pool: m2, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err = r2.getPayment(context.Background(), uid)
	require.ErrorIs(t, err, ErrInconsistent)
	require.ErrorContains(t, err, "payment missing")
	require.NoError(t, m2.ExpectationsWereMet())

	m3, _ := pgxmock.NewPool()
	defer m3.Close()
	m3.ExpectQuery(regexp.QuoteMeta(qPayment)).WithArgs(uid).WillReturnError(errors.New("boom"))
	r3 := &OrdersRepo{Pool: m3, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err = r3.getPayment(context.Background(), uid)
	require.ErrorContains(t, err, "getPayment")
	require.ErrorContains(t, err, "boom")
	require.NoError(t, m3.ExpectationsWereMet())
}

func Test_getItems_Success_ScanErr_QueryErr_RowsErr(t *testing.T) {
	uid := "uid-1"

	m1, _ := pgxmock.NewPool()
	defer m1.Close()
	rows := pgxmock.NewRows([]string{
		"id", "chrt_id", "track_number", "price", "rid", "name", "sale", "size",
		"total_price", "nm_id", "brand", "status",
	}).AddRow(int64(1), int64(11), "TRK", int32(100), "r1", "N1", int32(0), "0", int32(100), int64(500), "B", int32(200)).
		AddRow(int64(2), int64(22), "TRK", int32(200), "r2", "N2", int32(0), "0", int32(200), int64(600), "B", int32(200))
	m1.ExpectQuery(regexp.QuoteMeta(qItems)).WithArgs(uid).WillReturnRows(rows)
	r1 := &OrdersRepo{Pool: m1, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	items, err := r1.getItems(context.Background(), uid)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, int32(100), items[0].Price)
	require.Equal(t, int32(200), items[1].Price)
	require.NoError(t, m1.ExpectationsWereMet())

	m2, _ := pgxmock.NewPool()
	defer m2.Close()
	badRows := pgxmock.NewRows([]string{"id"}).AddRow(int64(1))
	m2.ExpectQuery(regexp.QuoteMeta(qItems)).WithArgs(uid).WillReturnRows(badRows)
	r2 := &OrdersRepo{Pool: m2, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err = r2.getItems(context.Background(), uid)
	require.ErrorContains(t, err, "getItems scan:")
	require.NoError(t, m2.ExpectationsWereMet())

	m3, _ := pgxmock.NewPool()
	defer m3.Close()
	m3.ExpectQuery(regexp.QuoteMeta(qItems)).WithArgs(uid).WillReturnError(errors.New("qerr"))
	r3 := &OrdersRepo{Pool: m3, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err = r3.getItems(context.Background(), uid)
	require.ErrorContains(t, err, "getItems query")
	require.ErrorContains(t, err, "qerr")
	require.NoError(t, m3.ExpectationsWereMet())

	m4, _ := pgxmock.NewPool()
	defer m4.Close()
	rows4 := pgxmock.NewRows([]string{
		"id", "chrt_id", "track_number", "price", "rid", "name", "sale", "size",
		"total_price", "nm_id", "brand", "status",
	}).AddRow(int64(1), int64(11), "TRK", int32(100), "r1", "N1", int32(0), "0", int32(100), int64(500), "B", int32(200))
	rows4.RowError(1, errors.New("rows-err"))
	m4.ExpectQuery(regexp.QuoteMeta(qItems)).WithArgs(uid).WillReturnRows(rows4)
	r4 := &OrdersRepo{Pool: m4, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err = r4.getItems(context.Background(), uid)
	require.ErrorContains(t, err, "getItems rows")
	require.ErrorContains(t, err, "rows-err")
	require.NoError(t, m4.ExpectationsWereMet())
}

func Test_ListRecentOrderUIDs_AllBranches(t *testing.T) {
	r0 := &OrdersRepo{}
	out, err := r0.ListRecentOrderUIDs(context.Background(), 0)
	require.NoError(t, err)
	require.Empty(t, out)

	m1, _ := pgxmock.NewPool()
	defer m1.Close()
	rows := pgxmock.NewRows([]string{"order_uid"}).AddRow("u2").AddRow("u1")
	m1.ExpectQuery(`SELECT\s+order_uid`).WithArgs(2).WillReturnRows(rows)
	r1 := &OrdersRepo{Pool: m1, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	uids, err := r1.ListRecentOrderUIDs(context.Background(), 2)
	require.NoError(t, err)
	require.Equal(t, []string{"u2", "u1"}, uids)
	require.NoError(t, m1.ExpectationsWereMet())

	m2, _ := pgxmock.NewPool()
	defer m2.Close()
	m2.ExpectQuery(`SELECT\s+order_uid`).WithArgs(1).WillReturnError(errors.New("boom"))
	r2 := &OrdersRepo{Pool: m2, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err = r2.ListRecentOrderUIDs(context.Background(), 1)
	require.ErrorContains(t, err, "listRecent query")
	require.ErrorContains(t, err, "boom")
	require.NoError(t, m2.ExpectationsWereMet())

	m3, _ := pgxmock.NewPool()
	defer m3.Close()
	rows3 := pgxmock.NewRows([]string{"order_uid"})
	rows3.RowError(0, errors.New("scan-fail"))
	m3.ExpectQuery(`SELECT\s+order_uid`).WithArgs(1).WillReturnRows(rows3)
	r3 := &OrdersRepo{Pool: m3, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err = r3.ListRecentOrderUIDs(context.Background(), 1)
	require.ErrorContains(t, err, "listRecent rows")
	require.ErrorContains(t, err, "scan-fail")
	require.NoError(t, m3.ExpectationsWereMet())
}

func Test_GetOrder_AllBranches(t *testing.T) {
	r := &OrdersRepo{}
	_, err := r.GetOrder(context.Background(), "")
	require.ErrorIs(t, err, ErrBadUID)

	long := make([]byte, maxUIDLen+1)
	for i := range long {
		long[i] = 'a'
	}
	_, err = r.GetOrder(context.Background(), string(long))
	require.ErrorIs(t, err, ErrBadUID)

	uid := "uid-1"

	m1, _ := pgxmock.NewPool()
	defer m1.Close()
	m1.ExpectQuery(regexp.QuoteMeta(qOrder)).WithArgs(uid).WillReturnError(pgx.ErrNoRows)
	r1 := &OrdersRepo{Pool: m1, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err = r1.GetOrder(context.Background(), uid)
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, m1.ExpectationsWereMet())

	m2, _ := pgxmock.NewPool()
	defer m2.Close()
	m2.ExpectQuery(regexp.QuoteMeta(qOrder)).WithArgs(uid).WillReturnError(errors.New("hdr-err"))
	r2 := &OrdersRepo{Pool: m2, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err = r2.GetOrder(context.Background(), uid)
	require.ErrorContains(t, err, "getOrderHeader")
	require.ErrorContains(t, err, "hdr-err")
	require.NoError(t, m2.ExpectationsWereMet())

	m3, _ := pgxmock.NewPool()
	defer m3.Close()
	hRows3 := pgxmock.NewRows([]string{
		"order_uid", "track_number", "entry", "locale", "internal_signature", "customer_id",
		"delivery_service", "shardkey", "sm_id", "date_created", "oof_shard",
	}).AddRow(uid, "TRK", "WBIL", "en", "", "cust", "meest", "9", int32(99), tNow(), "1")
	m3.ExpectQuery(regexp.QuoteMeta(qOrder)).WithArgs(uid).WillReturnRows(hRows3)
	m3.ExpectQuery(regexp.QuoteMeta(qPayment)).WithArgs(uid).WillReturnError(pgx.ErrNoRows)
	r3 := &OrdersRepo{Pool: m3, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err = r3.GetOrder(context.Background(), uid)
	require.ErrorIs(t, err, ErrInconsistent)
	require.NoError(t, m3.ExpectationsWereMet())

	m4, _ := pgxmock.NewPool()
	defer m4.Close()
	hRows4 := pgxmock.NewRows([]string{
		"order_uid", "track_number", "entry", "locale", "internal_signature", "customer_id",
		"delivery_service", "shardkey", "sm_id", "date_created", "oof_shard",
	}).AddRow(uid, "TRK", "WBIL", "en", "", "cust", "meest", "9", int32(99), tNow(), "1")
	m4.ExpectQuery(regexp.QuoteMeta(qOrder)).WithArgs(uid).WillReturnRows(hRows4)
	pRows4 := pgxmock.NewRows([]string{
		"transaction_id", "request_id", "currency", "provider", "amount", "payment_dt",
		"bank", "delivery_cost", "goods_total", "custom_fee",
	}).AddRow("tx", "", "USD", "wbpay", int32(10), int64(123), "alpha", int32(1), int32(2), int32(0))
	m4.ExpectQuery(regexp.QuoteMeta(qPayment)).WithArgs(uid).WillReturnRows(pRows4)
	m4.ExpectQuery(regexp.QuoteMeta(qDelivery)).WithArgs(uid).WillReturnError(pgx.ErrNoRows)
	r4 := &OrdersRepo{Pool: m4, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err = r4.GetOrder(context.Background(), uid)
	require.ErrorIs(t, err, ErrInconsistent)
	require.NoError(t, m4.ExpectationsWereMet())

	m5, _ := pgxmock.NewPool()
	defer m5.Close()
	hRows5 := pgxmock.NewRows([]string{
		"order_uid", "track_number", "entry", "locale", "internal_signature", "customer_id",
		"delivery_service", "shardkey", "sm_id", "date_created", "oof_shard",
	}).AddRow(uid, "TRK", "WBIL", "en", "", "cust", "meest", "9", int32(99), tNow(), "1")
	m5.ExpectQuery(regexp.QuoteMeta(qOrder)).WithArgs(uid).WillReturnRows(hRows5)
	pRows5 := pgxmock.NewRows([]string{
		"transaction_id", "request_id", "currency", "provider", "amount", "payment_dt",
		"bank", "delivery_cost", "goods_total", "custom_fee",
	}).AddRow("tx", "", "USD", "wbpay", int32(10), int64(123), "alpha", int32(1), int32(2), int32(0))
	m5.ExpectQuery(regexp.QuoteMeta(qPayment)).WithArgs(uid).WillReturnRows(pRows5)
	dRows5 := pgxmock.NewRows([]string{
		"name", "phone", "zip", "city", "address", "region", "email",
	}).AddRow("Name", "+1", "0", "City", "Addr", "Region", "e@mail")
	m5.ExpectQuery(regexp.QuoteMeta(qDelivery)).WithArgs(uid).WillReturnRows(dRows5)
	m5.ExpectQuery(regexp.QuoteMeta(qItems)).WithArgs(uid).WillReturnError(errors.New("items-err"))
	r5 := &OrdersRepo{Pool: m5, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err = r5.GetOrder(context.Background(), uid)
	require.ErrorContains(t, err, "items-err")
	require.NoError(t, m5.ExpectationsWereMet())

	m6, _ := pgxmock.NewPool()
	defer m6.Close()
	hRows6 := pgxmock.NewRows([]string{
		"order_uid", "track_number", "entry", "locale", "internal_signature", "customer_id",
		"delivery_service", "shardkey", "sm_id", "date_created", "oof_shard",
	}).AddRow(uid, "TRK", "WBIL", "en", "", "cust", "meest", "9", int32(99), tNow(), "1")
	m6.ExpectQuery(regexp.QuoteMeta(qOrder)).WithArgs(uid).WillReturnRows(hRows6)
	pRows6 := pgxmock.NewRows([]string{
		"transaction_id", "request_id", "currency", "provider", "amount", "payment_dt",
		"bank", "delivery_cost", "goods_total", "custom_fee",
	}).AddRow("tx", "", "USD", "wbpay", int32(10), int64(123), "alpha", int32(1), int32(2), int32(0))
	m6.ExpectQuery(regexp.QuoteMeta(qPayment)).WithArgs(uid).WillReturnRows(pRows6)
	dRows6 := pgxmock.NewRows([]string{
		"name", "phone", "zip", "city", "address", "region", "email",
	}).AddRow("Name", "+1", "0", "City", "Addr", "Region", "e@mail")
	m6.ExpectQuery(regexp.QuoteMeta(qDelivery)).WithArgs(uid).WillReturnRows(dRows6)
	iRows6 := pgxmock.NewRows([]string{
		"id", "chrt_id", "track_number", "price", "rid", "name", "sale", "size",
		"total_price", "nm_id", "brand", "status",
	}).AddRow(int64(1), int64(11), "TRK", int32(100), "r1", "N1", int32(0), "0", int32(100), int64(500), "B", int32(200))
	m6.ExpectQuery(regexp.QuoteMeta(qItems)).WithArgs(uid).WillReturnRows(iRows6)
	r6 := &OrdersRepo{Pool: m6, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	o, err := r6.GetOrder(context.Background(), uid)
	require.NoError(t, err)
	require.Equal(t, uid, o.OrderUID)
	require.Len(t, o.Items, 1)
	require.NoError(t, m6.ExpectationsWereMet())
}

func Test_Ping_Success_And_Error(t *testing.T) {
	m1, _ := pgxmock.NewPool()
	defer m1.Close()
	m1.ExpectQuery(regexp.QuoteMeta("select 1")).WillReturnRows(pgxmock.NewRows([]string{"?column?"}).AddRow(1))
	r1 := &OrdersRepo{Pool: m1, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	require.NoError(t, r1.Ping(context.Background()))
	require.NoError(t, m1.ExpectationsWereMet())

	m2, _ := pgxmock.NewPool()
	defer m2.Close()
	m2.ExpectQuery(regexp.QuoteMeta("select 1")).WillReturnError(errors.New("nope"))
	r2 := &OrdersRepo{Pool: m2, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	err := r2.Ping(context.Background())
	require.ErrorContains(t, err, "ping")
	require.ErrorContains(t, err, "nope")
	require.NoError(t, m2.ExpectationsWereMet())
}

type fakeDBBatch struct {
	tx       *fakeTxBatch
	beginErr error
}

func (f *fakeDBBatch) QueryRow(context.Context, string, ...any) pgx.Row { panic("not used") }
func (f *fakeDBBatch) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("not used")
}
func (f *fakeDBBatch) BeginTx(ctx context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
	if f.beginErr != nil {
		return nil, f.beginErr
	}
	if f.tx == nil {
		f.tx = &fakeTxBatch{}
	}
	return f.tx, nil
}

type fakeTxBatch struct {
	br            pgx.BatchResults
	commitErr     error
	rolledBack    bool
	committed     bool
	panicOnCommit bool
}

func (t *fakeTxBatch) Begin(context.Context) (pgx.Tx, error) { return t, nil }
func (t *fakeTxBatch) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return &pgconn.StatementDescription{}, nil
}
func (t *fakeTxBatch) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	panic("not used")
}
func (t *fakeTxBatch) Query(context.Context, string, ...any) (pgx.Rows, error) { panic("not used") }
func (t *fakeTxBatch) QueryRow(context.Context, string, ...any) pgx.Row        { panic("not used") }
func (t *fakeTxBatch) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	if t.br == nil {
		t.br = &fakeBatchResults{}
	}
	return t.br
}
func (t *fakeTxBatch) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	panic("not used")
}
func (t *fakeTxBatch) LargeObjects() pgx.LargeObjects { panic("not used") }
func (t *fakeTxBatch) Conn() *pgx.Conn                { return nil }
func (t *fakeTxBatch) Commit(context.Context) error {
	t.committed = true
	if t.panicOnCommit {
		panic("panic-commit")
	}
	return t.commitErr
}
func (t *fakeTxBatch) Rollback(context.Context) error { t.rolledBack = true; return nil }

type fakeBatchResults struct {
	calls    int
	failAt   int
	closeErr error
}

func (b *fakeBatchResults) Exec() (pgconn.CommandTag, error) {
	b.calls++
	if b.failAt != 0 && b.calls == b.failAt {
		return pgconn.NewCommandTag(""), errors.New("step-fail")
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (b *fakeBatchResults) Query() (pgx.Rows, error) { return nil, errors.New("not used") }
func (b *fakeBatchResults) QueryRow() pgx.Row        { return nil }
func (b *fakeBatchResults) Close() error             { return b.closeErr }

func Test_UpsertOrder_Batch_Success_CommitOK(t *testing.T) {
	o := sampleOrder()

	fdb := &fakeDBBatch{}
	r := &OrdersRepo{Pool: fdb, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}

	err := r.UpsertOrder(context.Background(), o)
	require.NoError(t, err)

	require.NotNil(t, fdb.tx)
	require.True(t, fdb.tx.committed)
	require.False(t, fdb.tx.rolledBack)

	br := fdb.tx.br.(*fakeBatchResults)
	require.Equal(t, 4+len(o.Items), br.calls)
}

func Test_UpsertOrder_Batch_StepError_Rollback(t *testing.T) {
	o := sampleOrder()
	fdb := &fakeDBBatch{
		tx: &fakeTxBatch{
			br: &fakeBatchResults{failAt: 5},
		},
	}
	r := &OrdersRepo{Pool: fdb, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	err := r.UpsertOrder(context.Background(), o)
	require.ErrorContains(t, err, "batch step")
	require.NotNil(t, fdb.tx)
	require.True(t, fdb.tx.rolledBack)
	require.False(t, fdb.tx.committed)
}

func Test_UpsertOrder_Batch_CommitError(t *testing.T) {
	o := sampleOrder()
	fdb := &fakeDBBatch{
		tx: &fakeTxBatch{
			br:        &fakeBatchResults{},
			commitErr: errors.New("commit-fail"),
		},
	}
	r := &OrdersRepo{Pool: fdb, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	err := r.UpsertOrder(context.Background(), o)
	require.ErrorContains(t, err, "commit")
	require.ErrorContains(t, err, "commit-fail")
	require.True(t, fdb.tx.committed)
	require.False(t, fdb.tx.rolledBack)
}

func Test_UpsertOrder_BeginError_BadUID_NegAmount_PanicBatch(t *testing.T) {
	o := sampleOrder()
	f1 := &fakeDBBatch{beginErr: errors.New("begin-fail")}
	r1 := &OrdersRepo{Pool: f1, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	err := r1.UpsertOrder(context.Background(), o)
	require.EqualError(t, err, "begin tx: begin-fail")

	o2 := o
	o2.OrderUID = ""
	r2 := &OrdersRepo{Pool: &fakeDBBatch{}}
	require.ErrorIs(t, r2.UpsertOrder(context.Background(), o2), ErrBadUID)

	o3 := o
	o3.Payment.Amount = -1
	r3 := &OrdersRepo{Pool: &fakeDBBatch{}}
	err = r3.UpsertOrder(context.Background(), o3)
	require.ErrorContains(t, err, "negative amount")

	fdb := &fakeDBBatch{
		tx: &fakeTxBatch{
			br:            &fakeBatchResults{failAt: 0},
			panicOnCommit: true,
		},
	}
	r4 := &OrdersRepo{Pool: fdb, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}

	require.PanicsWithValue(t, "panic-commit", func() {
		_ = r4.UpsertOrder(context.Background(), o)
	})
	require.True(t, fdb.tx.rolledBack)
}

func Test_NewOrdersRepo_NilOK(t *testing.T) {
	r := NewOrdersRepo(nil)
	require.NotNil(t, r)
}

func Test_ListRecentOrderUIDs_RowsErrAfterLoop(t *testing.T) {
	m, _ := pgxmock.NewPool()
	defer m.Close()

	rows := pgxmock.NewRows([]string{"order_uid"}).AddRow("u1")
	rows.RowError(1, errors.New("rows-err"))
	m.ExpectQuery(`SELECT\s+order_uid`).WithArgs(1).WillReturnRows(rows)

	r := &OrdersRepo{Pool: m, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err := r.ListRecentOrderUIDs(context.Background(), 1)
	require.ErrorContains(t, err, "listRecent rows")
	require.ErrorContains(t, err, "rows-err")
	require.NoError(t, m.ExpectationsWereMet())
}

func Test_UpsertOrder_Batch_CloseError_Rollback(t *testing.T) {
	o := sampleOrder()

	fdb := &fakeDBBatch{}
	fdb.tx = &fakeTxBatch{
		br: &fakeBatchResults{closeErr: errors.New("close-fail")},
	}

	r := &OrdersRepo{Pool: fdb, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	err := r.UpsertOrder(context.Background(), o)

	require.ErrorContains(t, err, "batch close")
	require.ErrorContains(t, err, "close-fail")
	require.True(t, fdb.tx.rolledBack)
	require.False(t, fdb.tx.committed)
}

func Test_ListRecentOrderUIDs_ScanError(t *testing.T) {
	m, _ := pgxmock.NewPool()
	defer m.Close()

	rows := pgxmock.NewRows([]string{}).AddRow()

	m.ExpectQuery(`SELECT\s+order_uid`).WithArgs(1).WillReturnRows(rows)

	r := &OrdersRepo{Pool: m, qTimeout: 2 * time.Second, txTimeout: 5 * time.Second}
	_, err := r.ListRecentOrderUIDs(context.Background(), 1)

	require.ErrorContains(t, err, "listRecent scan")
	require.NoError(t, m.ExpectationsWereMet())
}
