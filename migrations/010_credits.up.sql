ALTER TABLE users ADD COLUMN ai_credits INT NOT NULL DEFAULT 50;

CREATE TABLE credit_transactions (
    id          UUID PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount      INT NOT NULL,
    type        VARCHAR(50) NOT NULL,
    description TEXT,
    payment_label TEXT UNIQUE,
    status      VARCHAR(20) NOT NULL DEFAULT 'completed',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_credit_transactions_user    ON credit_transactions (user_id, created_at DESC);
CREATE INDEX idx_credit_transactions_label   ON credit_transactions (payment_label) WHERE payment_label IS NOT NULL;
