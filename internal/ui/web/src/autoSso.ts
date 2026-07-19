import { useEffect, useState } from 'react'

const KEY = 'camunda-lab-auto-sso'

function readAutoSso(): boolean {
  const saved = localStorage.getItem(KEY)
  if (saved === '0' || saved === 'false') return false
  if (saved === '1' || saved === 'true') return true
  return true // default on
}

export function useAutoSso(): [boolean, (next: boolean) => void] {
  const [enabled, setEnabled] = useState(() => {
    if (typeof window === 'undefined') return true
    return readAutoSso()
  })

  useEffect(() => {
    localStorage.setItem(KEY, enabled ? '1' : '0')
  }, [enabled])

  return [enabled, setEnabled]
}
