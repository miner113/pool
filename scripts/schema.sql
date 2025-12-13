-- Juno Pool Database Schema
-- This file is used by docker-compose to initialize the database

-- Miners table
CREATE TABLE IF NOT EXISTS miners (
    id SERIAL PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    payout_address TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Balances table
CREATE TABLE IF NOT EXISTS balances (
    miner_id INTEGER PRIMARY KEY REFERENCES miners(id),
    balance DOUBLE PRECISION NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Shares table
CREATE TABLE IF NOT EXISTS shares (
    id BIGSERIAL PRIMARY KEY,
    miner_id INTEGER NOT NULL REFERENCES miners(id),
    job_id TEXT NOT NULL,
    difficulty DOUBLE PRECISION NOT NULL,
    accepted BOOLEAN NOT NULL,
    stale BOOLEAN NOT NULL DEFAULT FALSE,
    invalid BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for share queries
CREATE INDEX IF NOT EXISTS idx_shares_miner_created ON shares(miner_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_shares_job ON shares(job_id);

-- Blocks table
CREATE TABLE IF NOT EXISTS blocks (
    id BIGSERIAL PRIMARY KEY,
    job_id TEXT NOT NULL,
    height BIGINT NOT NULL,
    status TEXT NOT NULL,
    hash TEXT,
    confirmations INTEGER NOT NULL DEFAULT 0,
    paid BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for block queries
CREATE INDEX IF NOT EXISTS idx_blocks_height ON blocks(height DESC);
CREATE INDEX IF NOT EXISTS idx_blocks_status ON blocks(status);

-- Payouts table
CREATE TABLE IF NOT EXISTS payouts (
    id BIGSERIAL PRIMARY KEY,
    miner_id INTEGER NOT NULL REFERENCES miners(id),
    amount DOUBLE PRECISION NOT NULL,
    status TEXT NOT NULL,
    txid TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for payout queries
CREATE INDEX IF NOT EXISTS idx_payouts_miner ON payouts(miner_id);
CREATE INDEX IF NOT EXISTS idx_payouts_status ON payouts(status);
