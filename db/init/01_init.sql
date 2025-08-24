SET ROLE orders_user;
SET search_path TO public;

-- Таблицы
CREATE TABLE IF NOT EXISTS orders (
  order_uid          varchar(100) PRIMARY KEY,
  track_number       text NOT NULL,
  entry              text NOT NULL,
  locale             text NOT NULL,
  internal_signature text NOT NULL,
  customer_id        text NOT NULL,
  delivery_service   text NOT NULL,
  shardkey           text NOT NULL,
  sm_id              integer NOT NULL,
  date_created       timestamptz NOT NULL,
  oof_shard          text NOT NULL
);

CREATE TABLE IF NOT EXISTS order_payment (
  order_uid      varchar(100) PRIMARY KEY REFERENCES orders(order_uid) ON DELETE CASCADE,
  transaction_id text NOT NULL,
  request_id     text NOT NULL,
  currency       text NOT NULL,
  provider       text NOT NULL,
  amount         integer NOT NULL,
  payment_dt     bigint NOT NULL,
  bank           text NOT NULL,
  delivery_cost  integer NOT NULL,
  goods_total    integer NOT NULL,
  custom_fee     integer NOT NULL
);

CREATE TABLE IF NOT EXISTS order_items (
  id           bigserial PRIMARY KEY,
  order_uid    varchar(100) NOT NULL REFERENCES orders(order_uid) ON DELETE CASCADE,
  chrt_id      bigint NOT NULL,
  track_number text   NOT NULL,
  price        integer NOT NULL,
  rid          text   NOT NULL,
  name         text   NOT NULL,
  sale         integer NOT NULL,
  size         text   NOT NULL,
  total_price  integer NOT NULL,
  nm_id        bigint NOT NULL,
  brand        text   NOT NULL,
  status       integer NOT NULL
);

CREATE TABLE IF NOT EXISTS order_delivery (
  order_uid varchar(100) PRIMARY KEY REFERENCES orders(order_uid) ON DELETE CASCADE,
  name      text NOT NULL,
  phone     text NOT NULL,
  zip       text NOT NULL,
  city      text NOT NULL,
  address   text NOT NULL,
  region    text NOT NULL,
  email     text NOT NULL
); 

-- Индексы
CREATE INDEX IF NOT EXISTS idx_order_items_order_uid ON order_items(order_uid);
CREATE INDEX IF NOT EXISTS idx_orders_date_created   ON orders(date_created DESC);
