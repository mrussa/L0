-- создаём логин для приложения (если нет)
DO $do$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'orders_user') THEN
    CREATE ROLE orders_user LOGIN PASSWORD 'orders_pass';
  END IF;
END
$do$;

-- назначаем владельца БД и схемы
ALTER DATABASE orders_db OWNER TO orders_user;
ALTER SCHEMA public OWNER TO orders_user;

-- выдача прав на подключение/создание объектов
GRANT ALL PRIVILEGES ON DATABASE orders_db TO orders_user;
