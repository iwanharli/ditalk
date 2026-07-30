/**
 * Local allowlist filter.
 *
 * This is the first of two gates. Dropping non-allowlisted chats here means
 * their content never leaves the connector process at all, so the machine only
 * ever reads the numbers the owner registered. The backend re-checks
 * independently, because a filter that exists in only one place is one bug away
 * from leaking.
 */

const DEFAULT_COUNTRY_CODE = '62';

/** Mirrors backend waid.NormalizePhone. */
export function normalizePhone(input) {
  if (!input) return null;

  let digits = '';
  for (const ch of String(input).trim()) {
    if (ch >= '0' && ch <= '9') digits += ch;
    else if (!'+ -().'.includes(ch)) return null;
  }
  if (!digits) return null;

  if (digits.startsWith('00')) digits = digits.slice(2);
  else if (digits.startsWith('0')) digits = DEFAULT_COUNTRY_CODE + digits.slice(1);

  return digits.length >= 8 ? digits : null;
}

/** Mirrors backend waid.PhoneFromJID. */
export function phoneFromJid(jid) {
  if (!jid) return null;
  const s = String(jid).trim();

  const at = s.lastIndexOf('@');
  if (at < 0) return normalizePhone(s);

  const domain = s.slice(at);
  // A @lid holds an opaque linked id, not a phone number.
  if (domain !== '@s.whatsapp.net') return null;

  let local = s.slice(0, at);
  const colon = local.indexOf(':');
  if (colon >= 0) local = local.slice(0, colon);
  const underscore = local.indexOf('_');
  if (underscore >= 0) local = local.slice(0, underscore);

  if (!/^\d{8,}$/.test(local)) return null;
  return local;
}

export function isGroup(jid) {
  return typeof jid === 'string' && jid.endsWith('@g.us');
}

export class Allowlist {
  constructor() {
    this.phones = new Set();
    this.version = -1;
    this.loaded = false;
  }

  replace(phones, version) {
    this.phones = new Set((phones ?? []).filter(Boolean));
    this.version = version ?? this.version;
    this.loaded = true;
  }

  get size() {
    return this.phones.size;
  }

  /**
   * Returns { allowed, reason }. Fails closed: anything not positively matched
   * to an active allowlisted contact is rejected, including while the allowlist
   * has not loaded yet.
   */
  check(jid) {
    if (!this.loaded) return { allowed: false, reason: 'allowlist_not_loaded' };
    if (isGroup(jid)) return { allowed: false, reason: 'group_chat' };

    const phone = phoneFromJid(jid);
    if (!phone) return { allowed: false, reason: 'unsupported_jid' };
    if (!this.phones.has(phone)) return { allowed: false, reason: 'not_allowlisted' };

    return { allowed: true, reason: null };
  }
}
