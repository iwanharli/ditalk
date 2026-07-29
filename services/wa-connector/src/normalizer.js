// Converts a raw Baileys message into the canonical message object (doc 6.1).
// Display names and JIDs are NOT resolved here; the Go backend hashes/encrypts them.

function unwrap(message) {
  if (!message) return null;
  if (message.ephemeralMessage) return unwrap(message.ephemeralMessage.message);
  if (message.viewOnceMessage) return unwrap(message.viewOnceMessage.message);
  if (message.viewOnceMessageV2) return unwrap(message.viewOnceMessageV2.message);
  if (message.documentWithCaptionMessage)
    return unwrap(message.documentWithCaptionMessage.message);
  if (message.editedMessage) return unwrap(message.editedMessage.message);
  return message;
}

function detectType(inner) {
  if (!inner) return 'unknown';
  if (inner.conversation || inner.extendedTextMessage) return 'text';
  if (inner.imageMessage) return 'image';
  if (inner.audioMessage) return 'audio';
  if (inner.videoMessage) return 'video';
  if (inner.stickerMessage) return 'sticker';
  if (inner.documentMessage) return 'document';
  return 'unknown';
}

function extractText(inner) {
  if (!inner) return null;
  return (
    inner.conversation ??
    inner.extendedTextMessage?.text ??
    inner.imageMessage?.caption ??
    inner.videoMessage?.caption ??
    inner.documentMessage?.caption ??
    null
  );
}

function isViewOnce(message) {
  return Boolean(message?.viewOnceMessage || message?.viewOnceMessageV2);
}

export function normalizeMessage(raw) {
  const inner = unwrap(raw.message);
  const contextInfo =
    inner?.extendedTextMessage?.contextInfo ?? inner?.imageMessage?.contextInfo;

  return {
    message_id: raw.key?.id ?? null,
    conversation_id: raw.key?.remoteJid ?? null,
    sender_role: raw.key?.fromMe ? 'SELF' : 'OTHER',
    timestamp: raw.messageTimestamp
      ? new Date(Number(raw.messageTimestamp) * 1000).toISOString()
      : null,
    message_type: detectType(inner),
    text: extractText(inner),
    quoted_message_id: contextInfo?.stanzaId ?? null,
    reactions: [],
    is_view_once: isViewOnce(raw.message),
    is_ephemeral: Boolean(raw.message?.ephemeralMessage),
  };
}
