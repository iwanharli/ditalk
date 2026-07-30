/**
 * Maps WhatsApp LID addresses back to phone numbers.
 *
 * WhatsApp increasingly addresses a one-to-one chat by a LID
 * ("184610646409253@lid") instead of the phone JID. A LID's digits are NOT a
 * phone number, so an allowlist keyed on numbers can never match one — the chat
 * simply looks like an unsupported address and every message is dropped.
 *
 * Baileys carries the phone-number variant alongside the LID, which is the only
 * reliable way to map it back:
 *   - history contacts arrive as { id: <phone JID>, lid: <lid JID> }
 *   - incoming messages carry key.senderPn / key.participantPn
 *
 * Outgoing messages are deliberately NOT used to learn a mapping: on a message
 * you sent, senderPn is your own number, so learning from it would map the
 * contact's LID to yourself and pull your own chats into their conversation.
 */

import { phoneFromJid } from './allowlist.js';

/** Extracts the bare LID digits, or null when the value is not a LID. */
export function lidKey(jid) {
  if (typeof jid !== 'string') return null;
  const s = jid.trim();
  if (!s.endsWith('@lid')) return null;

  let local = s.slice(0, -'@lid'.length);
  const colon = local.indexOf(':');
  if (colon >= 0) local = local.slice(0, colon);

  return /^\d+$/.test(local) ? local : null;
}

export class LidMap {
  constructor() {
    /** @type {Map<string, string>} lid digits -> phone digits */
    this.byLid = new Map();
  }

  get size() {
    return this.byLid.size;
  }

  /** Records a pairing seen in history contacts. */
  note(phoneJid, lidJid) {
    const lid = lidKey(lidJid);
    const phone = phoneFromJid(phoneJid);
    if (!lid || !phone) return false;
    if (this.byLid.get(lid) === phone) return false;

    this.byLid.set(lid, phone);
    return true;
  }

  /** Learns from an incoming message whose chat is LID-addressed. */
  noteFromKey(key) {
    if (!key || key.fromMe) return false;

    const lid = lidKey(key.remoteJid);
    if (!lid) return false;

    return this.note(key.senderPn ?? key.participantPn, key.remoteJid);
  }

  /**
   * Resolves the phone number a message belongs to.
   * Returns null when the chat is a group, a broadcast, or a LID that has not
   * been mapped yet — all of which must stay rejected.
   */
  phoneForKey(key) {
    if (!key) return null;

    const direct = phoneFromJid(key.remoteJid);
    if (direct) return direct;

    const lid = lidKey(key.remoteJid);
    return lid ? (this.byLid.get(lid) ?? null) : null;
  }

  /** The canonical chat JID to report, so the backend keys on the number. */
  jidForKey(key) {
    const phone = this.phoneForKey(key);
    return phone ? `${phone}@s.whatsapp.net` : null;
  }

  toJSON() {
    return [...this.byLid.entries()];
  }

  loadFrom(entries) {
    if (!Array.isArray(entries)) return false;
    for (const pair of entries) {
      if (Array.isArray(pair) && typeof pair[0] === 'string' && typeof pair[1] === 'string') {
        this.byLid.set(pair[0], pair[1]);
      }
    }
    return this.byLid.size > 0;
  }
}
