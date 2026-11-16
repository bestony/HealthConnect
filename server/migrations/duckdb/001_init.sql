CREATE TABLE quantity_samples (
  user_id UUID NOT NULL,
  import_id UUID NOT NULL,
  hk_uuid VARCHAR,
  natural_key BLOB NOT NULL,
  type VARCHAR NOT NULL,
  value DOUBLE NOT NULL,
  unit VARCHAR NOT NULL,
  source_name VARCHAR NOT NULL,
  source_version VARCHAR,
  device VARCHAR,
  start_utc TIMESTAMP NOT NULL,
  end_utc TIMESTAMP NOT NULL,
  tz_offset_min SMALLINT NOT NULL,
  creation_utc TIMESTAMP,
  metadata JSON,
  attrs JSON
);

CREATE TABLE quantity_samples_staging AS SELECT * FROM quantity_samples WHERE FALSE;

CREATE TABLE category_samples (
  user_id UUID NOT NULL,
  import_id UUID NOT NULL,
  hk_uuid VARCHAR,
  natural_key BLOB NOT NULL,
  type VARCHAR NOT NULL,
  value_text VARCHAR NOT NULL,
  source_name VARCHAR NOT NULL,
  source_version VARCHAR,
  device VARCHAR,
  start_utc TIMESTAMP NOT NULL,
  end_utc TIMESTAMP NOT NULL,
  tz_offset_min SMALLINT NOT NULL,
  creation_utc TIMESTAMP,
  metadata JSON,
  attrs JSON
);

CREATE TABLE category_samples_staging AS SELECT * FROM category_samples WHERE FALSE;

CREATE TABLE correlations (
  id UUID NOT NULL,
  user_id UUID NOT NULL,
  import_id UUID NOT NULL,
  hk_uuid VARCHAR,
  natural_key BLOB NOT NULL,
  type VARCHAR NOT NULL,
  source_name VARCHAR NOT NULL,
  source_version VARCHAR,
  device VARCHAR,
  start_utc TIMESTAMP NOT NULL,
  end_utc TIMESTAMP NOT NULL,
  tz_offset_min SMALLINT NOT NULL,
  creation_utc TIMESTAMP,
  metadata JSON,
  attrs JSON
);

CREATE TABLE correlations_staging AS SELECT * FROM correlations WHERE FALSE;

CREATE TABLE correlation_members (
  correlation_id UUID NOT NULL,
  import_id UUID NOT NULL,
  member_natural_key BLOB NOT NULL
);

CREATE TABLE correlation_members_staging AS SELECT * FROM correlation_members WHERE FALSE;

CREATE TABLE workouts (
  id UUID NOT NULL,
  user_id UUID NOT NULL,
  import_id UUID NOT NULL,
  hk_uuid VARCHAR,
  natural_key BLOB NOT NULL,
  activity_type VARCHAR NOT NULL,
  duration_sec DOUBLE,
  distance DOUBLE,
  distance_unit VARCHAR,
  energy DOUBLE,
  energy_unit VARCHAR,
  source_name VARCHAR NOT NULL,
  source_version VARCHAR,
  device VARCHAR,
  start_utc TIMESTAMP NOT NULL,
  end_utc TIMESTAMP NOT NULL,
  tz_offset_min SMALLINT NOT NULL,
  creation_utc TIMESTAMP,
  metadata JSON,
  attrs JSON
);

CREATE TABLE workouts_staging AS SELECT * FROM workouts WHERE FALSE;

CREATE TABLE workout_events (
  workout_id UUID NOT NULL,
  import_id UUID NOT NULL,
  event_type VARCHAR NOT NULL,
  event_utc TIMESTAMP NOT NULL,
  tz_offset_min SMALLINT NOT NULL,
  duration_sec DOUBLE
);

CREATE TABLE workout_events_staging AS SELECT * FROM workout_events WHERE FALSE;

CREATE TABLE workout_statistics (
  workout_id UUID NOT NULL,
  import_id UUID NOT NULL,
  type VARCHAR NOT NULL,
  sum_value DOUBLE,
  minimum DOUBLE,
  maximum DOUBLE,
  average DOUBLE,
  unit VARCHAR
);

CREATE TABLE workout_statistics_staging AS SELECT * FROM workout_statistics WHERE FALSE;

CREATE TABLE activity_summaries (
  user_id UUID NOT NULL,
  import_id UUID NOT NULL,
  day DATE NOT NULL,
  active_energy DOUBLE,
  active_energy_goal DOUBLE,
  active_energy_unit VARCHAR,
  exercise_minutes DOUBLE,
  exercise_goal DOUBLE,
  stand_hours DOUBLE,
  stand_goal DOUBLE,
  move_minutes DOUBLE,
  move_goal DOUBLE,
  attrs JSON
);

CREATE TABLE activity_summaries_staging AS SELECT * FROM activity_summaries WHERE FALSE;

CREATE INDEX quantity_user_type_start ON quantity_samples (user_id, type, start_utc);
CREATE INDEX category_user_type_start ON category_samples (user_id, type, start_utc);
CREATE INDEX workouts_user_start ON workouts (user_id, start_utc);
CREATE INDEX activity_user_day ON activity_summaries (user_id, day);
