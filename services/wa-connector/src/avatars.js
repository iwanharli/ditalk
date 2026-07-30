/**
 * Fetches profile pictures for contacts, one at a time and only when needed.
 *
 * SCOPE: only contacts the user actually selected (allowlisted). Fetching for
 * every discovered chat would mean hundreds of requests to WhatsApp on an
 * unofficial client — a realistic way to get an account restricted — and it
 * would download photographs of people the user never chose to analyze, which
 * is exactly what the allowlist exists to prevent.
 *
 * Pictures are held in memory only, here and in the backend. They are never
 * written to disk or to the database.
 */

import pino from 'pino';

const logger = pino({ level: process.env.LOG_LEVEL ?? 'info' });

const MAX_AVATAR_BYTES = 512 * 1024;
// One request at a time with a gap between them. Profile pictures are cosmetic;
// there is no reason to burst.
const REQUEST_GAP_MS = Number(process.env.AVATAR_GAP_MS ?? 800);
// Re-check a contact at most this often, so a picture change is eventually
// picked up without polling WhatsApp continuously.
const REFRESH_AFTER_MS = 6 * 60 * 60 * 1000;
// A failed attempt is retried far sooner. Treating a transient failure as final
// would leave a contact without a picture for hours: the socket is assigned
// before it finishes authenticating, so the first attempts can fail purely
// because the connection was not ready yet.
const RETRY_AFTER_ERROR_MS = 60 * 1000;

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

export class AvatarFetcher {
  constructor(sockProvider) {
    this.sockProvider = sockProvider;
    /** @type {Map<string, {at: number, hasPicture: boolean}>} */
    this.attempted = new Map();
    this.running = false;
  }

  /** Drops contacts that are no longer allowlisted so they get re-fetched if re-added. */
  retainOnly(phones) {
    const keep = new Set(phones);
    for (const phone of [...this.attempted.keys()]) {
      if (!keep.has(phone)) this.attempted.delete(phone);
    }
  }

  needsFetch(phone) {
    const seen = this.attempted.get(phone);
    if (!seen) return true;
    const wait = seen.settled ? REFRESH_AFTER_MS : RETRY_AFTER_ERROR_MS;
    return Date.now() - seen.at > wait;
  }

  /**
   * Fetches any missing pictures and hands them to `publishOne`.
   * Returns the number published. Safe to call repeatedly; only one pass runs
   * at a time.
   */
  async run(phones, publishOne) {
    if (this.running) return 0;
    const sock = this.sockProvider();
    if (!sock) return 0;

    this.running = true;
    let published = 0;

    try {
      for (const phone of phones) {
        if (!this.needsFetch(phone)) continue;

        const { status, avatar, mime } = await this.fetchOne(sock, phone);

        // 'settled' means WhatsApp answered: either a picture, or a definite
        // "there is none". Only then does the long refresh interval apply.
        this.attempted.set(phone, {
          at: Date.now(),
          settled: status === 'ok' || status === 'none',
        });

        if (status === 'ok') {
          await publishOne(phone, avatar, mime);
          published++;
        }
        await sleep(REQUEST_GAP_MS);
      }
    } finally {
      this.running = false;
    }

    return published;
  }

  /** Returns { status: 'ok' | 'none' | 'error', avatar?, mime? }. */
  async fetchOne(sock, phone) {
    const jid = `${phone}@s.whatsapp.net`;
    try {
      const url = await sock.profilePictureUrl(jid, 'image');
      // WhatsApp answered that there is no picture, or privacy hides it.
      if (!url) return { status: 'none' };

      const res = await fetch(url);
      if (!res.ok) return { status: 'error' };

      const buf = Buffer.from(await res.arrayBuffer());
      if (buf.byteLength > MAX_AVATAR_BYTES) return { status: 'none' };

      return {
        status: 'ok',
        avatar: buf.toString('base64'),
        mime: res.headers.get('content-type') ?? 'image/jpeg',
      };
    } catch (err) {
      // 401/404 from WhatsApp means no accessible picture; anything else is
      // likely transient, such as the socket not being ready yet. The number
      // must not reach the log.
      const code = err?.output?.statusCode;
      const definite = code === 401 || code === 404;
      logger.debug({ err: err.message, code }, 'foto profil kontak tidak terambil');
      return { status: definite ? 'none' : 'error' };
    }
  }
}
