ALTER TABLE web3_deposit_wallets
    DROP CONSTRAINT IF EXISTS web3_deposit_wallets_next_index_check;

ALTER TABLE web3_deposit_wallets
    ADD CONSTRAINT web3_deposit_wallets_next_index_check
    CHECK (next_derivation_index >= 0 AND next_derivation_index <= 2147483648);
