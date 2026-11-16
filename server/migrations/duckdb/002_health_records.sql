CREATE TABLE health_records (
  user_id UUID NOT NULL,
  import_id UUID NOT NULL,
  kind VARCHAR NOT NULL,
  type_identifier VARCHAR NOT NULL,
  hk_uuid VARCHAR,
  start_utc TIMESTAMP,
  end_utc TIMESTAMP,
  tz_offset_min SMALLINT,
  source_name VARCHAR,
  device VARCHAR,
  payload JSON
);

CREATE TABLE health_records_staging AS SELECT * FROM health_records WHERE FALSE;

CREATE INDEX health_records_user_type ON health_records (user_id, type_identifier, start_utc);
