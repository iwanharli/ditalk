-- +goose Up
-- Deduplicate the same message arriving through two different paths.
--
-- Import derives a synthetic id from message content because an exported chat
-- carries no ids, while live sync uses the real WhatsApp id. The same message
-- therefore lands twice whenever an import covers a period already synced live.
--
-- content_hash gives both paths one identity: the same message produces the same
-- hash regardless of which path stored it.

ALTER TABLE messages ADD COLUMN content_hash text;

-- Partial, so existing rows without a hash do not collide with each other.
CREATE UNIQUE INDEX idx_messages_content_hash
    ON messages (conversation_id, content_hash)
    WHERE content_hash IS NOT NULL;

-- Clean what the two-path problem already produced.
--
-- A message that failed to decrypt is stored with no text at all. It looks like
-- a real message to analysis while carrying nothing, and it also defeats
-- content-based deduplication, so it is removed.
DELETE FROM messages
WHERE text_cipher IS NULL
  AND caption_cipher IS NULL
  AND NOT is_deleted
  AND message_type = 'text';

-- Where an imported row and a live row describe the same moment from the same
-- side, keep the live one: it carries the real WhatsApp id, which edits,
-- reactions, and deletions refer to.
DELETE FROM messages i
WHERE i.wa_message_id LIKE 'import:%'
  AND EXISTS (
    SELECT 1 FROM messages l
    WHERE l.conversation_id = i.conversation_id
      AND l.timestamp = i.timestamp
      AND l.sender_role = i.sender_role
      AND l.wa_message_id NOT LIKE 'import:%'
      AND l.text_cipher IS NOT NULL
  );

-- +goose Down
DROP INDEX IF EXISTS idx_messages_content_hash;
ALTER TABLE messages DROP COLUMN IF EXISTS content_hash;
