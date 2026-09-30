import { useState } from 'react'
import { Copy } from '@phosphor-icons/react'
import { Button } from './Button'
import { useT } from '../i18n'

/** Copia `text` e mostra "copiado" por um instante no lugar do ícone sozinho. */
export function CopyButton({ text, label }: { text: string; label: string }) {
  const t = useT('common')
  const [done, setDone] = useState(false)
  return (
    <Button
      variant="ghost"
      size="sm"
      aria-label={label}
      icon={<Copy size={14} />}
      onClick={() => {
        void navigator.clipboard.writeText(text).then(() => {
          setDone(true)
          setTimeout(() => setDone(false), 1200)
        })
      }}
    >
      {done ? t('copied') : undefined}
    </Button>
  )
}
