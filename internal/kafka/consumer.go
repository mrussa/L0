package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/mrussa/L0/internal/repo"
	"github.com/segmentio/kafka-go"
)

type orderStore interface {
	UpsertOrder(ctx context.Context, o repo.Order) error
}

type OrderCache interface {
	Set(key string, o repo.Order)
}

type reader interface {
	FetchMessage(context.Context) (kafka.Message, error)
	CommitMessages(context.Context, ...kafka.Message) error
	Close() error
}

const (
	minBytes  = 1
	maxBytes  = 10 * 1024 * 1024
	retryBase = 300 * time.Millisecond
)

var newReader = func(cfg kafka.ReaderConfig) reader { return kafka.NewReader(cfg) }

type Decoder func([]byte, *repo.Order) error
type Validator func(*repo.Order) error

func defaultDecode(b []byte, o *repo.Order) error { return json.Unmarshal(b, o) }

func defaultValidate(o *repo.Order) error {
	if o.OrderUID == "" {
		return fmt.Errorf("field order_uid: empty")
	}
	if len(o.OrderUID) > 100 {
		return fmt.Errorf("field order_uid: too long")
	}
	if o.TrackNumber == "" {
		return fmt.Errorf("field track_number: empty")
	}
	if o.Payment.Currency == "" {
		return fmt.Errorf("field currency: empty")
	}
	if o.Payment.Amount < 0 {
		return fmt.Errorf("field amount: negative")
	}
	return nil
}

type Consumer struct {
	Brokers []string
	Topic   string
	Group   string

	Repo  orderStore
	Cache OrderCache

	Logf     func(string, ...any)
	Decode   Decoder
	Validate Validator

	RetryBase time.Duration
}

func NewConsumer(brokersCSV, topic, group string, r *repo.OrdersRepo, c OrderCache, logf func(string, ...any)) *Consumer {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Consumer{
		Brokers:   splitCSV(brokersCSV),
		Topic:     topic,
		Group:     group,
		Repo:      r,
		Cache:     c,
		Logf:      logf,
		Decode:    defaultDecode,
		Validate:  defaultValidate,
		RetryBase: retryBase,
	}
}

func (c *Consumer) Run(ctx context.Context) error {
	r := newReader(kafka.ReaderConfig{
		Brokers:        c.Brokers,
		GroupID:        c.Group,
		Topic:          c.Topic,
		MinBytes:       minBytes,
		MaxBytes:       maxBytes,
		CommitInterval: 0,
	})
	defer r.Close()

	c.Logf("[KAFKA] reader connected (group=%s topic=%s brokers=%v)", c.Group, c.Topic, c.Brokers)

	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				c.Logf("[KAFKA] stopped: %v", err)
				return err
			}
			c.Logf("[KAFKA] fetch error: %v", err)
			return err
		}
		c.handleMessage(ctx, r, msg)
	}
}

func (c *Consumer) handleMessage(ctx context.Context, r reader, msg kafka.Message) {
	var ord repo.Order

	if err := c.Decode(msg.Value, &ord); err != nil {
		c.Logf("[KAFKA] bad json %s[%d]#%d: %v", msg.Topic, msg.Partition, msg.Offset, err)
		_ = r.CommitMessages(ctx, msg)
		return
	}

	if len(msg.Key) > 0 && string(msg.Key) != ord.OrderUID {
		c.Logf("[KAFKA] key/payload mismatch %s[%d]#%d: key=%q payload=%q",
			msg.Topic, msg.Partition, msg.Offset, string(msg.Key), ord.OrderUID)
	}

	if err := c.Validate(&ord); err != nil {
		c.Logf("[KAFKA] invalid %q %s[%d]#%d: %v",
			ord.OrderUID, msg.Topic, msg.Partition, msg.Offset, err)
		_ = r.CommitMessages(ctx, msg)
		return
	}

	if err := c.Repo.UpsertOrder(ctx, ord); err != nil {
		c.Logf("[KAFKA] upsert %s: %v", ord.OrderUID, err)
		c.backoff()
		return
	}

	c.Cache.Set(ord.OrderUID, ord)
	c.Logf("[KAFKA] stored %s (items=%d)", ord.OrderUID, len(ord.Items))

	if err := r.CommitMessages(ctx, msg); err != nil {
		c.Logf("[KAFKA] commit error %s[%d]#%d: %v", msg.Topic, msg.Partition, msg.Offset, err)
	}
}

func (c *Consumer) backoff() {
	j := time.Duration(rand.Intn(200)) * time.Millisecond
	time.Sleep(c.RetryBase + j)
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
