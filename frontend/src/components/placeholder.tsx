import { Construction } from 'lucide-react'
import { PageContainer, PageHeader } from '@/components/page-shell'

export function Placeholder({
  title,
  description,
}: {
  title: string
  description?: string
}) {
  return (
    <PageContainer>
      <PageHeader title={title} description={description} />

      <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed px-6 py-16 text-center">
        <span className="grid size-10 place-items-center rounded-full bg-muted">
          <Construction className="size-5 text-muted-foreground" />
        </span>
        <p className="text-sm font-medium">Belum dibangun</p>
        <p className="max-w-sm text-sm text-muted-foreground">
          Halaman ini menunggu implementasi.
        </p>
      </div>
    </PageContainer>
  )
}
