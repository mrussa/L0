package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mrussa/L0/internal/repo"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/require"
)

type step struct {
	msg kafka.Message
	err error
}

type stubRepo struct {
	calls int
	err   error
	last  repo.Order
}

type fakeReader struct {
	steps       []step
	i           int
	closed      bool
	commits     []kafka.Message
	commitErrAt int
	commitCalls int
}

func (f *fakeReader) FetchMessage(ctx context.Context) (kafka.Message, error) {
	if f.i >= len(f.steps) {
		return kafka.Message{}, context.Canceled
	}
	s := f.steps[f.i]
	f.i++
	if s.err != nil {
		return kafka.Message{}, s.err
	}
	return s.msg, nil
}

func (f *fakeReader) CommitMessages(ctx context.Context, msgs ...kafka.Message) error {
	f.commits = append(f.commits, msgs...)
	f.commitCalls++
	if f.commitErrAt != 0 && f.commitCalls == f.commitErrAt {
		return errors.New("commit-err")
	}
	return nil
}

func (f *fakeReader) Close() error { f.closed = true; return nil }

type fakeCache struct {
	setCalls int
	lastKey  string
	lastOrd  repo.Order
}

func (c *fakeCache) Set(key string, o repo.Order) {
	c.setCalls++
	c.lastKey = key
	c.lastOrd = o
}

func validOrder() repo.Order {
	return repo.Order{
		OrderUID:    "uid-1",
		TrackNumber: "TRK",
		Locale:      "en",
		Entry:       "WBIL",
		DateCreated: time.Now().UTC(),
		Payment: repo.Payment{
			TransactionID: "tx-1",
			Currency:      "USD",
			Amount:        10,
			PaymentDT:     1,
		},
		Delivery: repo.Delivery{
			Name: "Name", City: "City", Address: "Addr", Region: "Reg", Phone: "+1", Email: "a@b.c",
		},
	}
}

func toJSON(t *testing.T, o repo.Order) []byte {
	t.Helper()
	b, err := json.Marshal(o)
	require.NoError(t, err)
	return b
}

func Test_splitCSV(t *testing.T) {
	require.Nil(t, splitCSV(""))
	require.Equal(t, []string{"a"}, splitCSV("a"))
	require.Equal(t, []string{"a", "b", "c"}, splitCSV(" , a, b ,, c , "))
}

func Test_NewConsumer(t *testing.T) {
	r := &repo.OrdersRepo{}
	fc := &fakeCache{}
	got := NewConsumer("k1:9092,  k2:9092 , ,", "topic", "group", r, fc, func(string, ...any) {})
	require.Equal(t, []string{"k1:9092", "k2:9092"}, got.Brokers)
	require.Equal(t, "topic", got.Topic)
	require.Equal(t, "group", got.Group)
	require.Same(t, r, got.Repo)
	require.Same(t, fc, got.Cache)
	require.NotNil(t, got.Logf)
	require.NotNil(t, got.Decode)
	require.NotNil(t, got.Validate)
	require.Equal(t, retryBase, got.RetryBase)
}

func Test_defaultValidate_OK_and_Errors(t *testing.T) {
	o := validOrder()
	require.NoError(t, defaultValidate(&o))

	o2 := o
	o2.OrderUID = ""
	require.EqualError(t, defaultValidate(&o2), "field order_uid: empty")

	o3 := o
	o3.OrderUID = string(make([]byte, 101))
	require.EqualError(t, defaultValidate(&o3), "field order_uid: too long")

	o4 := o
	o4.TrackNumber = ""
	require.EqualError(t, defaultValidate(&o4), "field track_number: empty")

	o5 := o
	o5.Payment.Currency = ""
	require.EqualError(t, defaultValidate(&o5), "field currency: empty")

	o6 := o
	o6.Payment.Amount = -1
	require.EqualError(t, defaultValidate(&o6), "field amount: negative")
}

func withReader(t *testing.T, fr *fakeReader, fn func(*Consumer) error) error {
	t.Helper()
	orig := newReader
	newReader = func(cfg kafka.ReaderConfig) reader { return fr }
	defer func() { newReader = orig }()

	return fn(&Consumer{
		Brokers:   []string{"dummy:9092"},
		Topic:     "t",
		Group:     "g",
		Repo:      &repo.OrdersRepo{},
		Cache:     &fakeCache{},
		Logf:      func(string, ...any) {},
		Decode:    defaultDecode,
		Validate:  defaultValidate,
		RetryBase: 0,
	})
}

func Test_Run_FetchCanceled(t *testing.T) {
	fr := &fakeReader{steps: []step{{err: context.Canceled}}}
	err := withReader(t, fr, func(c *Consumer) error { return c.Run(context.Background()) })
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, fr.closed)
}

func Test_Run_FetchOtherError(t *testing.T) {
	fr := &fakeReader{steps: []step{{err: errors.New("boom")}}}
	err := withReader(t, fr, func(c *Consumer) error { return c.Run(context.Background()) })
	require.EqualError(t, err, "boom")
}

func Test_Run_BadJSON_CommitsAndContinues(t *testing.T) {
	fr := &fakeReader{
		steps: []step{
			{msg: kafka.Message{Topic: "t", Partition: 0, Offset: 1, Value: []byte("not-json")}},
			{err: context.Canceled},
		},
	}
	err := withReader(t, fr, func(c *Consumer) error { return c.Run(context.Background()) })
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, fr.commits, 1)
	require.Equal(t, int64(1), fr.commits[0].Offset)
}

func Test_Run_InvalidOrder_CommitsAndSkipsUpsert(t *testing.T) {
	ord := validOrder()
	ord.Payment.Currency = ""
	fr := &fakeReader{
		steps: []step{
			{msg: kafka.Message{Topic: "t", Partition: 0, Offset: 2, Key: []byte(ord.OrderUID), Value: toJSON(t, ord)}},
			{err: context.Canceled},
		},
	}
	err := withReader(t, fr, func(c *Consumer) error {
		c.Repo = &repo.OrdersRepo{}
		return c.Run(context.Background())
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, fr.commits, 1)
}

func Test_Run_UpsertError_RetriesWithoutCommit(t *testing.T) {
	ord := validOrder()
	msg := kafka.Message{Topic: "t", Partition: 0, Offset: 3, Key: []byte(ord.OrderUID), Value: toJSON(t, ord)}
	fr := &fakeReader{
		steps: []step{
			{msg: msg},
			{err: context.Canceled},
		},
	}
	err := withReader(t, fr, func(c *Consumer) error {
		mock, _ := pgxmock.NewPool()
		defer mock.Close()
		mock.ExpectBegin()
		mock.ExpectExec(".*").WithArgs(
			ord.OrderUID, ord.TrackNumber, ord.Entry, ord.Locale, ord.InternalSignature, ord.CustomerID,
			ord.DeliveryService, ord.ShardKey, ord.SMID, pgxmock.AnyArg(), ord.OofShard,
		).WillReturnError(errors.New("order-fail"))
		mock.ExpectRollback()
		c.Repo = &repo.OrdersRepo{Pool: mock}
		return c.Run(context.Background())
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, fr.commits, 0)
}

func Test_newReader_DefaultUsesKafkaNewReader(t *testing.T) {
	r := newReader(kafka.ReaderConfig{
		Brokers:        []string{"127.0.0.1:1"},
		GroupID:        "g",
		Topic:          "t",
		MinBytes:       1,
		MaxBytes:       1,
		CommitInterval: 0,
	})
	require.NoError(t, r.Close())
}

func Test_NewConsumer_NilLogger_NoPanic(t *testing.T) {
	c := NewConsumer("b1:9092", "t", "g", &repo.OrdersRepo{}, &fakeCache{}, nil)
	require.NotNil(t, c.Logf)
	require.NotPanics(t, func() { c.Logf("hello %d", 1) })
}

func (s *stubRepo) UpsertOrder(ctx context.Context, o repo.Order) error {
	s.calls++
	s.last = o
	return s.err
}

func Test_handleMessage_Success_CommitsAndCaches_OneUpsert(t *testing.T) {
	ord := validOrder()
	msg := kafka.Message{
		Topic:     "t",
		Partition: 0,
		Offset:    10,
		Key:       []byte("DIFF_KEY"),
		Value:     toJSON(t, ord),
	}

	fr := &fakeReader{}
	fc := &fakeCache{}
	sr := &stubRepo{}

	c := &Consumer{
		Repo:     sr,
		Cache:    fc,
		Logf:     func(string, ...any) {},
		Decode:   defaultDecode,
		Validate: defaultValidate,
	}

	c.handleMessage(context.Background(), fr, msg)

	require.Equal(t, 1, sr.calls, "UpsertOrder должен быть вызван ровно один раз")
	require.Equal(t, ord.OrderUID, sr.last.OrderUID, "в UpsertOrder должен прийти тот же заказ")
	require.Equal(t, 1, fc.setCalls, "кэш должен обновиться")
	require.Equal(t, ord.OrderUID, fc.lastKey)
	require.Equal(t, 1, fr.commitCalls, "должна быть одна попытка коммита")
}

func Test_handleMessage_CommitError_StillCaches_AndNoPanic(t *testing.T) {
	ord := validOrder()
	msg := kafka.Message{
		Topic:     "t",
		Partition: 0,
		Offset:    11,
		Key:       []byte("DIFF_KEY"),
		Value:     toJSON(t, ord),
	}

	fr := &fakeReader{commitErrAt: 1}
	fc := &fakeCache{}
	sr := &stubRepo{}

	c := &Consumer{
		Repo:     sr,
		Cache:    fc,
		Logf:     func(string, ...any) {},
		Decode:   defaultDecode,
		Validate: defaultValidate,
	}

	c.handleMessage(context.Background(), fr, msg)

	require.Equal(t, 1, sr.calls, "UpsertOrder должен выполниться")
	require.Equal(t, 1, fc.setCalls, "кэш должен обновиться даже если commit вернул ошибку")
	require.Equal(t, 1, fr.commitCalls, "должна быть хотя бы одна попытка коммита")
}

func Test_handleMessage_UpsertError_NoCache_NoCommit(t *testing.T) {
	ord := validOrder()
	msg := kafka.Message{
		Topic:     "t",
		Partition: 0,
		Offset:    12,
		Key:       []byte(ord.OrderUID),
		Value:     toJSON(t, ord),
	}

	fr := &fakeReader{}
	fc := &fakeCache{}
	sr := &stubRepo{err: errors.New("upsert-fail")}

	c := &Consumer{
		Repo:      sr,
		Cache:     fc,
		Logf:      func(string, ...any) {},
		Decode:    defaultDecode,
		Validate:  defaultValidate,
		RetryBase: 0,
	}

	c.handleMessage(context.Background(), fr, msg)

	require.Equal(t, 1, sr.calls, "попытка UpsertOrder должна быть")
	require.Equal(t, 0, fc.setCalls, "кэш не должен обновляться при ошибке Upsert")
	require.Equal(t, 0, fr.commitCalls, "коммита быть не должно при ошибке Upsert")
}
