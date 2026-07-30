import { z } from 'zod'
import { api } from '@/lib/api'

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

export async function requestPairing() {
  return api.post<{ status: string }>('/wa/pair')
}

export async function logoutDevice() {
  return api.delete<{ status: string }>('/wa/session')
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
