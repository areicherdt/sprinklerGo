import { ReactNode, createContext, useCallback, useContext, useRef, useState } from 'react'

// ---- Toasts ----

export type ToastKind = 'ok' | 'error'

interface Toast {
  id: number
  kind: ToastKind
  message: string
}

const ToastContext = createContext<(message: string, kind?: ToastKind) => void>(() => {})

/** useToast()("Gespeichert.") shows a transient notification. */
export function useToast() {
  return useContext(ToastContext)
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const nextID = useRef(1)

  const push = useCallback((message: string, kind: ToastKind = 'ok') => {
    const id = nextID.current++
    setToasts((prev) => [...prev, { id, kind, message }])
    setTimeout(() => setToasts((prev) => prev.filter((t) => t.id !== id)), 4000)
  }, [])

  return (
    <ToastContext.Provider value={push}>
      {children}
      <div className="toasts" role="status" aria-live="polite">
        {toasts.map((t) => (
          <div key={t.id} className={`toast ${t.kind}`}>
            {t.message}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}

// ---- Confirm dialog ----

export function ConfirmDialog({
  open,
  title,
  text,
  confirmLabel = 'Löschen',
  onConfirm,
  onCancel,
}: {
  open: boolean
  title: string
  text: string
  confirmLabel?: string
  onConfirm: () => void
  onCancel: () => void
}) {
  if (!open) return null
  return (
    <div className="dialog-backdrop" onClick={onCancel}>
      <div
        className="dialog"
        role="alertdialog"
        aria-modal="true"
        aria-label={title}
        onClick={(e) => e.stopPropagation()}
      >
        <h2>{title}</h2>
        <p>{text}</p>
        <div className="row" style={{ justifyContent: 'flex-end' }}>
          <button onClick={onCancel} autoFocus>
            Abbrechen
          </button>
          <button className="danger" onClick={onConfirm}>
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}

// ---- Stepper ----

export function Stepper({
  value,
  min,
  max,
  onChange,
  label,
}: {
  value: number
  min: number
  max: number
  onChange: (v: number) => void
  label?: string
}) {
  const clamp = (v: number) => Math.max(min, Math.min(max, v))
  return (
    <span className="stepper">
      <button type="button" aria-label="verringern" onClick={() => onChange(clamp(value - 1))}>
        −
      </button>
      <input
        type="number"
        min={min}
        max={max}
        value={value}
        aria-label={label}
        onChange={(e) => onChange(clamp(Number(e.target.value) || 0))}
      />
      <button type="button" aria-label="erhöhen" onClick={() => onChange(clamp(value + 1))}>
        +
      </button>
    </span>
  )
}
