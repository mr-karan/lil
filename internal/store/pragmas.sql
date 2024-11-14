PRAGMA busy_timeout       = 5000;        -- Increased wait time to 5s for busy/locked DB
PRAGMA journal_mode       = WAL;         -- Write-Ahead Logging for better concurrency
PRAGMA journal_size_limit = 31457280;    -- Increased WAL file size limit to 30MB
PRAGMA synchronous       = NORMAL;       -- Balance between safety and performance
PRAGMA foreign_keys      = ON;           -- Keep foreign key constraints enabled
PRAGMA temp_store        = MEMORY;       -- Use memory for temp storage
PRAGMA cache_size        = -64000;       -- Increased cache to 64MB to reduce disk I/O
PRAGMA page_size         = 4096;         -- Optimal page size for most SSDs
PRAGMA mmap_size         = 536870912;    -- Use memory mapping for up to 512MB
PRAGMA wal_autocheckpoint = 2000;        -- Checkpoint WAL file every 2000 pages
PRAGMA locking_mode      = NORMAL;       -- Use NORMAL locking for better concurrency
PRAGMA read_uncommitted  = 1;            -- Enable read uncommitted isolation for better concurrency
