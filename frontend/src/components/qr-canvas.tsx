import { useEffect, useRef, useState } from 'react'
import QRCode from 'qrcode'

/**
 * Renders a Baileys pairing payload as a QR image.
 *
 * The code is drawn locally from the string the backend relays; it is never
 * uploaded anywhere, and it is deliberately not persisted since a pairing code
 * is a short-lived credential.
 */
export function QRCanvas({ value, size = 264 }: { value: string; size?: number }) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas || !value) return

    let cancelled = false
    QRCode.toCanvas(canvas, value, {
      width: size,
      margin: 2,
      errorCorrectionLevel: 'M',
      color: { dark: '#000000', light: '#ffffff' },
    })
      .then(() => {
        if (!cancelled) setError(null)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : 'gagal membuat QR')
      })

    return () => {
      cancelled = true
    }
  }, [value, size])

  if (error) {
    return (
      <div
        className="flex items-center justify-center rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground"
        style={{ width: size, height: size }}
      >
        Gagal membuat QR: {error}
      </div>
    )
  }

  return (
    // A white plate keeps the code scannable in dark mode.
    <div className="inline-flex rounded-lg bg-white p-3">
      <canvas ref={canvasRef} aria-label="Kode QR untuk menautkan WhatsApp" />
    </div>
  )
}
