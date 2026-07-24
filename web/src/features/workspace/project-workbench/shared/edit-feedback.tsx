import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react"
import { AlertCircleIcon, CheckCircle2Icon, XIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

type FeedbackMessage = {
  id: number
  message: string
  tone: "success"
}

type FailedEdit = {
  id: number
  message: string
  title: string
}

type EditFeedbackValue = {
  failure: (title: string, message: string) => void
  success: (message: string) => void
}

const noopFeedback: EditFeedbackValue = {
  failure: () => {},
  success: () => {},
}

const EditFeedbackContext = createContext<EditFeedbackValue>(noopFeedback)

type EditFeedbackProviderProps = {
  children: ReactNode
  timeoutMs?: number
}

export function EditFeedbackProvider({
  children,
  timeoutMs = 2500,
}: EditFeedbackProviderProps) {
  const { t } = useTranslation()
  const [message, setMessage] = useState<FeedbackMessage | null>(null)
  const [failures, setFailures] = useState<FailedEdit[]>([])
  const nextIDRef = useRef(1)
  const feedback = useMemo<EditFeedbackValue>(
    () => ({
      failure: (title, nextMessage) => {
        const id = nextIDRef.current++
        setFailures((current) =>
          [
            { id, message: nextMessage, title },
            ...current.filter((item) => item.title !== title),
          ].slice(0, 3)
        )
      },
      success: (nextMessage) => {
        setMessage({
          id: Date.now(),
          message: nextMessage,
          tone: "success",
        })
      },
    }),
    []
  )

  useEffect(() => {
    if (!message) {
      return
    }
    const timer = window.setTimeout(() => setMessage(null), timeoutMs)
    return () => window.clearTimeout(timer)
  }, [message, timeoutMs])

  return (
    <EditFeedbackContext.Provider value={feedback}>
      {children}
      {message ? (
        <div
          className={cn(
            "fixed top-14 right-4 z-40 inline-flex max-w-[calc(100vw-2rem)] items-center gap-2 border bg-popover px-3 py-2 text-sm text-popover-foreground shadow-sm"
          )}
          role="status"
        >
          <CheckCircle2Icon className="size-4 text-primary" />
          <span className="truncate">{message.message}</span>
        </div>
      ) : null}
      {failures.length > 0 ? (
        <Alert
          className="fixed top-28 right-4 z-40 w-[min(24rem,calc(100vw-2rem))] bg-popover shadow-sm"
          variant="destructive"
        >
          <AlertCircleIcon className="size-4" />
          <AlertTitle>
            {t("common.unsavedEdits", { count: failures.length })}
          </AlertTitle>
          <AlertDescription>
            <ul className="mt-1 space-y-1">
              {failures.map((failure) => (
                <li
                  className="grid grid-cols-[1fr_auto] items-center gap-2 text-xs"
                  key={failure.id}
                >
                  <span className="min-w-0 truncate">
                    {t("common.failedEdit", {
                      title: failure.title,
                      message: failure.message,
                    })}
                  </span>
                  <Button
                    aria-label={t("common.dismissFailedEdit", {
                      title: failure.title,
                    })}
                    className="text-muted-foreground hover:text-foreground"
                    onClick={() =>
                      setFailures((current) =>
                        current.filter((item) => item.id !== failure.id)
                      )
                    }
                    size="icon-xs"
                    type="button"
                    variant="ghost"
                  >
                    <XIcon className="size-3" />
                  </Button>
                </li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      ) : null}
    </EditFeedbackContext.Provider>
  )
}

export function useEditFeedback(): EditFeedbackValue {
  return useContext(EditFeedbackContext)
}
