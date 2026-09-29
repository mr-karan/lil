CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    oidc_issuer   TEXT NOT NULL,
    oidc_subject  TEXT NOT NULL,
    email         TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL,
    last_login_at DATETIME,
    disabled_at   DATETIME,
    session_epoch INTEGER NOT NULL DEFAULT 0,
    UNIQUE (oidc_issuer, oidc_subject)
);

CREATE TABLE api_tokens (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id),
    name         TEXT NOT NULL,
    token_hash   BLOB NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    created_at   DATETIME NOT NULL,
    revoked_at   DATETIME
);

CREATE TABLE sessions (
    token  TEXT PRIMARY KEY,
    data   BLOB NOT NULL,
    expiry REAL NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions(expiry);

ALTER TABLE urls ADD COLUMN created_by INTEGER REFERENCES users(id);
ALTER TABLE urls ADD COLUMN updated_by INTEGER REFERENCES users(id);
ALTER TABLE urls ADD COLUMN updated_at DATETIME;

CREATE TABLE audit_log (
    id         INTEGER PRIMARY KEY,
    created_at DATETIME NOT NULL,
    user_id    INTEGER NOT NULL REFERENCES users(id),
    token_id   INTEGER REFERENCES api_tokens(id),
    action     TEXT NOT NULL CHECK (action IN ('url.create','url.update','url.delete','token.create','token.revoke','user.disable','user.enable')),
    target     TEXT NOT NULL,
    before     TEXT,
    after      TEXT
);
CREATE INDEX audit_log_target_idx ON audit_log(action, target, id);
