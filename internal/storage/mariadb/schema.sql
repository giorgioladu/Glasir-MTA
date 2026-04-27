-- Glasir MariaDB Schema
-- Apply manually only if you prefer not to use the auto-migration in Go.
-- The Go backend runs these statements automatically on first start.

CREATE TABLE IF NOT EXISTS users (
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    address    VARCHAR(320) NOT NULL UNIQUE COMMENT 'Full Yggdrasil email address',
    passhash   VARCHAR(255) NOT NULL      COMMENT 'bcrypt hash of the password',
    quota_bytes BIGINT UNSIGNED DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT NOW()
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS mailboxes (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id      BIGINT UNSIGNED NOT NULL,
    name         VARCHAR(255) NOT NULL       COMMENT 'Folder name e.g. INBOX, Sent, Trash',
    uid_validity INT UNSIGNED NOT NULL DEFAULT 1,
    uid_next     INT UNSIGNED NOT NULL DEFAULT 1,
    UNIQUE KEY uq_user_folder (user_id, name),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS messages (
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    mailbox_id BIGINT UNSIGNED NOT NULL,
    uid        INT UNSIGNED NOT NULL       COMMENT 'IMAP UID, unique within mailbox',
    flags      TEXT                        COMMENT 'Space-separated IMAP flags e.g. \\Seen \\Answered',
    size       INT UNSIGNED NOT NULL DEFAULT 0,
    body       LONGBLOB                    COMMENT 'Raw RFC 5322 message, up to 4 GB',
    received   DATETIME NOT NULL DEFAULT NOW(),
    UNIQUE KEY uq_mailbox_uid (mailbox_id, uid),
    INDEX idx_mailbox   (mailbox_id),
    FOREIGN KEY (mailbox_id) REFERENCES mailboxes(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 ROW_FORMAT=DYNAMIC;

CREATE TABLE IF NOT EXISTS aliases (
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    alias      VARCHAR(320) NOT NULL UNIQUE,
    user_id    BIGINT UNSIGNED NOT NULL,
    is_Glasir BOOLEAN NOT NULL DEFAULT 0,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS queue (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    destination VARCHAR(320) NOT NULL,
    message_id  BIGINT UNSIGNED NOT NULL,
    from_addr   VARCHAR(320) NOT NULL,
    rcpt_addr   VARCHAR(320) NOT NULL,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    attempts INT DEFAULT 0,
    last_attempt TIMESTAMP NULL,
    last_error TEXT,

    INDEX idx_dest (destination),
    INDEX idx_attempts (attempts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS message_index (
    message_id BIGINT UNSIGNED PRIMARY KEY,
    subject    VARCHAR(998),
    from_addr  VARCHAR(320),
    to_addr    TEXT,
    date_hdr   DATETIME,
    message_id_hdr VARCHAR(255),
    parsed     BOOLEAN DEFAULT 0,

    INDEX idx_from (from_addr),
    INDEX idx_date (date_hdr),
    INDEX idx_msgid (message_id_hdr),
    FULLTEXT KEY ft_subject (subject)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE OR REPLACE VIEW user_quota_usage AS
SELECT 
    u.id,
    u.quota_bytes,
    COALESCE(SUM(m.size),0) AS used_bytes,
    (u.quota_bytes - COALESCE(SUM(m.size),0)) AS remaining_bytes
FROM users u
LEFT JOIN mailboxes mb ON mb.user_id = u.id
LEFT JOIN messages m ON m.mailbox_id = mb.id
GROUP BY u.id;

CREATE TABLE IF NOT EXISTS message_processing_queue (
    message_id BIGINT UNSIGNED PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    processed BOOLEAN DEFAULT 0
);

DELIMITER $$

CREATE TRIGGER after_message_insert
AFTER INSERT ON messages
FOR EACH ROW
BEGIN
    INSERT IGNORE INTO message_processing_queue (message_id)
    VALUES (NEW.id);
END$$

DELIMITER ;

CREATE INDEX idx_mailbox_received ON messages(mailbox_id, received);
ALTER TABLE messages ADD COLUMN body_hash BINARY(32);
CREATE INDEX idx_body_hash ON messages(body_hash);

INSERT IGNORE INTO users (address, passhash, quota_bytes) 
VALUES ('system', '$2a$10$password_sicura#disabled', 0);

INSERT IGNORE INTO mailboxes (user_id, name)
SELECT id, 'Outbox' FROM users WHERE address = 'system';

INSERT IGNORE INTO mailboxes (user_id, name)
SELECT id, 'Sent' FROM users WHERE address = 'system';
