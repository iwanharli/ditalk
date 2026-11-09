import { z } from 'zod'
import { api, ApiError } from '@/lib/api'

export const waStatusSchema = z.enum([
  'disconnected',
  'connecting',
  'pairing',
  'connected',
  'logged_out',
  'error',
])

export const connectionSchema = z.object({
  status: waStatusSchema,
  qr: z.string().optional(),
  qr_expires_at: z.string().optional(),
  self_phone: z.string().optional(),
  self_name: z.string().optional(),
  has_avatar: z.boolean(),
  avatar_version: z.string().optional(),
  last_connected_at: z.string().nullish(),
  last_error: z.string().optional(),
  updated_at: z.string(),
  // Only the connector can generate a QR, so the UI must be able to say
  // "connector is not running" instead of the ambiguous "not connected".
  connector_online: z.boolean(),
  connector_last_seen: z.string().nullish(),
})

export const waStatusResponseSchema = z.object({
  connection: connectionSchema,
  allowlist_total: z.number(),
  allowlist_active: z.number(),
  reads_only_allowlisted: z.boolean(),
})

export const allowedContactSchema = z.object({
  id: z.string(),
  phone: z.string(),
  label: z.string().optional().default(''),
  is_active: z.boolean(),
  consent_note: z.string().optional().default(''),
  created_at: z.string(),
})

export const rejectionSchema = z.object({
  reason: z.string(),
  count: z.number(),
})

export const allowlistResponseSchema = z.object({
  contacts: z.array(allowedContactSchema),
  rejections: z.array(rejectionSchema).nullable(),
})

export type WAStatus = z.infer<typeof waStatusSchema>
export type Connection = z.infer<typeof connectionSchema>
export type WAStatusResponse = z.infer<typeof waStatusResponseSchema>
export type AllowedContact = z.infer<typeof allowedContactSchema>
export type Rejection = z.infer<typeof rejectionSchema>

export async function getWAStatus(): Promise<WAStatusResponse> {
  return waStatusResponseSchema.parse(await api.get('/wa/status'))
}

export async function getAllowlist() {
  return allowlistResponseSchema.parse(await api.get('/wa/allowlist'))
}

/** One row in the picker: a chat found on the device, a registered number, or both. */
export const contactRowSchema = z.object({
  phone: z.string(),
  name: z.string().optional().default(''),
  label: z.string().optional().default(''),
  last_message_at: z.string().nullish(),
  registered: z.boolean(),
  is_active: z.boolean(),
  contact_id: z.string().optional().default(''),
  from_device: z.boolean(),
  // Set only for selected contacts, whose picture the connector fetches.
  avatar_version: z.string().optional().default(''),
  stored: z.number().optional().default(0),
  backfill: z
    .object({
      done: z.boolean(),
      running: z.boolean(),
      fetched: z.number(),
    })
    .nullish(),
})

export const contactsResponseSchema = z.object({
  contacts: z.array(contactRowSchema),
})

export type ContactRow = z.infer<typeof contactRowSchema>

export async function getContacts() {
  return contactsResponseSchema.parse(await api.get('/wa/contacts'))
}

export async function addContact(input: {
  phone: string
  label?: string
  consent_note?: string
}) {
  return allowedContactSchema.parse(await api.post('/wa/allowlist', input))
}

export async function setContactActive(id: string, isActive: boolean) {
  return api.patch(`/wa/allowlist/${id}`, { is_active: isActive })
}

export async function deleteContact(id: string) {
  return api.delete<void>(`/wa/allowlist/${id}`)
}

export const importFileSchema = z.object({
  name: z.string(),
  size_bytes: z.number(),
  modified_at: z.string(),
  is_archive: z.boolean(),
})

export const importFilesResponseSchema = z.object({
  dir: z.string(),
  files: z.array(importFileSchema),
})

export const importPreviewSchema = z.object({
  date_order: z.string(),
  total_messages: z.number(),
  senders: z.record(z.string(), z.number()),
  first_at: z.string().nullish(),
  last_at: z.string().nullish(),
  warnings: z.array(z.string()).nullish(),
})

export const importResultSchema = z.object({
  conversation_id: z.string(),
  inserted: z.number(),
  skipped: z.number(),
  sessions: z.number(),
  date_order: z.string(),
  warnings: z.array(z.string()).nullish(),
})

export type ImportFile = z.infer<typeof importFileSchema>
export type ImportPreview = z.infer<typeof importPreviewSchema>
export type ImportResult = z.infer<typeof importResultSchema>

export async function getImportFiles() {
  return importFilesResponseSchema.parse(await api.get('/imports/files'))
}

/** Either `file` (uploaded) or `fileName` (already in the import folder). */
function importForm(input: {
  file?: File
  fileName?: string
  selfName?: string
  displayName?: string
  chatKey?: string
}) {
  const fd = new FormData()
  if (input.file) fd.append('file', input.file)
  if (input.fileName) fd.append('file_name', input.fileName)
  if (input.selfName) fd.append('self_name', input.selfName)
  if (input.displayName) fd.append('display_name', input.displayName)
  if (input.chatKey) fd.append('chat_key', input.chatKey)
  return fd
}

export async function previewImport(input: { file?: File; fileName?: string }) {
  const res = await fetch('/api/imports/preview', {
    method: 'POST',
    body: importForm(input),
  })
  if (!res.ok) throw new ApiError(res.status, (await res.json()).error ?? res.statusText)
  return importPreviewSchema.parse(await res.json())
}

export async function runImport(input: {
  file?: File
  fileName?: string
  selfName: string
  displayName: string
  chatKey: string
}) {
  const res = await fetch('/api/imports', { method: 'POST', body: importForm(input) })
  if (!res.ok) throw new ApiError(res.status, (await res.json()).error ?? res.statusText)
  return importResultSchema.parse(await res.json())
}

/** Asks the engine to try pulling older history for this contact again. */
export async function retryBackfill(phone: string) {
  return api.post<{ status: string }>(`/wa/contacts/${phone}/backfill`)
}

export async function requestPairing() {
  return api.post<{ status: string }>('/wa/pair')
}

export async function logoutDevice() {
  return api.delete<{ status: string }>('/wa/session')
}

export const conversationSummarySchema = z.object({
  id: z.string(),
  display_name: z.string(),
  phone: z.string().optional().default(''),
  last_message_text: z.string().optional().default(''),
  last_message_at: z.string().nullish(),
  last_sender_role: z.string().optional().default(''),
  last_message_type: z.string().optional().default(''),
  message_count: z.number(),
})

export const conversationsResponseSchema = z.object({
  conversations: z.array(conversationSummarySchema),
})

export const conversationMessageSchema = z.object({
  id: z.string(),
  wa_message_id: z.string(),
  sender_role: z.enum(['SELF', 'OTHER']),
  timestamp: z.string(),
  message_type: z.string(),
  text: z.string().optional().default(''),
  quoted_message_id: z.string().optional().default(''),
  reactions: z.array(z.unknown()).default([]),
  is_view_once: z.boolean(),
  is_ephemeral: z.boolean(),
  is_deleted: z.boolean(),
  edited_at: z.string().nullish(),
})

export const conversationMessagesResponseSchema = z.object({
  messages: z.array(conversationMessageSchema),
})

export type ConversationSummary = z.infer<typeof conversationSummarySchema>
export type ConversationMessage = z.infer<typeof conversationMessageSchema>

export async function getConversations() {
  return conversationsResponseSchema.parse(await api.get('/conversations'))
}

export async function getConversationMessages(conversationID: string) {
  return conversationMessagesResponseSchema.parse(
    await api.get(`/conversations/${conversationID}/messages`),
  )
}

export async function sendConversationMessage(conversationID: string, text: string) {
  return conversationMessageSchema.parse(
    await api.post(`/conversations/${conversationID}/messages`, { text }),
  )
}

/** Human-readable labels for the reasons the filter rejected traffic. */
export const rejectionLabels: Record<string, string> = {
  not_allowlisted: 'Nomor tidak terdaftar',
  group_chat: 'Percakapan grup',
  unsupported_jid: 'Jenis chat tidak didukung',
  inactive_contact: 'Kontak dijeda',
}

export const statusLabels: Record<WAStatus, string> = {
  disconnected: 'Belum tersambung',
  connecting: 'Menyambungkan',
  pairing: 'Menunggu pemindaian QR',
  connected: 'Tersambung',
  logged_out: 'Keluar dari perangkat',
  error: 'Terjadi kesalahan',
}
