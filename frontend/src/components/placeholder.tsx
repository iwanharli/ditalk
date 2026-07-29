import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

export function Placeholder({ title }: { title: string }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="font-heading">{title}</CardTitle>
        <CardDescription>Belum dibangun.</CardDescription>
      </CardHeader>
      <CardContent className="text-sm text-muted-foreground">
        Halaman ini menunggu implementasi.
      </CardContent>
    </Card>
  )
}
