DROP DATABASE IF EXISTS messenger;
CREATE DATABASE IF NOT EXISTS messenger;
SET DATABASE = messenger;

CREATE TABLE IF NOT EXISTS users (
    id SERIAL NOT NULL PRIMARY KEY,
    username STRING NOT NULL UNIQUE,
    avatar_url STRING,
    github_id INT UNIQUE
);

CREATE TABLE IF NOT EXISTS conversations (
    id SERIAL NOT NULL PRIMARY KEY,
    last_message_id INT,
    INDEX (last_message_id)
);

CREATE TABLE IF NOT EXISTS participants (
    user_id INT NOT NULL REFERENCES users ON DELETE CASCADE,
    conversation_id INT NOT NULL REFERENCES conversations ON DELETE CASCADE,
    messages_read_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, conversation_id)
);

CREATE TABLE IF NOT EXISTS messages (
    id SERIAL NOT NULL PRIMARY KEY,
    content STRING(480) NOT NULL,
    user_id INT NOT NULL REFERENCES users ON DELETE CASCADE,
    conversation_id INT NOT NULL REFERENCES conversations ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    INDEX (created_at DESC)
);

ALTER TABLE conversations ADD CONSTRAINT fk_last_message_id_ref_messages
FOREIGN KEY (last_message_id) REFERENCES messages (id) ON DELETE SET NULL;

INSERT INTO users (id, username) VALUES
    (1, 'john'),
    (2, 'jane');
