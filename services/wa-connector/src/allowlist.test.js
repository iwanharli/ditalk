import assert from 'node:assert/strict';
import { test } from 'node:test';
import { Allowlist, normalizePhone, phoneFromJid, isGroup } from './allowlist.js';

test('normalizePhone matches the backend rules', () => {
  assert.equal(normalizePhone('081234567890'), '6281234567890');
  assert.equal(normalizePhone('+62 812-3456-7890'), '6281234567890');
  assert.equal(normalizePhone('(0812) 3456.7890'), '6281234567890');
  assert.equal(normalizePhone('006281234567890'), '6281234567890');
  assert.equal(normalizePhone('6281234567890'), '6281234567890');
  assert.equal(normalizePhone('+14155552671'), '14155552671');

  assert.equal(normalizePhone(''), null);
  assert.equal(normalizePhone('0812abc'), null);
  assert.equal(normalizePhone('0812'), null, 'too short');
});

test('phoneFromJid strips device suffixes and rejects non-contact JIDs', () => {
  assert.equal(phoneFromJid('6281234567890@s.whatsapp.net'), '6281234567890');
  assert.equal(phoneFromJid('6281234567890:12@s.whatsapp.net'), '6281234567890');
  assert.equal(phoneFromJid('6281234567890_1@s.whatsapp.net'), '6281234567890');

  // @lid is an opaque identifier, not a phone number.
  assert.equal(phoneFromJid('123456789012345@lid'), null);
  assert.equal(phoneFromJid('6281234567890-1234@g.us'), null);
  assert.equal(phoneFromJid('status@broadcast'), null);
  assert.equal(phoneFromJid(''), null);
});

test('isGroup detects group JIDs', () => {
  assert.equal(isGroup('62812-1234@g.us'), true);
  assert.equal(isGroup('6281234567890@s.whatsapp.net'), false);
});

test('allowlist fails closed before it has loaded', () => {
  const list = new Allowlist();
  const got = list.check('6281234567890@s.whatsapp.net');

  assert.equal(got.allowed, false);
  assert.equal(got.reason, 'allowlist_not_loaded');
});

test('allowlist accepts only registered numbers', () => {
  const list = new Allowlist();
  list.replace(['6281234567890'], 1);

  assert.equal(list.check('6281234567890@s.whatsapp.net').allowed, true);

  const other = list.check('6289999999999@s.whatsapp.net');
  assert.equal(other.allowed, false);
  assert.equal(other.reason, 'not_allowlisted');
});

test('allowlist matches regardless of device suffix', () => {
  const list = new Allowlist();
  list.replace(['6281234567890'], 1);

  assert.equal(list.check('6281234567890:47@s.whatsapp.net').allowed, true);
});

test('allowlist rejects groups even when a member is registered', () => {
  const list = new Allowlist();
  list.replace(['6281234567890'], 1);

  const got = list.check('6281234567890-1600000000@g.us');
  assert.equal(got.allowed, false, 'group must never be read');
  assert.equal(got.reason, 'group_chat');
});

test('removing a number stops it being allowed', () => {
  const list = new Allowlist();
  list.replace(['6281234567890'], 1);
  assert.equal(list.check('6281234567890@s.whatsapp.net').allowed, true);

  list.replace([], 2);
  assert.equal(list.check('6281234567890@s.whatsapp.net').allowed, false);
  assert.equal(list.size, 0);
});
