# Fitur Baileys yang Relevan untuk ditalk

Catatan hasil penelusuran **source code Baileys v6.7.24** yang terpasang di
`services/wa-connector/node_modules/@whiskeysockets/baileys`, dilengkapi
dokumentasi resmi di <https://baileys.wiki>.

Source didahulukan atas dokumentasi: dokumentasi resmi tidak membahas read
receipt sama sekali, dan beberapa perilaku penting hanya terlihat di kode.

Ditinjau 30 Juli 2026.

---

## 1. Pertanyaan utama: bisa membaca tanpa centang biru?

**Bisa.** Membaca pesan **tidak** mengirim read receipt. Centang biru hanya
muncul kalau aplikasi memanggil `readMessages()` secara eksplisit, dan ditalk
tidak pernah memanggilnya.

Tetapi ada satu hal yang **tidak bisa dihindari**: centang abu-abu ganda
(*delivered*) tetap terkirim. Itu ack tingkat protokol — WhatsApp perlu tahu
pesannya sampai ke perangkat.

### Rantai buktinya di source

**a. Read receipt hanya dari `readMessages()`**

`lib/Socket/messages-send.js:105`

```js
const readMessages = async (keys) => {
    const privacySettings = await fetchPrivacySettings();
    const readType = privacySettings.readreceipts === 'all' ? 'read' : 'read-self';
    await sendReceipts(keys, readType);
};
```

`lib/Socket/messages-send.js:64` — hanya dua tipe ini yang dihitung read:

```js
const isReadReceipt = type === 'read' || type === 'read-self';
```

Tipe receipt yang ada (`lib/Types/Message.d.ts:46`):

```
'read' | 'read-self' | 'hist_sync' | 'peer_msg' | 'sender' | 'inactive' | 'played' | undefined
```

**b. Receipt otomatis saat pesan masuk bukan read receipt**

`lib/Socket/messages-recv.js:672-690`

```js
// no type in the receipt => message delivered
let type = undefined;
...
else if (!sendActiveReceipts) {
    type = 'inactive';
}
await sendReceipt(msg.key.remoteJid, participant, [msg.key.id], type);
```

Jadi setiap pesan masuk memicu receipt bertipe `undefined` (delivered) atau
`'inactive'` — **tidak pernah** `'read'`.

**c. `markOnlineOnConnect: false` membuat tipenya `inactive`**

`lib/Socket/chats.js:861`

```js
sendPresenceUpdate(markOnlineOnConnect ? 'available' : 'unavailable')
```

`lib/Socket/messages-recv.js:34, 1068-1071`

```js
let sendActiveReceipts = false;
...
ev.on('connection.update', ({ isOnline }) => {
    if (typeof isOnline !== 'undefined') {
        sendActiveReceipts = isOnline;
    }
});
```

Karena presence dikirim `unavailable`, `isOnline` tidak pernah true, sehingga
`sendActiveReceipts` tetap `false` dan receipt bertipe `'inactive'` —
memberi tahu WhatsApp bahwa perangkat ini tidak sedang aktif membaca.

### Apa yang dilihat lawan bicara

| Sinyal | Terkirim? | Keterangan |
| --- | --- | --- |
| ✓ terkirim ke server | Ya | Tidak terhindarkan |
| ✓✓ delivered (abu-abu) | **Ya** | Ack protokol, tidak bisa dimatikan |
| ✓✓ read (biru) | **Tidak** | Butuh `readMessages()`, tidak pernah dipanggil |
| "online" / last seen | **Tidak** | `markOnlineOnConnect: false` |
| "typing…" | Tidak | Butuh `sendPresenceUpdate('composing')` |
| Voice note diputar | Tidak | Butuh receipt tipe `'played'` |

**Catatan penting:** delivered ✓✓ akan muncul **lebih cepat** daripada biasanya,
karena connector selalu tersambung. Kalau ponsel Anda sedang mati, pengirim
tetap melihat ✓✓ karena Linked Device sudah menerimanya. Ini konsekuensi wajar
dari perangkat tertaut yang selalu hidup, bukan hal yang bisa disembunyikan.

### Yang harus dihindari agar tetap begitu

Jangan pernah memanggil:

- `sock.readMessages(keys)` — mengirim read receipt (centang biru)
- `sock.sendReceipt(jid, p, ids, 'read')` atau `'read-self'` — sama
- `sock.sendReceipt(..., 'played')` — menandai voice note sudah diputar
- `sock.sendPresenceUpdate('available')` — tampil online
- `sock.sendPresenceUpdate('composing' | 'recording')` — tampil sedang menulis
- `sock.chatModify({ markRead: true, lastMessages }, jid)` — menandai chat
  terbaca lewat app state, menyebar ke perangkat lain Anda
- `sock.presenceSubscribe(jid)` — meminta status online orang lain; ini
  mengirim sinyal ke server dan mengumpulkan data yang tidak diperlukan ditalk

Read-only di ditalk berarti **tidak ada satu pun** dari daftar itu dipanggil.
Yang dipakai hanya `downloadMediaMessage`, `profilePictureUrl` untuk akun
sendiri, `logout`, dan listener event.

---

## 2. History sync: hanya sekali per pairing

Ini yang paling sering mengejutkan.

WhatsApp mengirim riwayat percakapan **hanya sekali, tepat setelah perangkat
ditautkan**. Pada koneksi berikutnya `messaging-history.set` tidak dikirim lagi.

Terbukti pada log ditalk: run dengan auth baru menghasilkan 2 kali `history
sync`; reconnect berikutnya 0 kali, dengan pesan
`History sync is enabled, awaiting notification with a 20s timeout` lalu tidak
ada apa pun.

**Implikasi:** daftar percakapan harus di-cache sendiri. ditalk menyimpannya di
`services/wa-connector/.wa-chats.json`, di samping auth state, dan menghapusnya
bersamaan saat logout.

### Saran "pakai browser Desktop" tidak berlaku — sudah diuji dan ditolak

Dokumentasi resmi menyarankan `syncFullHistory` dipasangkan dengan descriptor
desktop seperti `Browsers.macOS("Desktop")`. Secara kode, saran itu masuk akal.

`lib/Utils/validate-connection.js:27-37`

```js
const PLATFORM_MAP = {
    'Mac OS': proto.ClientPayload.WebInfo.WebSubPlatform.DARWIN,
    Windows: proto.ClientPayload.WebInfo.WebSubPlatform.WIN32
};
const getWebInfo = (config) => {
    let webSubPlatform = proto.ClientPayload.WebInfo.WebSubPlatform.WEB_BROWSER;
    if (config.syncFullHistory && PLATFORM_MAP[config.browser[0]]) {
        webSubPlatform = PLATFORM_MAP[config.browser[0]];
    }
    return { webSubPlatform };
};
```

Default Baileys adalah `Browsers.ubuntu('Chrome')` (`lib/Defaults/index.js:31`),
dan `'Ubuntu'` tidak ada di `PLATFORM_MAP`, jadi `webSubPlatform` tetap
`WEB_BROWSER`.

**Tetapi dalam praktiknya WhatsApp menolak semua descriptor desktop.** Diukur
30 Juli 2026 dengan auth baru, masing-masing 14 detik:

| `browser` | QR muncul | Koneksi ditutup |
| --- | --- | --- |
| `["Ubuntu","Chrome","22.04.4"]` | **1** | 0 |
| `["Mac OS","Desktop","14.4.1"]` | 0 | 1 |
| `["Mac OS","Safari","14.4.1"]` | 0 | 1 |
| `["Mac OS","Chrome","14.4.1"]` | 0 | 1 |
| `["Windows","Desktop","10.0.22631"]` | 0 | 1 |

Descriptor desktop ditutup seketika dan **tidak pernah menghasilkan QR sama
sekali** — jadi bukan sekadar riwayat lebih sedikit, tetapi pairing mustahil.

Sebagai tambahan, mengubah descriptor pada sesi yang **sudah ada** juga
membatalkan sesi itu: login ditolak dengan `Connection Terminated`, sementara
auth yang sama tetap berhasil bila descriptor dikembalikan. Terverifikasi lewat
perbandingan terkontrol.

**Kesimpulan praktis:** biarkan `browser` pada default. `webSubPlatform` akan
tetap `WEB_BROWSER`, dan riwayat yang didapat adalah sebanyak yang diberikan
WhatsApp untuk web client. Untuk riwayat lengkap satu kontak, jalur yang andal
adalah **Export Chat dari WhatsApp lalu impor** lewat `POST /imports`.

`Browsers` yang tersedia (`lib/Utils/generics.js:23`): `ubuntu`, `macOS`,
`baileys`, `windows`, `appropriate`. Uji ulang sebelum mengganti — perilaku
server WhatsApp bisa berubah kapan saja.

### Mengambil riwayat lebih jauh

`sock.fetchMessageHistory(count, oldestMsgKey, oldestMsgTimestamp)` bisa meminta
riwayat di luar sync awal, tetapi **butuh satu message key sebagai titik awal**.
Jadi tidak menolong saat database masih kosong.

`sock.requestPlaceholderResend(messageKey)` meminta pesan yang gagal didekripsi
dikirim ulang.

---

## 3. Opsi socket yang penting

| Opsi | Default | Catatan untuk ditalk |
| --- | --- | --- |
| `markOnlineOnConnect` | `true` | **Wajib `false`.** Kalau `true`, akun tampil online, notifikasi di ponsel berhenti, dan receipt jadi aktif. |
| `syncFullHistory` | `false` | `true` di ditalk. Saran memasangkannya dengan browser desktop **tidak berlaku**; lihat bagian 2. |
| `browser` | `Browsers.ubuntu('Chrome')` | **Biarkan default.** Descriptor desktop ditolak WhatsApp dan tidak menghasilkan QR sama sekali. |
| `shouldSyncHistoryMessage` | mengikuti `syncFullHistory` | `() => false` mematikan history sync. |
| `shouldIgnoreJid` | — | Filter JID di level socket. Bisa dipakai sebagai lapisan allowlist ketiga. |
| `emitOwnEvents` | `true` | Memunculkan event untuk aksi dari socket ini sendiri. |
| `getMessage` | — | Dipakai untuk retry kirim dan dekripsi vote polling. Tidak relevan untuk read-only. |
| `fireInitQueries` | `true` | Query awal saat konek. |
| `printQRInTerminal` | — | Deprecated; tangani sendiri dari `connection.update`. |
| `version` | bawaan | Dokumentasi menyarankan **jangan** mengambil versi terbaru setiap konek; berisiko tidak kompatibel. |
| `keepAliveIntervalMs` | — | Ping-pong WebSocket. |
| `qrTimeout` | — | Batas waktu QR sebelum diganti. |
| `appStateMacVerification` | — | Verifikasi MAC app state. |

---

## 4. Event yang dipakai ditalk

| Event | Isi | Dipakai untuk |
| --- | --- | --- |
| `connection.update` | `connection`, `qr`, `lastDisconnect`, `isOnline` | Status pairing, QR, reconnect |
| `creds.update` | — | Menyimpan kredensial |
| `messaging-history.set` | `chats`, `contacts`, `messages`, `isLatest`, `progress` | Sync awal; **hanya sekali per pairing** |
| `messages.upsert` | `{ messages: [], type: 'notify' \| 'append' }` | Pesan baru dan lama |
| `messages.update` | array update | Edit dan perubahan receipt |
| `messages.delete` | `{ keys }` atau `{ jid, all }` | Pesan dihapus |
| `messages.reaction` | array reaksi | Reaksi emoji |
| `chats.upsert` / `chats.update` | array chat | Kandidat kontak untuk pemilih |
| `contacts.upsert` / `contacts.update` | array kontak | Nama untuk chat yang ada |
| `message-receipt.update` | — | Siapa menerima/melihat/memutar; **tidak dipakai** |

### Dua jebakan pada event

1. **`messages.upsert` membawa ARRAY.** Dokumentasi menekankan semua elemen
   harus diproses; memproses elemen pertama saja akan melewatkan pesan.
2. **`type: 'append'` vs `'notify'`.** `notify` biasanya pesan baru, `append`
   pesan lama yang sudah pernah dilihat. Keduanya perlu ditangani.

---

## 5. Media

`downloadMediaMessage(message, 'buffer' | 'stream', options)` mengunduh media.
Untuk media yang gagal, opsi `reuploadRequest: sock.updateMediaMessage` meminta
pengirim mengunggah ulang.

Media yang perlu diperhatikan ditalk:

- **View-once** (`viewOnceMessage`, `viewOnceMessageV2`) — ditalk **tidak
  menyimpannya**; pengirim memilih pesan yang menghilang.
- **Ephemeral** (`ephemeralMessage`) — pesan menghilang; perlu kebijakan retensi
  eksplisit.
- `documentWithCaptionMessage` — dokumen dengan caption, perlu di-unwrap.

Mengunduh media **tidak** mengirim receipt `'played'`; menandai voice note
sudah diputar butuh panggilan terpisah.

---

## 6. Identitas kontak: JID vs LID

| Bentuk | Arti |
| --- | --- |
| `628xxx@s.whatsapp.net` | Kontak satu-lawan-satu; bagian sebelum `@` adalah nomor telepon |
| `628xxx:12@s.whatsapp.net` | Sama, dengan suffix perangkat |
| `xxx@lid` | **Identifier opaque, BUKAN nomor telepon** |
| `xxx-yyy@g.us` | Grup |
| `status@broadcast` | Status/broadcast |
| `xxx@newsletter` | Channel |

**`@lid` adalah jebakan paling berbahaya untuk allowlist.** Digitnya terlihat
seperti nomor telepon tetapi bukan. Kalau diperlakukan sebagai nomor, filter
akan mencocokkan **kontak yang salah**. ditalk menolak `@lid` secara eksplisit
di `waid.PhoneFromJID` dan di `allowlist.js`.

Helper: `jidNormalizedUser`, `jidDecode`, `isJidUser`, `isJidGroup`.

---

## 7. Auth state

`useMultiFileAuthState(dir)` menyimpan kredensial sebagai banyak file JSON.
Dokumentasi Baileys menyebutnya **hanya cocok sebagai contoh/demo dan tidak
efisien untuk produksi**.

ditalk masih memakainya. Untuk produksi perlu custom auth state terenkripsi
dengan kunci dari KMS/Vault, terpisah dari database aplikasi (doc bab 5.3).

`sock.logout()` melepas perangkat dari sisi WhatsApp. Menghapus direktori auth
saja tidak melepas tautan di ponsel.

---

## 8. API yang sengaja TIDAK dipakai

Baileys menyediakan banyak hal yang bertentangan dengan tujuan read-only ditalk:

- `sendMessage`, `relayMessage` — mengirim pesan
- `readMessages`, `sendReceipt('read')` — centang biru
- `sendPresenceUpdate('available' \| 'composing')` — online dan typing
- `presenceSubscribe` — memantau status online orang lain
- `chatModify({ markRead })` — menandai terbaca lewat app state
- `groupCreate`, `groupParticipantsUpdate`, dll — manajemen grup
- `updateProfilePicture`, `updateProfileName`, `updateProfileStatus` — mengubah
  profil
- `updateBlockStatus` — blokir
- `onWhatsApp` — memeriksa apakah nomor terdaftar; tidak diperlukan
- `updateReadReceiptsPrivacy`, `updateOnlinePrivacy`, `updateLastSeenPrivacy` —
  mengubah pengaturan privasi akun; **jangan**, ini mengubah setelan akun
  pengguna dari luar

---

## 9. Batasan dan risiko

- **Bukan API resmi.** Baileys memakai protokol WhatsApp Web lewat WebSocket dan
  tidak berafiliasi dengan WhatsApp. Dokumentasi upstream memperingatkan agar
  tidak dipakai untuk spam, stalkerware, bulk messaging, atau automated
  messaging.
- **Ketentuan layanan WhatsApp** melarang pengumpulan informasi secara tidak
  sah, bulk/auto messaging, dan eksploitasi layanan lewat otomasi. ditalk
  membatasi diri pada akun sendiri, read-only, tanpa distribusi ke pihak ketiga.
- **Breaking change.** v7 membawa perubahan besar; versi harus di-pin dan
  upgrade perlu regression test (pairing, reconnect, history, media, edit,
  delete, reaction).
- **Risiko pembatasan akun** tetap ada karena ini klien tidak resmi.

---

## 10. Yang berlaku di ditalk saat ini

```js
// services/wa-connector/src/index.js
makeWASocket({
  auth: state,
  logger,
  markOnlineOnConnect: false,          // tidak tampil online, receipt 'inactive'
  syncFullHistory: true,
  browser: Browsers.ubuntu('Chrome'),  // default; descriptor desktop ditolak
})
```

Tidak ada satu pun API pengirim yang dipanggil. Yang dipakai hanya listener
event, `downloadMediaMessage`, `profilePictureUrl` untuk akun sendiri, dan
`logout`.

Connector juga berhenti setelah 5 kegagalan login berturut-turut tanpa pernah
mencapai `open`, lalu melaporkan status `error` dengan instruksi pairing ulang.
Sesi yang ditolak WhatsApp gagal dengan cara yang sama setiap kali, jadi mencoba
terus hanya menghasilkan loop tanpa QR dan tanpa penjelasan. Hitungan direset
ketika QR muncul, karena munculnya QR berarti pairing memang sedang mungkin.

---

## Sumber

- Source Baileys v6.7.24 di `services/wa-connector/node_modules/@whiskeysockets/baileys`
  (paling menentukan; dokumentasi resmi tidak membahas read receipt)
- <https://baileys.wiki/docs/socket/configuration>
- <https://baileys.wiki/docs/socket/receiving-updates>
- <https://baileys.wiki/docs/socket/history-sync>
- <https://baileys.wiki/docs/intro/>
- <https://www.whatsapp.com/legal/terms-of-service>
