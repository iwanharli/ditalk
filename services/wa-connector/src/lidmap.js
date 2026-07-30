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

  /** The LID digits paired with a phone number, or null. */
  lidForPhone(phone) {
    for (const [lid, mapped] of this.byLid.entries()) {
      if (mapped === phone) return lid;
    }
    return null;
  }

  /** Whether a phone number already has a LID pairing. */
  hasPhone(phone) {
    return this.lidForPhone(phone) !== null;
  }

  /**
   * The JID WhatsApp itself uses for a conversation with this number.
   *
   * Requests that name a chat — history on demand, for instance — must use the
   * form WhatsApp knows it by. Asking about 628…@s.whatsapp.net for a chat the
   * server tracks as …@lid simply matches nothing, with no error to show for it.
   */
  chatJidForPhone(phone) {
    const lid = this.lidForPhone(phone);
    return lid ? `${lid}@lid` : `${phone}@s.whatsapp.net`;
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
