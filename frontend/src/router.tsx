import { createBrowserRouter } from 'react-router-dom'
import { AppLayout } from '@/components/app-layout'
import { Placeholder } from '@/components/placeholder'

// Sections mirror the dashboard concept in doc bab 15.
export const navigation = [
  { path: '/', label: 'Overview' },
  { path: '/conversations', label: 'Percakapan' },
  { path: '/trends', label: 'Tren Emosi' },
  { path: '/media', label: 'Media & Transkrip' },
  { path: '/vault', label: 'Knowledge Vault' },
  { path: '/relationship', label: 'Relationship' },
  { path: '/search', label: 'Pencarian Semantik' },
  { path: '/reports', label: 'Laporan' },
  { path: '/privacy', label: 'Privasi & Data' },
] as const

export const router = createBrowserRouter([
  {
    path: '/',
    element: <AppLayout />,
    children: navigation.map(({ path, label }) => ({
      index: path === '/',
      path: path === '/' ? undefined : path.slice(1),
      element: <Placeholder title={label} />,
    })),
  },
])
