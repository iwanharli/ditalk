import { createBrowserRouter } from 'react-router-dom'
import { AppLayout } from '@/components/app-layout'
import { Placeholder } from '@/components/placeholder'
import { PairingPage } from '@/pages/pairing'
import { ImportPage } from '@/pages/import'
import { ChatPage } from '@/pages/chat'
import { navItems } from '@/lib/nav'

const pages: Record<string, React.ReactNode> = {
  '/pairing': <PairingPage />,
  '/import': <ImportPage />,
  '/chat': <ChatPage />,
}

export const router = createBrowserRouter([
  {
    path: '/',
    element: <AppLayout />,
    children: navItems.map((item) => ({
      index: item.path === '/',
      path: item.path === '/' ? undefined : item.path.slice(1),
      element:
        pages[item.path] ?? (
          <Placeholder
            title={item.title ?? item.label}
            description={item.description}
          />
        ),
    })),
  },
])
