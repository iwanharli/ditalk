/**
 * Builds the list of contacts the user can choose to analyze.
 *
 * PRIVACY: this is the one path that deliberately looks past the allowlist. The
 * user cannot pick which numbers to register without seeing what exists, so a
 * candidate list is unavoidable — but it is kept to the minimum that makes a
 * choice possible:
 *
 *   - name, phone number, and last activity timestamp only
 *   - never message content, previews, or media
 *   - one-to-one chats only; groups can never be analyzed anyway
 *   - held in memory here and in the backend, never written to the database
 *
 * Everything else about a non-allowlisted chat is still dropped as before.
 */

import { phoneFromJid } from './allowlist.js';

// Enough to cover an active account without shipping an unbounded list.
const MAX_CANDIDATES = 300;

export class ContactDirectory {
  constructor() {
    /**
     * Only chats become candidates. The address book is kept separately and used
     * solely to put a name on a chat that already exists.
     *
     * A synced address book is far larger than the set of people actually
     * messaged — on a real account, hundreds of entries against a few dozen
     * conversations. Listing all of them would both exceed what the user asked
     * for and expose people they have never talked to.
     */
    /** @type {Map<string, {phone: string, name: string, lastAt: number|null}>} */
    this.chats = new Map();
    /** @type {Map<string, string>} */
    this.names = new Map();
    this.dirty = false;
  }

  /** Remembers a display name. Does not, on its own, create a candidate. */
  noteContact(contact) {
    const phone = phoneFromJid(contact?.id);
    if (!phone) return;

    const name =
      contact.name?.trim() ||
      contact.notify?.trim() ||
      contact.verifiedName?.trim() ||
      '';
    if (!name) return;

    // A saved contact name beats a pushName picked up from a message.
    const existing = this.names.get(phone) ?? '';
    if (name.length <= existing.length) return;

    this.names.set(phone, name);
    if (this.chats.has(phone)) this.dirty = true;
  }

  /** Records an actual conversation. This is what makes a contact a candidate. */
  noteChat(chat) {
    const phone = phoneFromJid(chat?.id);
    if (!phone) return;

    const ts = Number(chat.conversationTimestamp ?? 0);
    const lastAt = Number.isFinite(ts) && ts > 0 ? ts : null;

    const existing = this.chats.get(phone);
    if (!existing) {
      this.chats.set(phone, { phone, name: chat.name?.trim() || '', lastAt });
      this.dirty = true;
      return;
    }

    if (lastAt && (existing.lastAt === null || lastAt > existing.lastAt)) {
      existing.lastAt = lastAt;
      this.dirty = true;
    }
    if (!existing.name && chat.name?.trim()) {
      existing.name = chat.name.trim();
      this.dirty = true;
    }
  }

  /**
   * Records a conversation from a message.
   *
   * The existence of a message for a JID is the most reliable evidence that a
   * conversation exists: WhatsApp sends the full history sync only once per
   * pairing, and offline message delivery arrives through messages.upsert without
   * a matching chats.* event, which left the picker empty.
   *
   * Only the JID, timestamp, and pushName are read. Message content is never
   * touched here, and the allowlist still decides what gets analyzed.
   */
  noteMessageActivity(msg) {
    const phone = phoneFromJid(msg?.key?.remoteJid);
    if (!phone) return;

    const ts = Number(msg.messageTimestamp ?? 0);
    // pushName is the sender's own display name, so it only names the other
    // party on incoming messages.
    const name = msg.key?.fromMe ? '' : msg.pushName?.trim() || '';

    this.noteChat({
      id: msg.key.remoteJid,
      name,
      conversationTimestamp: Number.isFinite(ts) && ts > 0 ? ts : 0,
    });
  }

  get size() {
    return this.chats.size;
  }

  /**
   * WhatsApp sends the full history sync only once per pairing. Without a cache
   * the picker would be empty after every restart, so the directory is persisted
   * beside the connector's own auth state rather than in the analysis database:
   * these people are not allowlisted, so the app has no basis to keep records
   * about them where analysis data lives. Logout deletes both.
   */
  toJSON() {
    return {
      version: 1,
      chats: [...this.chats.values()],
      names: [...this.names.entries()],
    };
  }

  loadFrom(data) {
    if (!data || data.version !== 1) return false;

    for (const c of data.chats ?? []) {
      if (typeof c?.phone === 'string') {
        this.chats.set(c.phone, {
          phone: c.phone,
          name: typeof c.name === 'string' ? c.name : '',
          lastAt: typeof c.lastAt === 'number' ? c.lastAt : null,
        });
      }
    }
    for (const [phone, name] of data.names ?? []) {
      if (typeof phone === 'string' && typeof name === 'string') {
        this.names.set(phone, name);
      }
    }
    return this.chats.size > 0;
  }

  /** Most recently active first; chats without a timestamp fall to the end. */
  snapshot() {
    return [...this.chats.values()]
      .sort((a, b) => (b.lastAt ?? 0) - (a.lastAt ?? 0))
      .slice(0, MAX_CANDIDATES)
      .map(({ phone, name, lastAt }) => ({
        phone,
        name: name || this.names.get(phone) || '',
        last_message_at: lastAt ? new Date(lastAt * 1000).toISOString() : null,
      }));
  }
}
