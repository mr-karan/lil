CREATE TABLE IF NOT EXISTS urls (
    short_code TEXT PRIMARY KEY,
    url TEXT NOT NULL,
    title TEXT,
    created_at DATETIME NOT NULL,
    expires_at DATETIME
);

CREATE TABLE IF NOT EXISTS device_urls (
    short_code TEXT,
    platform TEXT CHECK(platform IN ('android', 'ios', 'web')),
    url TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (short_code) REFERENCES urls(short_code) ON DELETE CASCADE,
    PRIMARY KEY (short_code, platform)
);
