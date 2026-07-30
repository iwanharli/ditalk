import { mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import makeWASocket, {
  Browsers,
  DisconnectReason,
  useMultiFileAuthState,
} from '@whiskeysockets/baileys';
import { Boom } from '@hapi/boom';
import pino from 'pino';
import { normalizeMessage } from './normalizer.js';
import { publish, fetchCommands } from './publisher.js';
import { Allowlist } from './allowlist.js';
import { readOwnProfile } from './profile.js';
import { ContactDirectory } from './contacts.js';
import { AvatarFetcher } from './avatars.js';

const logger = pino({ level: process.env.LOG_LEVEL ?? 'info' });

// useMultiFileAuthState is demo-grade. Replace with an encrypted custom auth
// state before production use (doc bab 5.3).
const AUTH_DIR = process.env.AUTH_DIR ?? './.auth';
const POLL_INTERVAL_MS = Number(process.env.POLL_INTERVAL_MS ?? 3000);

// Kept beside the auth state so unlinking wipes both in one step.
const DIRECTORY_FILE = join(dirname(AUTH_DIR), '.wa-chats.json');

const allowlist = new Allowlist();
const directory = new ContactDirectory();
let sock = null;
// The socket object exists before it finishes authenticating, so anything that
// queries WhatsApp must wait for 'open' rather than merely for sock to be set.
let socketOpen = false;
const avatars = new AvatarFetcher(() => (socketOpen ? sock : null));
let stopping = false;
let rejectedSinceLastLog = 0;

// A session that WhatsApp refuses is rejected the same way every time, so
// retrying forever produces a silent loop with no QR and no explanation. After
// this many consecutive failures without ever reaching 'open', the connector
// reports the problem and stops instead of hiding it.
const MAX_LOGIN_FAILURES = 5;
let consecutiveFailures = 0;

async function reportConnection(fields) {
  await publish('connection.update', fields);
}

/**
 * Forwards a message only if its chat belongs to an active allowlisted contact.
 * Non-allowlisted content is dropped here, before it leaves this process.
 */
function forwardIfAllowed(event, jid, buildPayload) {
  const { allowed, reason } = allowlist.check(jid);
  if (!allowed) {
    rejectedSinceLastLog++;
    // Log the reason only, never the JID or content.
    logger.debug({ event, reason }, 'pesan dibuang oleh allowlist');
    return;
  }
  publish(event, buildPayload());
}

async function startSocket() {
  const { state, saveCreds } = await useMultiFileAuthState(AUTH_DIR);

  sock = makeWASocket({
    auth: state,
    logger,

    // Read-only by design (doc bab 1.3). Beyond never calling a send API, this
    // also keeps the account from appearing online and stops WhatsApp from
    // treating the link as an actively-reading client: with presence set to
    // unavailable, incoming receipts are typed 'inactive' rather than active,
    // and no read receipt (blue ticks) is ever sent. See baileys-features.md.
    markOnlineOnConnect: false,

    syncFullHistory: true,

    // Deliberately the Baileys default, despite the documented advice that a
    // desktop descriptor is needed for a full history sync.
    //
    // Measured against WhatsApp on 30 July 2026 with fresh auth, only this
    // descriptor is accepted at all. Every desktop descriptor is closed
    // immediately and never even produces a QR:
    //
    //   ["Ubuntu","Chrome"]   -> QR emitted, pairing works
    //   ["Mac OS","Desktop"]  -> connection closed, no QR
    //   ["Mac OS","Safari"]   -> connection closed, no QR
    //   ["Mac OS","Chrome"]   -> connection closed, no QR
    //   ["Windows","Desktop"] -> connection closed, no QR
    //
    // So webSubPlatform stays WEB_BROWSER and history is whatever that yields.
    // See baileys-features.md section 2 before changing this.
    browser: Browsers.ubuntu('Chrome'),
  });

  sock.ev.on('creds.update', saveCreds);

  sock.ev.on('connection.update', async (update) => {
    const { connection, lastDisconnect, qr } = update;

    if (qr) {
      // Receiving a QR means WhatsApp is talking to us and pairing is possible,
      // so this is not a rejected session. QR refresh cycles also emit 'close',
      // and counting those would exhaust the budget before the user can scan.
      consecutiveFailures = 0;
      logger.info('QR baru dibuat, menunggu pemindaian');
      await publish('connection.qr', { status: 'pairing', qr });
    }

    if (connection === 'connecting') {
      await reportConnection({ status: 'connecting' });
    }

    if (connection === 'open') {
      const selfJid = sock.user?.id ?? '';
      socketOpen = true;
      consecutiveFailures = 0;
      logger.info('tersambung ke WhatsApp');

      const profile = await readOwnProfile(sock);
      await reportConnection({
        status: 'connected',
        self_jid: selfJid,
        self_name: profile.name,
        avatar: profile.avatar,
        avatar_mime: profile.avatarMime,
      });
    }

    if (connection === 'close') {
      socketOpen = false;
      const statusCode = new Boom(lastDisconnect?.error)?.output?.statusCode;

      if (statusCode === DisconnectReason.loggedOut) {
        logger.warn('logout dari perangkat; perlu pairing ulang');
        await reportConnection({ status: 'logged_out' });
        await clearAuth();
        if (!stopping) restart();
        return;
      }

      consecutiveFailures++;
      if (consecutiveFailures >= MAX_LOGIN_FAILURES) {
        logger.error(
          { failures: consecutiveFailures },
          'sesi ditolak berulang kali; perlu pairing ulang',
        );
        await reportConnection({
          status: 'error',
          detail:
            'Sesi ditolak WhatsApp. Tekan "Lepas perangkat" lalu pindai QR baru untuk menautkan ulang.',
        });
        return;
      }

      await reportConnection({ status: 'disconnected', detail: String(statusCode ?? '') });
      if (!stopping) restart();
    }
  });

  sock.ev.on('messaging-history.set', ({ chats, contacts, messages, isLatest, syncType }) => {
    // syncType and named counts matter for diagnosing a nameless contact list:
    // display names arrive almost entirely via the PUSH_NAME sync, while RECENT
    // and FULL syncs derive a name from chat.name, which is normally empty for
    // one-to-one chats. Counts only — no names or numbers reach the log.
    logger.info(
      {
        syncType,
        messages: messages.length,
        chats: chats?.length ?? 0,
        contacts: contacts?.length ?? 0,
        named: (contacts ?? []).filter((c) => c.name || c.notify || c.verifiedName).length,
        isLatest,
      },
      'history sync',
    );

    // Metadata for the picker. Names and numbers only; see contacts.js.
    for (const c of contacts ?? []) directory.noteContact(c);
    for (const c of chats ?? []) directory.noteChat(c);

    for (const m of messages) {
      directory.noteMessageActivity(m);
      forwardIfAllowed('message.ingested', m.key?.remoteJid, () => normalizeMessage(m));
    }
  });

  // messages.upsert delivers an ARRAY; every element must be processed (doc 5.2).
  sock.ev.on('messages.upsert', ({ messages }) => {
    for (const m of messages) {
      // Note the conversation before the allowlist check: a dropped message
      // still proves the chat exists, which is what the picker needs. Only the
      // JID, timestamp, and pushName are taken — never the content.
      directory.noteMessageActivity(m);
      forwardIfAllowed('message.ingested', m.key?.remoteJid, () => normalizeMessage(m));
    }
  });

  sock.ev.on('messages.update', (updates) => {
    for (const u of updates) {
      forwardIfAllowed('message.updated', u.key?.remoteJid, () => ({
        conversation_id: u.key?.remoteJid ?? null,
        message_id: u.key?.id ?? null,
        update: u.update ?? null,
      }));
    }
  });

  sock.ev.on('messages.delete', (payload) => {
    const jid = payload?.keys?.[0]?.remoteJid ?? payload?.jid;
    forwardIfAllowed('message.deleted', jid, () => ({
      conversation_id: jid ?? null,
      keys: payload?.keys ?? null,
    }));
  });

  sock.ev.on('messages.reaction', (reactions) => {
    for (const rct of reactions) {
      forwardIfAllowed('message.reaction', rct.key?.remoteJid, () => ({
        conversation_id: rct.key?.remoteJid ?? null,
        message_id: rct.key?.id ?? null,
        reaction: rct.reaction ?? null,
      }));
    }
  });

  // chats.* and contacts.* feed the picker only. Their message content is never
  // forwarded; the allowlist still governs everything that gets analyzed.
  sock.ev.on('chats.upsert', (chats) => {
    for (const c of chats) directory.noteChat(c);
  });
  sock.ev.on('chats.update', (chats) => {
    for (const c of chats) directory.noteChat(c);
  });
  sock.ev.on('contacts.upsert', (contacts) => {
    for (const c of contacts) directory.noteContact(c);
  });
  sock.ev.on('contacts.update', (contacts) => {
    for (const c of contacts) directory.noteContact(c);
  });
}

function restart() {
  setTimeout(() => {
    startSocket().catch((err) => logger.error({ err: err.message }, 'gagal restart'));
  }, 2000);
}

async function clearAuth() {
  try {
    await rm(AUTH_DIR, { recursive: true, force: true });
    // The chat directory describes people who were never allowlisted; unlinking
    // must not leave it behind.
    await rm(DIRECTORY_FILE, { force: true });
    logger.info('auth state dan daftar chat dihapus');
  } catch (err) {
    logger.error({ err: err.message }, 'gagal menghapus auth state');
  }
}

async function loadDirectory() {
  try {
    const raw = await readFile(DIRECTORY_FILE, 'utf8');
    if (directory.loadFrom(JSON.parse(raw))) {
      logger.info({ count: directory.size }, 'daftar chat dimuat dari cache');
    }
  } catch (err) {
    if (err.code !== 'ENOENT') {
      logger.warn({ err: err.message }, 'cache daftar chat tidak terbaca');
    }
  }
}

async function saveDirectory() {
  try {
    await mkdir(dirname(DIRECTORY_FILE), { recursive: true });
    await writeFile(DIRECTORY_FILE, JSON.stringify(directory.toJSON()), 'utf8');
  } catch (err) {
    logger.warn({ err: err.message }, 'gagal menyimpan daftar chat');
  }
}

async function handlePairRequest() {
  logger.info('QR baru diminta dari dashboard');

  if (sock) {
    socketOpen = false;
    try {
      // end() drops the socket without logging the device out, so an existing
      // link survives if the user only wanted to refresh a stale code.
      sock.end(undefined);
    } catch {
      // Already closed.
    }
    sock = null;
  }

  await startSocket().catch((err) =>
    logger.error({ err: err.message }, 'gagal memulai socket untuk pairing'),
  );
}

async function handleLogout() {
  logger.warn('logout diminta dari dashboard');
  // Unlinking is the remedy for a rejected session, so the budget starts over.
  consecutiveFailures = 0;
  try {
    await sock?.logout();
  } catch {
    // Already disconnected; clearing local state is what matters.
  }
  await clearAuth();
  await reportConnection({ status: 'logged_out' });
  restart();
}

/**
 * Polls the backend for the allowlist and pending commands. The allowlist is
 * refreshed on every tick so removing a number takes effect within seconds.
 */
async function pollLoop() {
  while (!stopping) {
    const cmd = await fetchCommands();

    if (cmd) {
      const before = allowlist.size;
      allowlist.replace(cmd.allowed_phones, cmd.allowlist_version);
      if (allowlist.size !== before) {
        logger.info({ count: allowlist.size }, 'allowlist diperbarui');
      }

      if (rejectedSinceLastLog > 0) {
        logger.info({ dropped: rejectedSinceLastLog }, 'pesan di luar allowlist dibuang');
        rejectedSinceLastLog = 0;
      }

      // Only publish when something changed; history sync produces long bursts.
      if (directory.dirty) {
        directory.dirty = false;
        await publish('contacts.discovered', { contacts: directory.snapshot() });
        await saveDirectory();
        logger.info({ count: directory.size }, 'kandidat kontak dikirim');
      }

      // Profile pictures only for contacts the user selected. See avatars.js for
      // why this is not done for every discovered chat.
      const selected = cmd.allowed_phones ?? [];
      avatars.retainOnly(selected);
      if (selected.length > 0) {
        void avatars
          .run(selected, (phone, avatar, mime) =>
            publish('contact.avatar', { phone, avatar, avatar_mime: mime }),
          )
          .then((n) => {
            if (n > 0) logger.info({ count: n }, 'foto profil kontak dikirim');
          })
          .catch((err) => logger.warn({ err: err.message }, 'gagal mengambil foto kontak'));
      }

      if (cmd.logout) await handleLogout();
      // Force a fresh socket even when one exists: a socket that is present but
      // stuck disconnected would otherwise never emit a new QR.
      if (cmd.pair) await handlePairRequest();
    }

    await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
  }
}

/**
 * Waits for the backend before opening the socket.
 *
 * The allowlist must be loaded first: check() fails closed, so starting early
 * would silently discard a history sync we are actually allowed to read. When
 * both processes start together the backend often is not listening yet, so this
 * retries instead of giving up on the first attempt.
 */
async function loadInitialAllowlist(attempts = 20, delayMs = 1000) {
  for (let i = 1; i <= attempts; i++) {
    const cmd = await fetchCommands();
    if (cmd) {
      allowlist.replace(cmd.allowed_phones, cmd.allowlist_version);
      logger.info({ count: allowlist.size }, 'allowlist awal dimuat');
      return true;
    }
    if (i === 1) logger.info('menunggu backend siap');
    await new Promise((resolve) => setTimeout(resolve, delayMs));
  }
  return false;
}

async function main() {
  await loadDirectory();
  // Force a publish on the first poll so the dashboard has the cached list even
  // when WhatsApp sends no history sync on this reconnect.
  if (directory.size > 0) directory.dirty = true;

  if (!(await loadInitialAllowlist())) {
    logger.warn(
      'backend tidak dapat dihubungi; pesan akan dibuang sampai allowlist termuat',
    );
  }

  pollLoop().catch((err) => logger.error({ err: err.message }, 'poll loop berhenti'));
  await startSocket();
}

for (const sig of ['SIGINT', 'SIGTERM']) {
  process.on(sig, () => {
    stopping = true;
    logger.info('menghentikan connector');
    process.exit(0);
  });
}

main().catch((err) => {
  logger.error({ err: err.message }, 'connector gagal');
  process.exit(1);
});
