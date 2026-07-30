import { rm } from 'node:fs/promises';
import makeWASocket, {
  DisconnectReason,
  useMultiFileAuthState,
} from '@whiskeysockets/baileys';
import { Boom } from '@hapi/boom';
import pino from 'pino';
import { normalizeMessage } from './normalizer.js';
import { publish, fetchCommands } from './publisher.js';
import { Allowlist } from './allowlist.js';

const logger = pino({ level: process.env.LOG_LEVEL ?? 'info' });

// useMultiFileAuthState is demo-grade. Replace with an encrypted custom auth
// state before production use (doc bab 5.3).
const AUTH_DIR = process.env.AUTH_DIR ?? './.auth';
const POLL_INTERVAL_MS = Number(process.env.POLL_INTERVAL_MS ?? 3000);

const allowlist = new Allowlist();
let sock = null;
let stopping = false;
let rejectedSinceLastLog = 0;

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
    // This connector never sends messages. Read-only by design (doc bab 1.3).
    markOnlineOnConnect: false,
    syncFullHistory: true,
  });

  sock.ev.on('creds.update', saveCreds);

  sock.ev.on('connection.update', async (update) => {
    const { connection, lastDisconnect, qr } = update;

    if (qr) {
      logger.info('QR baru dibuat, menunggu pemindaian');
      await publish('connection.qr', { status: 'pairing', qr });
    }

    if (connection === 'connecting') {
      await reportConnection({ status: 'connecting' });
    }

    if (connection === 'open') {
      const selfJid = sock.user?.id ?? '';
      logger.info('tersambung ke WhatsApp');
      await reportConnection({ status: 'connected', self_jid: selfJid });
    }

    if (connection === 'close') {
      const statusCode = new Boom(lastDisconnect?.error)?.output?.statusCode;

      if (statusCode === DisconnectReason.loggedOut) {
        logger.warn('logout dari perangkat; perlu pairing ulang');
        await reportConnection({ status: 'logged_out' });
        await clearAuth();
        if (!stopping) restart();
        return;
      }

      await reportConnection({ status: 'disconnected', detail: String(statusCode ?? '') });
      if (!stopping) restart();
    }
  });

  sock.ev.on('messaging-history.set', ({ messages, isLatest }) => {
    logger.info({ count: messages.length, isLatest }, 'history sync');
    for (const m of messages) {
      forwardIfAllowed('message.ingested', m.key?.remoteJid, () => normalizeMessage(m));
    }
  });

  // messages.upsert delivers an ARRAY; every element must be processed (doc 5.2).
  sock.ev.on('messages.upsert', ({ messages }) => {
    for (const m of messages) {
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

  // chats.update and contacts.update are not forwarded: they cover every chat on
  // the device, including ones outside the allowlist.
}

function restart() {
  setTimeout(() => {
    startSocket().catch((err) => logger.error({ err: err.message }, 'gagal restart'));
  }, 2000);
}

async function clearAuth() {
  try {
    await rm(AUTH_DIR, { recursive: true, force: true });
    logger.info('auth state dihapus');
  } catch (err) {
    logger.error({ err: err.message }, 'gagal menghapus auth state');
  }
}

async function handlePairRequest() {
  logger.info('QR baru diminta dari dashboard');

  if (sock) {
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
