/*
Per-User Group Ratio Override Editor (Phase 2)

显示该用户的所有分组（来自 group 字段拆分），每行允许填写一个倍率覆盖。
- 空字符串/未填写：该分组不覆盖（沿用全局或用户组级倍率）
- 填了具体数字：该分组对该用户使用此倍率
- 倍率必须 >= 0
*/
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getUserGroupRatios, setUserGroupRatios } from '../api'

type Props = {
  userId: number
  /** 用户被授权的分组列表 (从 user.group 逗号字段拆出来) */
  groupOptions: string[]
}

export function UserGroupRatioEditor({ userId, groupOptions }: Props) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  /** 用户填的字符串值（保留原始输入，提交时再转 number），空字符串 = 不覆盖 */
  const [ratiosInput, setRatiosInput] = useState<Record<string, string>>({})

  // 拉取现有 overrides
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    getUserGroupRatios(userId)
      .then((res) => {
        if (cancelled) return
        const data = res.data || {}
        const next: Record<string, string> = {}
        for (const [g, r] of Object.entries(data)) {
          next[g] = String(r)
        }
        setRatiosInput(next)
      })
      .catch(() => {
        // ignore (admin 才能调，无权限就静默)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [userId])

  const handleChange = (group: string, value: string) => {
    setRatiosInput((prev) => ({ ...prev, [group]: value }))
  }

  const handleSave = async () => {
    // 校验 + 构造 payload
    const payload: Record<string, number> = {}
    for (const [g, str] of Object.entries(ratiosInput)) {
      const trimmed = str.trim()
      if (trimmed === '') continue // 空 = 不覆盖
      const n = Number(trimmed)
      if (!isFinite(n) || n < 0) {
        toast.error(
          t('Invalid ratio for group {{group}}: {{value}}', {
            group: g,
            value: trimmed,
          })
        )
        return
      }
      payload[g] = n
    }
    setSaving(true)
    try {
      const res = await setUserGroupRatios(userId, payload)
      if (res.success) {
        toast.success(t('Group ratio overrides saved'))
      } else {
        toast.error(res.message || t('Failed to save'))
      }
    } catch (e: unknown) {
      toast.error((e as Error).message || t('Failed to save'))
    } finally {
      setSaving(false)
    }
  }

  if (groupOptions.length === 0) {
    return (
      <Card className='border-dashed'>
        <CardHeader className='pb-2'>
          <CardTitle className='text-sm'>
            {t('Per-Group Ratio Overrides')}
          </CardTitle>
          <CardDescription className='text-xs'>
            {t('Select at least one group above to configure ratio overrides.')}
          </CardDescription>
        </CardHeader>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader className='pb-3'>
        <CardTitle className='text-sm'>
          {t('Per-Group Ratio Overrides')}
        </CardTitle>
        <CardDescription className='text-xs'>
          {t(
            'Per-user override of group ratio. Leave empty to use the global / user-group ratio. Lower = cheaper (e.g. 0.8 = 20% off).'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-3'>
        {loading ? (
          <p className='text-muted-foreground text-xs'>{t('Loading...')}</p>
        ) : (
          groupOptions.map((g) => (
            <div key={g} className='flex items-center gap-2'>
              <Label className='w-32 truncate text-xs' title={g}>
                {g}
              </Label>
              <Input
                type='number'
                min='0'
                step='0.01'
                placeholder={t('default (no override)')}
                value={ratiosInput[g] ?? ''}
                onChange={(e) => handleChange(g, e.target.value)}
                className='h-8 text-sm'
              />
            </div>
          ))
        )}
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
      </CardContent>
    </Card>
  )
}
