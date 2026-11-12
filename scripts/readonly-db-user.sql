-- Create a read-only Postgres role for ad-hoc queries against the application
-- database (e.g. server/cmd/walkthrough-export). SELECT only, including on
-- tables the server creates later at startup.
--
-- Run against the local dev database after the server has started (so that
-- tables exist):
--
--   npm run db:init-reader
--
-- For any other database, substitute the database name, the application role
-- (the owner of the tables, `ripls` locally) and a real password. The Cloud SQL
-- instances predate this name and carry the same grants as `metabase_reader`.

-- Create role if it doesn't already exist.
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'ripls_reader') THEN
    CREATE ROLE ripls_reader LOGIN PASSWORD 'ripls_reader_dev';
  END IF;
END
$$;

-- Grant read-only access to all existing tables and sequences.
GRANT CONNECT ON DATABASE ripls TO ripls_reader;
GRANT USAGE ON SCHEMA public TO ripls_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO ripls_reader;
GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO ripls_reader;

-- Auto-grant SELECT on any tables created in the future (by the app user).
ALTER DEFAULT PRIVILEGES FOR ROLE ripls IN SCHEMA public
  GRANT SELECT ON TABLES TO ripls_reader;
