import pino from 'pino';

const logger = pino({ level: process.env.LOG_LEVEL ?? 'info' });
const BACKEND_URL = process.env.BACKEND_URL ?? 'http://localhost:8080';
const INTERNAL_TOKEN = process.env.INTERNAL_TOKEN ?? '';

if (!INTERNAL_TOKEN) {
  logger.warn('INTERNAL_TOKEN kosong; backend akan menolak semua event');
}

function authHeaders() {
  return {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${INTERNAL_TOKEN}`,
  };
}

/** Sends an event to the backend. Returns the parsed body, or null on failure. */
export async function publish(event, payload) {
  try {
    const res = await fetch(`${BACKEND_URL}/internal/events`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ event, payload }),
    });

    if (!res.ok) {
      logger.warn({ event, status: res.status }, 'publish ditolak');
      return null;
    }
    return await res.json();
  } catch (err) {
    // Message content must never reach logs (doc bab 24.2).
    logger.error({ event, err: err.message }, 'publish gagal');
    return null;
  }
}

/**
 * Polls the backend for the current allowlist and any pending instruction.
 * Control flows in the same direction as events because the backend cannot call
 * into this process.
 */
export async function fetchCommands() {
  try {
    const res = await fetch(`${BACKEND_URL}/internal/commands`, {
      headers: authHeaders(),
    });
    if (!res.ok) {
      logger.warn({ status: res.status }, 'gagal mengambil command');
      return null;
    }
    return await res.json();
  } catch (err) {
    logger.error({ err: err.message }, 'gagal menghubungi backend');
    return null;
  }
}
