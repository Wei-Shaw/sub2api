-- Store the optional MetaCubeXD dashboard address for proxies managed in IP management.
ALTER TABLE proxies
    ADD COLUMN IF NOT EXISTS console_url VARCHAR(2048);
