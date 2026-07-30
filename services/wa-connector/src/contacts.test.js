import assert from 'node:assert/strict';
import { test } from 'node:test';
import { ContactDirectory } from './contacts.js';

test('hanya chat yang menjadi kandidat, bukan seluruh buku alamat', () => {
  const d = new ContactDirectory();
  // Tersimpan di buku alamat tetapi belum pernah dichat.
  d.noteContact({ id: '6289999999999@s.whatsapp.net', name: 'Belum Pernah Chat' });
  d.noteChat({ id: '6281234567890@s.whatsapp.net', conversationTimestamp: 1785000000 });

  const got = d.snapshot();
  assert.equal(got.length, 1, 'buku alamat tidak boleh ikut jadi kandidat');
  assert.equal(got[0].phone, '6281234567890');
});

test('nama dari buku alamat dipakai untuk chat yang ada', () => {
  const d = new ContactDirectory();
  d.noteContact({ id: '6281234567890@s.whatsapp.net', name: 'Kakak' });
  d.noteChat({ id: '6281234567890@s.whatsapp.net', conversationTimestamp: 1785000000 });

  const got = d.snapshot();
  assert.equal(got.length, 1);
  assert.equal(got[0].phone, '6281234567890');
  assert.equal(got[0].name, 'Kakak');
});

test('grup, broadcast, dan lid diabaikan', () => {
  const d = new ContactDirectory();
  d.noteChat({ id: '6281234567890-1600000000@g.us', name: 'Grup Keluarga' });
  d.noteChat({ id: 'status@broadcast' });
  // @lid berisi identifier opaque, bukan nomor telepon.
  d.noteChat({ id: '123456789012345@lid' });

  assert.equal(d.size, 0, 'tidak ada yang boleh masuk kandidat');
});

test('nomor yang sama dari kontak dan chat tidak menggandakan baris', () => {
  const d = new ContactDirectory();
  d.noteContact({ id: '6281234567890@s.whatsapp.net', notify: 'Budi' });
  d.noteChat({ id: '6281234567890@s.whatsapp.net', conversationTimestamp: 1785000000 });
  // Device suffix menunjuk orang yang sama.
  d.noteChat({ id: '6281234567890:12@s.whatsapp.net', conversationTimestamp: 1785000100 });

  const got = d.snapshot();
  assert.equal(got.length, 1);
  assert.equal(got[0].name, 'Budi');
  assert.equal(got[0].last_message_at, new Date(1785000100 * 1000).toISOString());
});

test('nama tersimpan menang atas pushName yang lebih pendek', () => {
  const d = new ContactDirectory();
  d.noteChat({ id: '6281234567890@s.whatsapp.net', conversationTimestamp: 1785000000 });
  d.noteContact({ id: '6281234567890@s.whatsapp.net', notify: 'Bud' });
  d.noteContact({ id: '6281234567890@s.whatsapp.net', name: 'Budi Santoso' });

  assert.equal(d.snapshot()[0].name, 'Budi Santoso');
});

test('diurutkan dari aktivitas terbaru, tanpa timestamp di belakang', () => {
  const d = new ContactDirectory();
  d.noteChat({ id: '6281111111111@s.whatsapp.net', conversationTimestamp: 1785000000 });
  d.noteChat({ id: '6282222222222@s.whatsapp.net', conversationTimestamp: 1785999999 });
  // Chat ada, tetapi WhatsApp tidak menyertakan timestamp.
  d.noteChat({ id: '6283333333333@s.whatsapp.net', name: 'Tanpa Timestamp' });

  const phones = d.snapshot().map((c) => c.phone);
  assert.deepEqual(phones, ['6282222222222', '6281111111111', '6283333333333']);
});

test('snapshot tidak pernah memuat isi pesan', () => {
  const d = new ContactDirectory();
  d.noteChat({
    id: '6281234567890@s.whatsapp.net',
    conversationTimestamp: 1785000000,
    // Baileys dapat menyertakan pesan terakhir pada objek chat.
    messages: [{ message: { conversation: 'rahasia jangan bocor' } }],
    unreadCount: 3,
  });

  const row = d.snapshot()[0];
  assert.deepEqual(Object.keys(row).sort(), ['last_message_at', 'name', 'phone']);
  assert.ok(!JSON.stringify(row).includes('rahasia'));
});

test('timestamp tidak masuk akal tidak dijadikan tanggal', () => {
  const d = new ContactDirectory();
  d.noteChat({ id: '6281234567890@s.whatsapp.net', conversationTimestamp: 0 });

  assert.equal(d.snapshot()[0].last_message_at, null);
});
