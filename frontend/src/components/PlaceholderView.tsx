type Props = { title: string; description?: string }

export function PlaceholderView({ title, description }: Props) {
  return (
    <div className="flex h-full flex-col">
      <div className="shrink-0 border-b border-border/40 px-4 py-3">
        <h1 className="text-sm font-medium text-foreground">{title}</h1>
      </div>
      <div className="flex flex-1 items-center justify-center p-6">
        <div className="text-center">
          <p className="text-sm text-muted-foreground">{description ?? '功能开发中'}</p>
        </div>
      </div>
    </div>
  )
}
