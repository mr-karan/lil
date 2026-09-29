DROP TABLE audit_log;
DROP TABLE sessions;
DROP TABLE api_tokens;

CREATE TEMP TABLE device_urls_backup AS SELECT * FROM device_urls;

CREATE TABLE urls_old (
    short_code TEXT PRIMARY KEY,
    url TEXT NOT NULL,
    title TEXT,
    created_at DATETIME NOT NULL,
    expires_at DATETIME
);
INSERT INTO urls_old (short_code, url, title, created_at, expires_at)
    SELECT short_code, url, title, created_at, expires_at FROM urls;
DROP TABLE urls;
ALTER TABLE urls_old RENAME TO urls;

INSERT INTO device_urls SELECT * FROM device_urls_backup;
DROP TABLE device_urls_backup;

DROP TABLE users;
