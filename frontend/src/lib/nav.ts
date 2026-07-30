import {
  FileText,
  HeartHandshake,
  LayoutDashboard,
  Library,
  MessagesSquare,
  Mic,
  QrCode,
  Search,
  ShieldCheck,
  TrendingUp,
  type LucideIcon,
} from 'lucide-react'

export type NavItem = {
  path: string
  label: string
  /** Shown in the topbar; the sidebar label stays short. */
  title?: string
  description?: string
  icon: LucideIcon
}

export type NavGroup = {
  label: string
  items: NavItem[]
}

/**
 * Sidebar structure. Grouping keeps eleven destinations scannable instead of
 * presenting one flat list. Sections follow the dashboard concept in doc bab 15.
 */
export const navGroups: NavGroup[] = [
  {
    label: 'Analisis',
    items: [
      {
        path: '/',
        label: 'Overview',
        description: 'Ringkasan periode, skor, dan momen yang perlu ditinjau.',
        icon: LayoutDashboard,
      },
      {
        path: '/conversations',
        label: 'Percakapan',
        description: 'Timeline pesan dengan label emosi dan bukti pendukung.',
        icon: MessagesSquare,
      },
      {
        path: '/trends',
        label: 'Tren Emosi',
        description: 'Perubahan suasana antarhari, sesi, dan periode.',
        icon: TrendingUp,
      },
      {
        path: '/media',
        label: 'Media & Transkrip',
        description: 'Transkrip voice note, OCR gambar, dan keyframe video.',
        icon: Mic,
      },
    ],
  },
  {
    label: 'Memori',
    items: [
      {
        path: '/vault',
        label: 'Knowledge Vault',
        description: 'Kejadian, tanggal, preferensi, komitmen, dan batasan.',
        icon: Library,
      },
      {
        path: '/relationship',
        label: 'Relationship',
        description: 'Tujuh pilar pola komunikasi beserta drill-down bukti.',
        icon: HeartHandshake,
      },
      {
        path: '/search',
        label: 'Pencarian',
        title: 'Pencarian Semantik',
        description: 'Cari berdasarkan makna, bukan hanya kata kunci.',
        icon: Search,
      },
    ],
  },
  {
    label: 'Sistem',
    items: [
      {
        path: '/pairing',
        label: 'Sambungkan WA',
        title: 'Sambungkan WhatsApp',
        description:
          'Tautkan akun Anda sendiri dan tentukan nomor mana yang boleh dibaca.',
        icon: QrCode,
      },
      {
        path: '/reports',
        label: 'Laporan',
        description: 'Ekspor PDF, JSON, dan CSV beserta disclaimer.',
        icon: FileText,
      },
      {
        path: '/privacy',
        label: 'Privasi & Data',
        description: 'Retensi, penghapusan, provider, dan redaksi identitas.',
        icon: ShieldCheck,
      },
    ],
  },
]

export const navItems: NavItem[] = navGroups.flatMap((g) => g.items)

export function findNavItem(pathname: string): NavItem | undefined {
  return navItems.find((i) => i.path === pathname)
}
