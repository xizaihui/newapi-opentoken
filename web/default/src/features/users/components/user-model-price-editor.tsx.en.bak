/*
Per-User Model Price Override Editor (Phase 3)

显示该用户已配置的模型按次价格覆盖，每行允许设置 (model, price, note)。
- 模型从全局 pricing 列表中选取（仅展示全局已配置 model_price 的模型）
- price 必须 >= 0；price > 全局价 10 倍时弹确认避免误操作
- 用户级覆盖优先级 > 全局价格
*/
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Trash2, Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  getUserModelPrices,
  setUserModelPrices,
  getPricing,
  type UserModelPrice,
  type PricingItem,
} from '../api'

type Props = {
  userId: number
}

type Row = {
  model_name: string
  price: string // 保留字符串以便用户中间态输入
  note: string
}

export function UserModelPriceEditor({ userId }: Props) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [rows, setRows] = useState<Row[]>([])
  const [allModels, setAllModels] = useState<PricingItem[]>([])
  const [addingModel, setAddingModel] = useState<string>('')

  // 拉取数据
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    Promise.all([getUserModelPrices(userId), getPricing()])
      .then(([overridesRes, pricingRes]) => {
        if (cancelled) return
        const overrides = (overridesRes.data || []) as UserModelPrice[]
        setRows(
          overrides.map((o) => ({
            model_name: o.model_name,
            price: String(o.price),
            note: o.note ?? '',
          }))
        )
        const items = (pricingRes.data || []) as PricingItem[]
        // 仅展示有 model_price 的模型（按次计费），与本功能语义一致
        setAllModels(items.filter((i) => (i.model_price ?? 0) > 0))
      })
      .catch(() => {
        // 静默：admin 才能调
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [userId])

  const modelPriceMap = useMemo(() => {
    const m: Record<string, number> = {}
    for (const it of allModels) {
      if (it.model_price !== undefined) m[it.model_name] = it.model_price
    }
    return m
  }, [allModels])

  const usedModels = useMemo(
    () => new Set(rows.map((r) => r.model_name)),
    [rows]
  )

  const handleAdd = () => {
    if (!addingModel) {
      toast.error(t('Please pick a model'))
      return
    }
    if (usedModels.has(addingModel)) {
      toast.error(t('Model already in override list'))
      return
    }
    const globalPrice = modelPriceMap[addingModel] ?? 0
    setRows((prev) => [
      ...prev,
      {
        model_name: addingModel,
        price: String(globalPrice), // 默认填全局价
        note: '',
      },
    ])
    setAddingModel('')
  }

  const handleChangeRow = (idx: number, key: keyof Row, value: string) => {
    setRows((prev) => prev.map((r, i) => (i === idx ? { ...r, [key]: value } : r)))
  }

  const handleRemoveRow = (idx: number) => {
    setRows((prev) => prev.filter((_, i) => i !== idx))
  }

  const handleSave = async () => {
    // 校验
    const payload: Array<{ model_name: string; price: number; note?: string }> = []
    for (const r of rows) {
      const trimmed = r.price.trim()
      if (trimmed === '') {
        toast.error(
          t('Price for model {{model}} is empty', { model: r.model_name })
        )
        return
      }
      const n = Number(trimmed)
      if (!isFinite(n) || n < 0) {
        toast.error(
          t('Invalid price for model {{model}}: {{value}}', {
            model: r.model_name,
            value: trimmed,
          })
        )
        return
      }
      // 防误操作：价格 > 全局价 * 10 弹确认
      const globalPrice = modelPriceMap[r.model_name] ?? 0
      if (globalPrice > 0 && n > globalPrice * 10) {
        const ok = window.confirm(
          t(
            'WARNING: Setting {{model}} to {{newPrice}} which is {{ratio}}x the global price ({{globalPrice}}). Confirm?',
            {
              model: r.model_name,
              newPrice: n,
              ratio: (n / globalPrice).toFixed(1),
              globalPrice: globalPrice,
            }
          )
        )
        if (!ok) return
      }
      payload.push({
        model_name: r.model_name,
        price: n,
        note: r.note?.trim() || undefined,
      })
    }

    setSaving(true)
    try {
      const res = await setUserModelPrices(userId, payload)
      if (res.success) {
        toast.success(t('Model price overrides saved'))
      } else {
        toast.error(res.message || t('Failed to save'))
      }
    } catch (e: unknown) {
      toast.error((e as Error).message || t('Failed to save'))
    } finally {
      setSaving(false)
    }
  }

  const availableModels = useMemo(
    () => allModels.filter((m) => !usedModels.has(m.model_name)),
    [allModels, usedModels]
  )

  return (
    <Card>
      <CardHeader className='pb-3'>
        <CardTitle className='text-sm'>
          {t('Per-Model Price Overrides')}
        </CardTitle>
        <CardDescription className='text-xs'>
          {t(
            'Per-user override of per-call model price. Highest priority — wins over global price. Will still be multiplied by the effective group ratio.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-3'>
        {loading ? (
          <p className='text-muted-foreground text-xs'>{t('Loading...')}</p>
        ) : (
          <>
            {rows.length === 0 ? (
              <p className='text-muted-foreground text-xs'>
                {t('No overrides. Use the picker below to add one.')}
              </p>
            ) : (
              <div className='space-y-2'>
                {rows.map((r, idx) => {
                  const globalPrice = modelPriceMap[r.model_name]
                  return (
                    <div
                      key={r.model_name}
                      className='flex items-start gap-2'
                    >
                      <div className='flex-1 min-w-0'>
                        <div className='truncate text-sm font-medium' title={r.model_name}>
                          {r.model_name}
                        </div>
                        {globalPrice !== undefined && (
                          <div className='text-muted-foreground text-xs'>
                            {t('Global: {{p}}', { p: globalPrice })}
                          </div>
                        )}
                      </div>
                      <Input
                        type='number'
                        min='0'
                        step='0.01'
                        placeholder={t('price')}
                        value={r.price}
                        onChange={(e) =>
                          handleChangeRow(idx, 'price', e.target.value)
                        }
                        className='h-8 w-24 text-sm'
                      />
                      <Input
                        placeholder={t('note')}
                        value={r.note}
                        onChange={(e) =>
                          handleChangeRow(idx, 'note', e.target.value)
                        }
                        className='h-8 w-32 text-sm'
                      />
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        onClick={() => handleRemoveRow(idx)}
                        className='h-8 w-8 p-0'
                        aria-label={t('Remove')}
                      >
                        <Trash2 className='h-4 w-4' />
                      </Button>
                    </div>
                  )
                })}
              </div>
            )}

            <div className='flex items-center gap-2 border-t pt-3'>
              <Select value={addingModel} onValueChange={setAddingModel}>
                <SelectTrigger className='h-8 flex-1 text-sm'>
                  <SelectValue placeholder={t('Pick a model to override...')} />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {availableModels.length === 0 ? (
                      <div className='text-muted-foreground px-2 py-1 text-xs'>
                        {t('No more models')}
                      </div>
                    ) : (
                      availableModels.map((m) => (
                        <SelectItem key={m.model_name} value={m.model_name}>
                          {m.model_name}
                          <span className='text-muted-foreground ml-2 text-xs'>
                            ({m.model_price})
                          </span>
                        </SelectItem>
                      ))
                    )}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <Button
                type='button'
                size='sm'
                variant='outline'
                onClick={handleAdd}
                disabled={!addingModel}
              >
                <Plus className='mr-1 h-4 w-4' />
                {t('Add')}
              </Button>
            </div>

            <div className='flex justify-end pt-2'>
              <Button
                type='button'
                size='sm'
                onClick={handleSave}
                disabled={loading || saving}
              >
                {saving ? t('Saving...') : t('Save Overrides')}
              </Button>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
