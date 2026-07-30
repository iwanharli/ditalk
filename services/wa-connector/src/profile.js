import pino from 'pino';

const logger = pino({ level: process.env.LOG_LEVEL ?? 'info' });

// A profile picture is a small JPEG. The cap guards against an unexpectedly
// large response filling memory.
const MAX_AVATAR_BYTES = 512 * 1024;

/**
 * Reads the linked account's own display name and picture.
 *
 * The image bytes are downloaded here rather than passing WhatsApp's CDN URL to
 * the browser: that would make the dashboard issue a request to WhatsApp on every
 * page load, and the URL expires anyway.
 *
 * Every field is optional. A missing name or a privacy setting that hides the
 * picture must not break pairing.
 */
export async function readOwnProfile(sock) {
  const profile = { name: sock?.user?.name ?? sock?.user?.verifiedName ?? '' };

  const jid = sock?.user?.id;
  if (!jid) return profile;

  try {
    const url = await sock.profilePictureUrl(jid, 'image');
    if (!url) return profile;

    const res = await fetch(url);
    if (!res.ok) {
      logger.debug({ status: res.status }, 'gagal mengunduh foto profil');
      return profile;
    }

    const buf = Buffer.from(await res.arrayBuffer());
    if (buf.byteLength > MAX_AVATAR_BYTES) {
      logger.debug({ bytes: buf.byteLength }, 'foto profil terlalu besar, dilewati');
      return profile;
    }

    profile.avatar = buf.toString('base64');
    profile.avatarMime = res.headers.get('content-type') ?? 'image/jpeg';
  } catch (err) {
    // No picture set, or the account hides it. Not an error worth surfacing.
    logger.debug({ err: err.message }, 'foto profil tidak tersedia');
  }

  return profile;
}
