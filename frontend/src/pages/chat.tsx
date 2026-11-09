import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CheckCheck,
  Image,
  Loader2,
  Mic,
  MoreHorizontal,
  PanelRightClose,
  PanelRightOpen,
  Paperclip,
  Phone,
  Search,
  Send,
  Smile,
  Sparkles,
  Video,
} from 'lucide-react'
import { toast } from 'sonner'

import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/lib/utils'
import {
  getConversationMessages,
  getConversations,
  sendConversationMessage,
  type ConversationMessage,
  type ConversationSummary,
} from '@/lib/wa'

const tones = [
  'from-teal-500 to-cyan-600',
  'from-sky-500 to-indigo-600',
  'from-amber-500 to-rose-600',
  'from-blue-500 to-slate-600',
  'from-fuchsia-500 to-pink-600',
  'from-orange-500 to-yellow-600',
  'from-violet-500 to-blue-600',
]

export function ChatPage() {
  const qc = useQueryClient()
  const [contextOpen, setContextOpen] = useState(true)
  const [selectedID, setSelectedID] = useState('')
  const [draft, setDraft] = useState('')

  const conversations = useQuery({
    queryKey: ['conversations'],
    queryFn: getConversations,
    refetchInterval: 10_000,
  })

  const rows = useMemo(
    () => conversations.data?.conversations ?? [],
    [conversations.data?.conversations],
  )

  useEffect(() => {
    if (rows.length === 0) {
      setSelectedID('')
      return
    }
    if (!selectedID || !rows.some((row) => row.id === selectedID)) {
      setSelectedID(rows[0].id)
    }
  }, [rows, selectedID])

  const selected = rows.find((row) => row.id === selectedID) ?? rows[0]

  const messages = useQuery({
    queryKey: ['conversations', selected?.id, 'messages'],
    queryFn: () => getConversationMessages(selected!.id),
    enabled: Boolean(selected?.id),
    refetchInterval: 10_000,
  })

  const send = useMutation({
    mutationFn: (text: string) => sendConversationMessage(selected!.id, text),
    onSuccess: () => {
      setDraft('')
      void qc.invalidateQueries({ queryKey: ['conversations'] })
      void qc.invalidateQueries({ queryKey: ['conversations', selected?.id, 'messages'] })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : 'Gagal menyimpan pesan.')
    },
  })

  return (
    <div className="h-[calc(100svh-3.5rem)] overflow-hidden bg-[#0f1110] text-zinc-100">
      <div
        className={cn(
          'grid h-full min-h-0 grid-cols-[minmax(300px,400px)_1fr] transition-[grid-template-columns] duration-200',
          contextOpen
            ? 'xl:grid-cols-[minmax(320px,420px)_1fr_300px]'
            : 'xl:grid-cols-[minmax(320px,420px)_1fr_64px]',
        )}
      >
        <aside className="flex min-h-0 min-w-0 flex-col border-r border-white/10 bg-[#191b1a]">
          <ChatListHeader total={rows.length} loading={conversations.isPending} />
          <ScrollArea className="min-h-0 flex-1">
            <div className="p-2">
              {conversations.isPending ? (
                <LoadingRows />
              ) : rows.length === 0 ? (
                <EmptyList />
              ) : (
                rows.map((chat, index) => (
                  <ChatRow
                    key={chat.id}
                    chat={chat}
                    tone={tones[index % tones.length]}
                    active={chat.id === selected?.id}
                    onClick={() => setSelectedID(chat.id)}
                  />
                ))
              )}
            </div>
          </ScrollArea>
        </aside>

        <section className="flex min-h-0 min-w-0 flex-col bg-[#070908]">
          <ThreadHeader selected={selected} />

          <ScrollArea className="min-h-0 flex-1">
            <div className="relative min-h-full px-5 py-5">
              <ChatPattern />
              <div className="relative mx-auto flex max-w-5xl flex-col gap-2">
                {!selected ? (
                  <EmptyThread />
                ) : messages.isPending ? (
                  <ThreadLoading />
                ) : (messages.data?.messages.length ?? 0) === 0 ? (
                  <NoMessages />
                ) : (
                  groupMessagesByDay(messages.data?.messages ?? []).map((group) => (
                    <div key={group.label} className="contents">
                      <DayDivider label={group.label} />
                      {group.messages.map((message) => (
                        <MessageBubble key={message.id} message={message} />
                      ))}
                    </div>
                  ))
                )}
              </div>
            </div>
          </ScrollArea>

          <Composer
            value={draft}
            disabled={!selected || send.isPending}
            sending={send.isPending}
            onChange={setDraft}
            onSend={() => {
              const text = draft.trim()
              if (text && selected) send.mutate(text)
            }}
          />
        </section>

        <aside className="hidden min-h-0 min-w-0 border-l border-white/10 bg-[#171a18] xl:flex xl:flex-col">
          {contextOpen ? (
            <ContextPanel
              conversation={selected}
              lastMessage={messages.data?.messages.at(-1)}
              onCollapse={() => setContextOpen(false)}
            />
          ) : (
            <ContextRail onExpand={() => setContextOpen(true)} />
          )}
        </aside>
      </div>
    </div>
  )
}

function ChatListHeader({ total, loading }: { total: number; loading: boolean }) {
  return (
    <div className="border-b border-white/10 p-4">
      <div className="mb-4 flex items-center justify-between gap-3">
        <h1 className="text-xl font-semibold">Chats</h1>
        <div className="flex gap-1">
          <Button variant="ghost" size="icon" className="text-zinc-300 hover:bg-white/10 hover:text-white">
            <MoreHorizontal className="size-4" />
          </Button>
          <Button variant="ghost" size="icon" className="text-zinc-300 hover:bg-white/10 hover:text-white">
            <Sparkles className="size-4" />
          </Button>
        </div>
      </div>

      <div className="relative">
        <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-zinc-500" />
        <Input
          aria-label="Cari chat"
          placeholder="Search"
          className="h-10 border-white/10 bg-[#101211] pl-9 text-zinc-100 placeholder:text-zinc-500 focus-visible:ring-teal-500/40"
        />
      </div>

      <div className="mt-3 flex gap-2 overflow-x-auto pb-1">
        {['All', loading ? 'Loading' : `${total} chat`, 'Favorites', 'Groups'].map((filter, index) => (
          <button
            key={filter}
            type="button"
            className={cn(
              'h-8 shrink-0 rounded-full border px-4 text-sm font-medium',
              index === 0
                ? 'border-teal-500/30 bg-teal-500/20 text-teal-200'
                : 'border-white/10 bg-transparent text-zinc-400 hover:bg-white/5 hover:text-zinc-100',
            )}
          >
            {filter}
          </button>
        ))}
      </div>
    </div>
  )
}

function ChatRow({
  chat,
  tone,
  active,
  onClick,
}: {
  chat: ConversationSummary
  tone: string
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'flex w-full items-center gap-3 rounded-md px-3 py-3 text-left transition-colors',
        active ? 'bg-white/10' : 'hover:bg-white/[0.06]',
      )}
    >
      <Avatar className="size-12 shrink-0">
        <AvatarFallback className={cn('bg-gradient-to-br text-sm font-semibold text-white', tone)}>
          {initials(chat.display_name)}
        </AvatarFallback>
      </Avatar>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <p className="min-w-0 flex-1 truncate font-semibold text-zinc-100">
            {chat.display_name}
          </p>
          <span className="ml-auto shrink-0 text-xs text-zinc-500">
            {formatShortTime(chat.last_message_at)}
          </span>
        </div>
        <div className="mt-1 flex items-center gap-2">
          <p className="min-w-0 flex-1 truncate text-sm text-zinc-400">
            {chat.last_sender_role === 'SELF' && 'You: '}
            {chat.last_message_text || mediaLabel(chat.last_message_type)}
          </p>
          <Badge variant="secondary" className="bg-white/10 text-[0.6875rem] text-zinc-300 hover:bg-white/10">
            {chat.message_count}
          </Badge>
        </div>
      </div>
    </button>
  )
}

function ThreadHeader({ selected }: { selected?: ConversationSummary }) {
  return (
    <header className="flex h-16 shrink-0 items-center gap-3 border-b border-white/10 bg-[#1d211f] px-4">
      <Avatar className="size-11">
        <AvatarFallback className="bg-gradient-to-br from-teal-500 to-cyan-600 font-semibold text-white">
          {initials(selected?.display_name ?? '?')}
        </AvatarFallback>
      </Avatar>
      <div className="min-w-0 flex-1">
        <p className="truncate font-semibold">{selected?.display_name ?? 'Pilih chat'}</p>
        <p className="truncate text-sm text-zinc-400">
          {selected ? `${selected.message_count.toLocaleString('id-ID')} pesan tersimpan` : 'Database belum memiliki percakapan'}
        </p>
      </div>
      <div className="flex gap-1">
        <Button variant="ghost" size="icon" className="text-zinc-300 hover:bg-white/10 hover:text-white">
          <Video className="size-5" />
        </Button>
        <Button variant="ghost" size="icon" className="text-zinc-300 hover:bg-white/10 hover:text-white">
          <Phone className="size-5" />
        </Button>
        <Button variant="ghost" size="icon" className="text-zinc-300 hover:bg-white/10 hover:text-white">
          <Search className="size-5" />
        </Button>
      </div>
    </header>
  )
}

function Composer({
  value,
  disabled,
  sending,
  onChange,
  onSend,
}: {
  value: string
  disabled: boolean
  sending: boolean
  onChange: (value: string) => void
  onSend: () => void
}) {
  return (
    <form
      className="flex shrink-0 items-center gap-2 border-t border-white/10 bg-[#1d211f] px-4 py-3"
      onSubmit={(e) => {
        e.preventDefault()
        onSend()
      }}
    >
      <Button type="button" variant="ghost" size="icon" className="text-zinc-300 hover:bg-white/10 hover:text-white">
        <Paperclip className="size-5" />
      </Button>
      <Button type="button" variant="ghost" size="icon" className="text-zinc-300 hover:bg-white/10 hover:text-white">
        <Smile className="size-5" />
      </Button>
      <div className="flex min-w-0 flex-1 items-center rounded-full border border-white/10 bg-[#101211] px-4">
        <input
          value={value}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
          aria-label="Tulis pesan"
          placeholder={disabled ? 'Pilih chat dulu' : 'Message'}
          className="h-11 min-w-0 flex-1 bg-transparent text-sm text-zinc-100 outline-none placeholder:text-zinc-500 disabled:cursor-not-allowed"
        />
      </div>
      <Button
        type="submit"
        size="icon"
        disabled={disabled || value.trim() === ''}
        className="bg-teal-500 text-teal-950 hover:bg-teal-400"
      >
        {sending ? <Loader2 className="size-4 animate-spin" /> : <Send className="size-4" />}
      </Button>
      <Button type="button" variant="ghost" size="icon" className="text-zinc-300 hover:bg-white/10 hover:text-white">
        <Mic className="size-5" />
      </Button>
    </form>
  )
}

function MessageBubble({ message }: { message: ConversationMessage }) {
  const mine = message.sender_role === 'SELF'
  const text = message.is_deleted
    ? 'Pesan dihapus'
    : message.text || mediaLabel(message.message_type)

  return (
    <div className={cn('flex', mine ? 'justify-end' : 'justify-start')}>
      <div
        className={cn(
          'max-w-[min(78%,680px)] rounded-lg px-3 py-2 text-sm shadow-sm',
          mine ? 'bg-[#0b5a44] text-zinc-50' : 'bg-[#2b2d2c] text-zinc-100',
        )}
      >
        {message.message_type === 'audio' && (
          <div className="mb-1 flex items-center gap-2 text-teal-200">
            <Mic className="size-4" />
            <span className="text-xs font-semibold">Voice message</span>
          </div>
        )}
        <p className={cn('whitespace-pre-line leading-relaxed', message.is_deleted && 'italic text-zinc-300')}>
          {text}
        </p>
        <div className="mt-1 flex items-center justify-end gap-1 text-[0.6875rem] text-zinc-300/80">
          <span>{formatMessageTime(message.timestamp)}</span>
          {mine && <CheckCheck className="size-4 text-cyan-300" />}
        </div>
      </div>
    </div>
  )
}

function ContextRail({ onExpand }: { onExpand: () => void }) {
  return (
    <div className="flex h-full flex-col items-center gap-3 px-2 py-4">
      <Button
        variant="ghost"
        size="icon"
        onClick={onExpand}
        aria-label="Buka panel konteks"
        className="text-zinc-300 hover:bg-white/10 hover:text-white"
      >
        <PanelRightOpen className="size-5" />
      </Button>
      <div className="h-px w-8 bg-white/10" />
      <span className="grid size-9 place-items-center rounded-md bg-teal-500/10 text-teal-300">
        <Sparkles className="size-4" />
      </span>
      <span className="grid size-9 place-items-center rounded-md bg-white/[0.04] text-zinc-500">
        <Image className="size-4" />
      </span>
    </div>
  )
}

function ContextPanel({
  conversation,
  lastMessage,
  onCollapse,
}: {
  conversation?: ConversationSummary
  lastMessage?: ConversationMessage
  onCollapse: () => void
}) {
  return (
    <>
      <div className="border-b border-white/10 p-4">
        <div className="flex items-center justify-between gap-3">
          <div>
            <p className="font-semibold">Konteks</p>
            <p className="text-sm text-zinc-400">{conversation?.display_name ?? '-'}</p>
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={onCollapse}
            aria-label="Tutup panel konteks"
            className="text-zinc-300 hover:bg-white/10 hover:text-white"
          >
            <PanelRightClose className="size-5" />
          </Button>
        </div>
      </div>

      <div className="space-y-4 p-4">
        <div className="rounded-lg border border-teal-500/20 bg-teal-500/10 p-3">
          <div className="mb-2 flex items-center gap-2">
            <Sparkles className="size-4 text-teal-300" />
            <p className="text-sm font-medium text-teal-100">Ringkasan cepat</p>
          </div>
          <p className="text-sm leading-relaxed text-zinc-300">
            {conversation
              ? 'Percakapan ini sudah tersambung ke database lokal.'
              : 'Pilih percakapan untuk melihat konteks.'}
          </p>
        </div>

        <div className="space-y-2">
          <Insight label="Pesan tersimpan" value={conversation?.message_count.toLocaleString('id-ID') ?? '0'} />
          <Insight label="Pesan terakhir" value={lastMessage ? formatShortTime(lastMessage.timestamp) : '-'} />
          <Insight label="Mode kirim" value="Tersimpan lokal" />
        </div>

        <div>
          <p className="mb-2 text-sm font-medium">Media</p>
          <div className="grid grid-cols-3 gap-2">
            {[0, 1, 2, 3, 4, 5].map((item) => (
              <div
                key={item}
                className="grid aspect-square place-items-center rounded-md border border-white/10 bg-white/[0.04]"
              >
                <Image className="size-4 text-zinc-500" />
              </div>
            ))}
          </div>
        </div>

        <div>
          <p className="mb-2 text-sm font-medium">Label</p>
          <div className="flex flex-wrap gap-2">
            {['database', 'lokal', 'chat'].map((tag) => (
              <Badge key={tag} variant="secondary" className="bg-white/10 text-zinc-200 hover:bg-white/10">
                {tag}
              </Badge>
            ))}
          </div>
        </div>
      </div>
    </>
  )
}

function Insight({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-white/10 bg-white/[0.03] p-3">
      <p className="text-xs text-zinc-500">{label}</p>
      <p className="mt-1 text-sm font-medium text-zinc-100">{value}</p>
    </div>
  )
}

function DayDivider({ label }: { label: string }) {
  return (
    <div className="my-3 flex justify-center">
      <span className="rounded-md bg-[#1d211f]/90 px-4 py-1 text-xs font-medium text-zinc-300 shadow">
        {label}
      </span>
    </div>
  )
}

function ChatPattern() {
  return (
    <div
      aria-hidden
      className="pointer-events-none absolute inset-0 opacity-[0.16]"
      style={{
        backgroundImage:
          'radial-gradient(circle at 1px 1px, rgba(255,255,255,0.55) 1px, transparent 0), linear-gradient(135deg, transparent 46%, rgba(255,255,255,0.22) 47%, rgba(255,255,255,0.22) 53%, transparent 54%)',
        backgroundSize: '26px 26px, 86px 86px',
      }}
    />
  )
}

function LoadingRows() {
  return (
    <div className="space-y-2">
      {[0, 1, 2, 3].map((i) => (
        <div key={i} className="h-[72px] animate-pulse rounded-md bg-white/[0.06]" />
      ))}
    </div>
  )
}

function EmptyList() {
  return (
    <div className="rounded-lg border border-dashed border-white/10 px-4 py-8 text-center text-sm text-zinc-400">
      Belum ada percakapan di database. Impor chat atau pilih kontak WhatsApp dulu.
    </div>
  )
}

function EmptyThread() {
  return (
    <div className="mx-auto mt-16 max-w-sm rounded-lg border border-white/10 bg-[#1d211f]/80 p-4 text-center text-sm text-zinc-400">
      Pilih percakapan dari daftar kiri.
    </div>
  )
}

function ThreadLoading() {
  return (
    <div className="mx-auto mt-16 flex items-center gap-2 rounded-lg bg-[#1d211f]/80 px-4 py-3 text-sm text-zinc-400">
      <Loader2 className="size-4 animate-spin" />
      Memuat pesan
    </div>
  )
}

function NoMessages() {
  return (
    <div className="mx-auto mt-16 max-w-sm rounded-lg border border-white/10 bg-[#1d211f]/80 p-4 text-center text-sm text-zinc-400">
      Percakapan ini belum punya pesan.
    </div>
  )
}

function groupMessagesByDay(messages: ConversationMessage[]) {
  return messages.reduce<Array<{ label: string; messages: ConversationMessage[] }>>(
    (groups, message) => {
      const label = formatDay(message.timestamp)
      const last = groups.at(-1)
      if (last?.label === label) {
        last.messages.push(message)
      } else {
        groups.push({ label, messages: [message] })
      }
      return groups
    },
    [],
  )
}

function initials(name: string) {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase()
}

function mediaLabel(type: string) {
  switch (type) {
    case 'image':
      return 'Foto'
    case 'audio':
      return 'Voice message'
    case 'video':
      return 'Video'
    case 'sticker':
      return 'Sticker'
    case 'document':
      return 'Dokumen'
    default:
      return ''
  }
}

function formatShortTime(iso?: string | null) {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const now = new Date()
  if (d.toDateString() === now.toDateString()) return formatMessageTime(iso)
  return d.toLocaleDateString('id-ID', { day: '2-digit', month: '2-digit' })
}

function formatMessageTime(iso: string) {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' })
}

function formatDay(iso: string) {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const now = new Date()
  const yesterday = new Date(now)
  yesterday.setDate(now.getDate() - 1)
  if (d.toDateString() === now.toDateString()) return 'Today'
  if (d.toDateString() === yesterday.toDateString()) return 'Yesterday'
  return d.toLocaleDateString('id-ID', {
    day: '2-digit',
    month: 'long',
    year: 'numeric',
  })
}
