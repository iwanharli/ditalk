import makeWASocket, {
  DisconnectReason,
  useMultiFileAuthState,
} from '@whiskeysockets/baileys';
import { Boom } from '@hapi/boom';
import pino from 'pino';
import qrcode from 'qrcode-terminal';
import { normalizeMessage } from './normalizer.js';
import { publish } from './publisher.js';

const logger = pino({ level: process.env.LOG_LEVEL ?? 'info' });

// useMultiFileAuthState is a demo-grade auth store. Replace with an encrypted
// custom auth state before production use (see doc 5.3).
const AUTH_DIR = process.env.AUTH_DIR ?? './.auth';

async function start() {
  const { state, saveCreds } = await useMultiFileAuthState(AUTH_DIR);

  const sock = makeWASocket({
    auth: state,
    logger,
    // This connector never sends messages. Read-only by design (doc 1.3).
    markOnlineOnConnect: false,
    syncFullHistory: true,
  });

  sock.ev.on('creds.update', saveCreds);

  sock.ev.on('connection.update', (update) => {
    const { connection, lastDisconnect, qr } = update;

    if (qr) qrcode.generate(qr, { small: true });

    if (connection === 'close') {
      const statusCode = new Boom(lastDisconnect?.error)?.output?.statusCode;
      if (statusCode === DisconnectReason.loggedOut) {
        logger.warn('logged out; re-pairing required');
        return;
      }
      logger.info('connection closed, reconnecting');
      start();
    }

    if (connection === 'open') logger.info('connected to WhatsApp');
  });

  sock.ev.on('messaging-history.set', ({ messages, isLatest }) => {
    logger.info({ count: messages.length, isLatest }, 'history sync');
    for (const m of messages) publish('message.ingested', normalizeMessage(m));
  });

  // messages.upsert delivers an ARRAY; every element must be processed (doc 5.2).
  sock.ev.on('messages.upsert', ({ messages }) => {
    for (const m of messages) publish('message.ingested', normalizeMessage(m));
  });

  sock.ev.on('messages.update', (updates) => {
    for (const u of updates) publish('message.updated', u);
  });

  sock.ev.on('messages.delete', (payload) => {
    publish('message.deleted', payload);
  });

  sock.ev.on('messages.reaction', (reactions) => {
    for (const r of reactions) publish('message.reaction', r);
  });

  sock.ev.on('chats.update', (chats) => publish('chats.updated', chats));
  sock.ev.on('contacts.update', (contacts) => publish('contacts.updated', contacts));
}

start().catch((err) => {
  logger.error(err, 'fatal connector error');
  process.exit(1);
});
