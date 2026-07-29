import pino from 'pino';

const logger = pino({ level: process.env.LOG_LEVEL ?? 'info' });
const BACKEND_URL = process.env.BACKEND_URL ?? 'http://localhost:8080';

export async function publish(event, payload) {
  try {
    const res = await fetch(`${BACKEND_URL}/internal/events`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ event, payload }),
    });
    if (!res.ok) logger.warn({ event, status: res.status }, 'publish rejected');
  } catch (err) {
    // Message content must never reach logs (doc 24.2).
    logger.error({ event, err: err.message }, 'publish failed');
  }
}
