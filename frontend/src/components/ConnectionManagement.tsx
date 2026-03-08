import { useState } from 'react'
import { Plus, Pencil, Trash2, Play, Check, Loader2, Wifi } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { Connection } from '../../bindings/rocket-leaf/internal/model/models.js'
import { ConnectionStatus } from '../../bindings/rocket-leaf/internal/model/models.js'
import * as connectionApi from '@/api/connection'

type FormState = {
  id: number | null
  name: string
  env: string
  nameServer: string
  timeoutSec: number
  enableACL: boolean
  accessKey: string
  secretKey: string
  remark: string
}

const emptyForm: FormState = {
  id: null,
  name: '',
  env: '开发',
  nameServer: '',
  timeoutSec: 10,
  enableACL: false,
  accessKey: '',
  secretKey: '',
  remark: '',
}

type Props = {
  list: Connection[]
  loading: boolean
  error: string | null
  onRefresh: () => void
  onConnect: (id: number) => void
  connectingId: number | null
}

export function ConnectionManagement({ list, loading, error, onRefresh, onConnect, connectingId }: Props) {
  const [form, setForm] = useState<FormState>(emptyForm)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)
  const [testingId, setTestingId] = useState<number | null>(null)

  const openAdd = () => {
    setForm(emptyForm)
    setActionError(null)
    setDialogOpen(true)
  }
  const openEdit = (c: Connection) => {
    setForm({
      id: c.id,
      name: c.name,
      env: c.env ?? '开发',
      nameServer: c.nameServer ?? '',
      timeoutSec: c.timeoutSec ?? 10,
      enableACL: c.enableACL ?? false,
      accessKey: c.accessKey ?? '',
      secretKey: c.secretKey ?? '',
      remark: c.remark ?? '',
    })
    setActionError(null)
    setDialogOpen(true)
  }
  const closeDialog = () => {
    setDialogOpen(false)
    setForm(emptyForm)
    setActionError(null)
  }

  const submit = async () => {
    if (!form.name.trim() || !form.nameServer.trim()) {
      setActionError('请填写名称和 NameServer 地址')
      return
    }
    setSubmitting(true)
    setActionError(null)
    try {
      if (form.id != null) {
        await connectionApi.updateConnection(
          form.id,
          form.name.trim(),
          form.env,
          form.nameServer.trim(),
          form.timeoutSec,
          form.enableACL,
          form.accessKey,
          form.secretKey,
          form.remark.trim()
        )
      } else {
        await connectionApi.addConnection(
          form.name.trim(),
          form.env,
          form.nameServer.trim(),
          form.timeoutSec,
          form.enableACL,
          form.accessKey,
          form.secretKey,
          form.remark.trim()
        )
      }
      onRefresh()
      closeDialog()
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e))
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (id: number) => {
    if (!window.confirm('确定删除该连接？')) return
    try {
      await connectionApi.deleteConnection(id)
      onRefresh()
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e))
    }
  }

  const handleSetDefault = async (id: number) => {
    try {
      await connectionApi.setDefaultConnection(id)
      onRefresh()
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e))
    }
  }

  const handleTest = async (id: number) => {
    setTestingId(id)
    setActionError(null)
    try {
      const msg = await connectionApi.testConnection(id)
      alert(msg || '连接成功')
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e))
    } finally {
      setTestingId(null)
    }
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center justify-between border-b border-border/40 px-4 py-3">
        <h1 className="text-sm font-medium text-foreground">连接管理</h1>
        <button
          type="button"
          onClick={openAdd}
          className={cn(
            'inline-flex items-center gap-1.5 rounded-md border border-border/50 px-3 py-1.5 text-sm font-medium hover:bg-accent'
          )}
        >
          <Plus className="h-4 w-4" />
          添加连接
        </button>
      </div>
      <div className="flex-1 overflow-y-auto scroll-thin p-4">
        {actionError && (
          <div className="mb-3 rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {actionError}
          </div>
        )}
        {loading ? (
          <div className="flex items-center justify-center py-12 text-muted-foreground">加载中…</div>
        ) : error ? (
          <div className="rounded-md border border-border/50 bg-muted/50 px-4 py-3 text-sm text-muted-foreground">
            {error}
          </div>
        ) : list.length === 0 ? (
          <p className="text-sm text-muted-foreground">暂无连接，点击「添加连接」创建。</p>
        ) : (
          <ul className="space-y-1">
            {list.map((c) => (
              <li
                key={c.id}
                className={cn(
                  'flex items-center justify-between gap-2 rounded-md border border-border/40 px-3 py-2',
                  c.isDefault && 'ring-1 ring-primary/30'
                )}
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-foreground">{c.name}</span>
                    {c.isDefault && (
                      <span className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">
                        默认
                      </span>
                    )}
                    <span
                      className={cn(
                        'rounded px-1.5 py-0.5 text-xs',
                        c.status === ConnectionStatus.StatusOnline
                          ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'
                          : 'bg-muted text-muted-foreground'
                      )}
                    >
                      {c.status === ConnectionStatus.StatusOnline ? '已连接' : '未连接'}
                    </span>
                  </div>
                  <p className="mt-0.5 truncate text-xs text-muted-foreground">{c.nameServer}</p>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  {c.status !== ConnectionStatus.StatusOnline && (
                    <button
                      type="button"
                      onClick={() => onConnect(c.id)}
                      disabled={connectingId !== null}
                      className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-50"
                      title="连接"
                    >
                      {connectingId === c.id ? <Loader2 className="h-4 w-4 animate-spin" /> : <Wifi className="h-4 w-4" />}
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={() => handleTest(c.id)}
                    disabled={testingId === c.id}
                    className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-50"
                    title="测试连接"
                  >
                    {testingId === c.id ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
                  </button>
                  {!c.isDefault && (
                    <button
                      type="button"
                      onClick={() => handleSetDefault(c.id)}
                      className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                      title="设为默认"
                    >
                      <Check className="h-4 w-4" />
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={() => openEdit(c)}
                    className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                    title="编辑"
                  >
                    <Pencil className="h-4 w-4" />
                  </button>
                  <button
                    type="button"
                    onClick={() => handleDelete(c.id)}
                    className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-destructive"
                    title="删除"
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>

      {dialogOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={closeDialog}>
          <div
            className="w-full max-w-md rounded-md border border-border/50 bg-card p-4 shadow-sm"
            onClick={(e) => e.stopPropagation()}
          >
            <h2 className="text-sm font-medium text-card-foreground">{form.id == null ? '添加连接' : '编辑连接'}</h2>
            <div className="mt-4 space-y-3">
              <div>
                <label id="conn-name-label" className="mb-1 block text-xs text-muted-foreground">名称</label>
                <input
                  id="conn-name"
                  aria-labelledby="conn-name-label"
                  value={form.name}
                  onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                  placeholder="例如：生产集群"
                />
              </div>
              <div>
                <label id="conn-env-label" className="mb-1 block text-xs text-muted-foreground">环境</label>
                <select
                  id="conn-env"
                  aria-labelledby="conn-env-label"
                  value={form.env}
                  onChange={(e) => setForm((f) => ({ ...f, env: e.target.value }))}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                >
                  <option value="开发">开发</option>
                  <option value="测试">测试</option>
                  <option value="生产">生产</option>
                </select>
              </div>
              <div>
                <label id="conn-nameserver-label" className="mb-1 block text-xs text-muted-foreground">NameServer 地址</label>
                <input
                  id="conn-nameserver"
                  aria-labelledby="conn-nameserver-label"
                  value={form.nameServer}
                  onChange={(e) => setForm((f) => ({ ...f, nameServer: e.target.value }))}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                  placeholder="127.0.0.1:9876"
                />
              </div>
              <div>
                <label id="conn-timeout-label" className="mb-1 block text-xs text-muted-foreground">超时(秒)</label>
                <input
                  id="conn-timeout"
                  aria-labelledby="conn-timeout-label"
                  type="number"
                  min={1}
                  value={form.timeoutSec}
                  onChange={(e) => setForm((f) => ({ ...f, timeoutSec: Number(e.target.value) || 10 }))}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                  placeholder="10"
                />
              </div>
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={form.enableACL}
                  onChange={(e) => setForm((f) => ({ ...f, enableACL: e.target.checked }))}
                  className="rounded border-input"
                />
                <span className="text-xs text-muted-foreground">启用 ACL</span>
              </label>
              {form.enableACL && (
                <>
                  <div>
                    <label id="conn-accesskey-label" className="mb-1 block text-xs text-muted-foreground">AccessKey</label>
                    <input
                      id="conn-accesskey"
                      aria-labelledby="conn-accesskey-label"
                      value={form.accessKey}
                      onChange={(e) => setForm((f) => ({ ...f, accessKey: e.target.value }))}
                      className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                      placeholder="ACL AccessKey"
                    />
                  </div>
                  <div>
                    <label id="conn-secretkey-label" className="mb-1 block text-xs text-muted-foreground">SecretKey</label>
                    <input
                      id="conn-secretkey"
                      aria-labelledby="conn-secretkey-label"
                      type="password"
                      value={form.secretKey}
                      onChange={(e) => setForm((f) => ({ ...f, secretKey: e.target.value }))}
                      className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                      placeholder="ACL SecretKey"
                    />
                  </div>
                </>
              )}
              <div>
                <label id="conn-remark-label" className="mb-1 block text-xs text-muted-foreground">备注</label>
                <input
                  id="conn-remark"
                  aria-labelledby="conn-remark-label"
                  value={form.remark}
                  onChange={(e) => setForm((f) => ({ ...f, remark: e.target.value }))}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                  placeholder="可选"
                />
              </div>
            </div>
            {actionError && (
              <p className="mt-2 text-sm text-destructive">{actionError}</p>
            )}
            <div className="mt-4 flex justify-end gap-2">
              <button
                type="button"
                onClick={closeDialog}
                className="rounded-md border border-border/50 px-3 py-1.5 text-sm hover:bg-accent"
              >
                取消
              </button>
              <button
                type="button"
                onClick={submit}
                disabled={submitting}
                className="rounded-md bg-primary px-3 py-1.5 text-sm text-primary-foreground hover:opacity-90 disabled:opacity-50"
              >
                {submitting ? '保存中…' : '保存'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
