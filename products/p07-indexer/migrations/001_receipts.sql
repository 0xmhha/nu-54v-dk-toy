-- P07 receipts: one row per finalized PaymentSettled log ([N08][N19]).
-- The key is the log itself, so two logs for one (merchant, order_id) can both be stored and
-- the lookup can mark the receipt as duplicate (P07-FR-05).
CREATE TABLE IF NOT EXISTS receipts (
    tx_hash      TEXT        NOT NULL,
    log_index    INTEGER     NOT NULL,
    merchant     TEXT        NOT NULL,
    order_id     TEXT        NOT NULL,
    device       TEXT        NOT NULL,
    amount       NUMERIC(78) NOT NULL,
    nonce        NUMERIC(78) NOT NULL,
    block_number BIGINT      NOT NULL,
    block_time   BIGINT      NOT NULL,
    PRIMARY KEY (tx_hash, log_index)
);
CREATE INDEX IF NOT EXISTS receipts_lookup ON receipts (merchant, order_id, block_number, log_index);

-- The last finalized block whose logs are all stored.
CREATE TABLE IF NOT EXISTS cursor (
    id           SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    block_number BIGINT   NOT NULL
);
