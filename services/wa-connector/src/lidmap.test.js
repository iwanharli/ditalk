import assert from 'node:assert/strict';
import { test } from 'node:test';
import { LidMap, lidKey } from './lidmap.js';

const LID = '184610646409253@lid';
const PN = '6282258414330@s.whatsapp.net';

test('lidKey mengenali LID dan menolak yang lain', () => {
  assert.equal(lidKey(LID), '184610646409253');
  assert.equal(lidKey('184610646409253:12@lid'), '184610646409253');
  assert.equal(lidKey(PN), null);
  assert.equal(lidKey('62812-1234@g.us'), null);
  assert.equal(lidKey(''), null);
  assert.equal(lidKey(undefined), null);
});

test('memetakan LID dari kontak riwayat', () => {
  const m = new LidMap();
  assert.equal(m.note(PN, LID), true);
  assert.equal(m.phoneForKey({ remoteJid: LID }), '6282258414330');
});

test('pesan masuk pada chat LID mengajarkan pemetaan', () => {
  const m = new LidMap();
  const key = { remoteJid: LID, fromMe: false, senderPn: PN };

  assert.equal(m.noteFromKey(key), true);
  assert.equal(m.phoneForKey(key), '6282258414330');
  assert.equal(m.jidForKey(key), PN);
});

test('pesan KELUAR tidak boleh mengajarkan pemetaan', () => {
  const m = new LidMap();
  // Pada pesan yang kita kirim, senderPn adalah nomor kita sendiri. Belajar dari
  // sini akan memetakan LID lawan bicara ke nomor kita.
  const key = { remoteJid: LID, fromMe: true, senderPn: '6281249442476@s.whatsapp.net' };

  assert.equal(m.noteFromKey(key), false);
  assert.equal(m.size, 0);
  assert.equal(m.phoneForKey(key), null);
});

test('setelah dipetakan, pesan keluar ikut terselesaikan', () => {
  const m = new LidMap();
  m.noteFromKey({ remoteJid: LID, fromMe: false, senderPn: PN });

  const outgoing = { remoteJid: LID, fromMe: true, senderPn: '6281249442476@s.whatsapp.net' };
  assert.equal(m.phoneForKey(outgoing), '6282258414330', 'harus memakai peta, bukan senderPn');
});

test('JID nomor biasa tidak butuh peta', () => {
  const m = new LidMap();
  assert.equal(m.phoneForKey({ remoteJid: PN }), '6282258414330');
  assert.equal(m.phoneForKey({ remoteJid: '6282258414330:12@s.whatsapp.net' }), '6282258414330');
});

test('grup dan broadcast tetap ditolak', () => {
  const m = new LidMap();
  assert.equal(m.phoneForKey({ remoteJid: '62812-1600000000@g.us' }), null);
  assert.equal(m.phoneForKey({ remoteJid: 'status@broadcast' }), null);
  assert.equal(m.jidForKey({ remoteJid: '62812-1600000000@g.us' }), null);
});

test('LID yang belum dipetakan tetap ditolak', () => {
  const m = new LidMap();
  assert.equal(m.phoneForKey({ remoteJid: '999888777666555@lid' }), null);
});

test('chatJidForPhone memakai LID bila WhatsApp mengenalnya begitu', () => {
  const m = new LidMap();
  m.note(PN, LID);

  // Permintaan yang menyebut chat harus memakai bentuk yang dipakai server.
  // Menyebut 628...@s.whatsapp.net untuk chat yang dilacak sebagai @lid tidak
  // cocok dengan apa pun, dan gagal tanpa pesan error.
  assert.equal(m.chatJidForPhone('6282258414330'), LID);
});

test('chatJidForPhone jatuh ke JID nomor bila tidak ada pemetaan', () => {
  const m = new LidMap();
  assert.equal(m.chatJidForPhone('6289999999999'), '6289999999999@s.whatsapp.net');
});

test('lidForPhone dan hasPhone konsisten', () => {
  const m = new LidMap();
  m.note(PN, LID);

  assert.equal(m.lidForPhone('6282258414330'), '184610646409253');
  assert.equal(m.hasPhone('6282258414330'), true);
  assert.equal(m.lidForPhone('6289999999999'), null);
  assert.equal(m.hasPhone('6289999999999'), false);
});

test('bertahan lewat serialisasi', () => {
  const m = new LidMap();
  m.note(PN, LID);

  const loaded = new LidMap();
  assert.equal(loaded.loadFrom(m.toJSON()), true);
  assert.equal(loaded.phoneForKey({ remoteJid: LID }), '6282258414330');
});
