ALTER TABLE web3_deposits
    ALTER COLUMN token_amount TYPE NUMERIC(78,6)
    USING token_amount::NUMERIC(78,6);
