-- +goose Up
-- View-once messages are counted as skipped rather than stored.
--
-- The concept document is explicit that view-once content is "tidak disimpan atau
-- dianalisis secara permanen secara default" (Ringkasan Eksekutif, and bab 7
-- media policy). The sender chose a message that disappears; persisting it would
-- override that choice. The counter still exists so the user can see it happened.

ALTER TABLE ingest_rejections DROP CONSTRAINT ingest_rejections_reason_check;

ALTER TABLE ingest_rejections ADD CONSTRAINT ingest_rejections_reason_check
    CHECK (reason IN (
        'not_allowlisted','group_chat','unsupported_jid','inactive_contact',
        'view_once'));

-- +goose Down
DELETE FROM ingest_rejections WHERE reason = 'view_once';

ALTER TABLE ingest_rejections DROP CONSTRAINT ingest_rejections_reason_check;

ALTER TABLE ingest_rejections ADD CONSTRAINT ingest_rejections_reason_check
    CHECK (reason IN (
        'not_allowlisted','group_chat','unsupported_jid','inactive_contact'));
