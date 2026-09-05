CREATE TABLE plugin_generation_history (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL DEFAULT '',
  user_id BIGINT NOT NULL,
  user_email TEXT NOT NULL,
  plugin_key TEXT NOT NULL,
  prompt TEXT NOT NULL,
  status TEXT NOT NULL,
  request_payload TEXT NOT NULL,
  result_payload TEXT,
  error_message TEXT,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_plugin_generation_history_user_created
  ON plugin_generation_history(user_id, created_at DESC);

CREATE INDEX idx_plugin_generation_history_created
  ON plugin_generation_history(created_at DESC);

CREATE INDEX idx_plugin_generation_history_user_conversation_updated
  ON plugin_generation_history(user_id, conversation_id, updated_at DESC, id DESC);

CREATE INDEX idx_plugin_generation_history_conversation_updated
  ON plugin_generation_history(conversation_id, updated_at DESC, id DESC);

CREATE INDEX idx_plugin_generation_history_user_conversation_created
  ON plugin_generation_history(user_id, conversation_id, created_at DESC, id DESC);
