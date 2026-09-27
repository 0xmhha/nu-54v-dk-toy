-- P07 receipts: one row per finalized PaymentSettled log ([N08][N19]).
CREATE TABLE IF NOT EXISTS receipts (
    merchant     TEXT        NOT NULL,
    order_id     TEXT        NOT NULL,
    device       TEXT        NOT NULL,
    amount       NUMERIC(78) NOT NULL,
    nonce        NUMERIC(78) NOT NULL,
    tx_hash      TEXT        NOT NULL,
    log_index    INTEGER     NOT NULL,
    block_number BIGINT      NOT NULL,
    PRIMARY KEY (merchant, order_id),
    UNIQUE (tx_hash, log_index)
);

CREATE TABLE IF NOT EXISTS cursor (
    id                   SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    last_finalized_block BIGINT   NOT NULL
);
