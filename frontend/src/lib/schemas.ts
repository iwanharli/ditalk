import { z } from 'zod'

/** Analysis output contract from doc bab 13.1. */
export const evidenceSchema = z.object({
  source: z.enum(['text', 'audio', 'image', 'video', 'ocr', 'context']),
  quote: z.string().optional(),
  signal: z.string().optional(),
  message_id: z.string().optional(),
})

export const analysisSchema = z.object({
  id: z.string(),
  primary_emotion: z.string().nullable(),
  secondary_emotions: z.array(z.string()),
  valence: z.number().min(-1).max(1).nullable(),
  arousal: z.number().min(0).max(1).nullable(),
  intensity: z.number().min(0).max(1).nullable(),
  confidence: z.number().min(0).max(1).nullable(),
  communication_tone: z.array(z.string()),
  context_sufficiency: z.enum(['low', 'medium', 'high']).nullable(),
  modality_agreement: z.enum(['conflict', 'partial', 'agree']).nullable(),
  evidence: z.array(evidenceSchema),
  alternative_interpretations: z.array(z.string()),
  status: z.enum(['complete', 'insufficient_data', 'not_applicable', 'failed']),
})

/** Aggregate score contract from doc bab 12F.1. */
export const reliabilityStatusSchema = z.enum([
  'insufficient',
  'provisional',
  'sufficient',
  'high',
  'very_high',
])

export const aggregateScoreSchema = z.object({
  conversation_id: z.string(),
  aggregation_level: z.enum(['session', 'day', 'week', 'month', 'custom']),
  period_start: z.string(),
  period_end: z.string(),
  // Null whenever reliability is insufficient; the UI must hide the number
  // rather than show a zero (doc bab 12E.2).
  global_communication_index: z.number().min(0).max(100).nullable(),
  data_reliability: z.number().min(0).max(100),
  reliability_status: reliabilityStatusSchema,
  scores: z.record(z.string(), z.number()),
  coverage: z.object({ eligible: z.number(), analyzed: z.number() }),
  score_version: z.string(),
})

export const memoryStatusSchema = z.enum([
  'candidate',
  'confirmed',
  'superseded',
  'archived',
  'outdated',
])

export const memorySchema = z.object({
  id: z.string(),
  category: z.string(),
  title: z.string(),
  body: z.string().nullable(),
  status: memoryStatusSchema,
  confidence: z.number().min(0).max(100),
  importance: z.number().min(0).max(100).nullable(),
  sensitivity: z.enum(['low', 'medium', 'high']),
  valid_from: z.string().nullable(),
  valid_until: z.string().nullable(),
})

export type Evidence = z.infer<typeof evidenceSchema>
export type Analysis = z.infer<typeof analysisSchema>
export type AggregateScore = z.infer<typeof aggregateScoreSchema>
export type Memory = z.infer<typeof memorySchema>
export type ReliabilityStatus = z.infer<typeof reliabilityStatusSchema>
