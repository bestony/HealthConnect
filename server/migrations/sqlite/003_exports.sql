CREATE TABLE IF NOT EXISTS exports (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id),
  started_at TEXT NOT NULL,
  status TEXT NOT NULL,
  sample_count INTEGER,
  batch_count INTEGER,
  received_batch_count INTEGER NOT NULL DEFAULT 0,
  completed_at TEXT,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS exports_user ON exports(user_id);
