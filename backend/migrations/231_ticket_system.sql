CREATE TABLE IF NOT EXISTS tickets (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL,
    description TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at TIMESTAMPTZ,
    closed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT tickets_status_check CHECK (status IN ('pending', 'processing', 'closed'))
);

CREATE INDEX IF NOT EXISTS idx_tickets_user_created_at ON tickets(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tickets_status_updated_at ON tickets(status, updated_at DESC);

CREATE TABLE IF NOT EXISTS ticket_messages (
    id BIGSERIAL PRIMARY KEY,
    ticket_id BIGINT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    sender_type VARCHAR(10) NOT NULL,
    sender_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    images TEXT NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ticket_messages_sender_type_check CHECK (sender_type IN ('user', 'admin'))
);

CREATE INDEX IF NOT EXISTS idx_ticket_messages_ticket_created_at ON ticket_messages(ticket_id, created_at ASC, id ASC);
