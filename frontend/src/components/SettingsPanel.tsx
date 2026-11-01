import { useState } from "react"
import { ShieldCheck, KeyRound, Save, Trash2, Sun, Moon, RefreshCw } from "lucide-react"
import { listDeadJobs } from "../api/deadletter"
import { ApiError, getErrorMessage } from "../utils/errors"
import { getSessionToken, saveSessionCredentials, STORAGE_KEYS } from "../utils/storage"

interface SettingsPanelProps {
  theme: string
  onThemeChange: (theme: "light" | "dark") => void
  onCredentialsChange: () => void
}

export function SettingsPanel({ theme, onThemeChange, onCredentialsChange }: SettingsPanelProps) {
  const [writeToken, setWriteToken] = useState(() => getSessionToken(STORAGE_KEYS.WRITE_TOKEN))
  const [internalToken, setInternalToken] = useState(() => getSessionToken(STORAGE_KEYS.INTERNAL_TOKEN))
  const [message, setMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [checking, setChecking] = useState(false)

  const save = (clear = false) => {
    try {
      saveSessionCredentials(clear ? "" : writeToken, clear ? "" : internalToken)
      if (clear) { setWriteToken(""); setInternalToken("") }
      setError(null)
      setMessage(clear ? "Session credentials cleared." : "Credentials saved for this browser session.")
      onCredentialsChange()
    } catch (failure) {
      setError(getErrorMessage(failure, "Session storage is unavailable."))
    }
  }

  const checkAccess = async () => {
    setChecking(true)
    setError(null)
    setMessage(null)
    try {
      await listDeadJobs(1, internalToken)
      setMessage("Operator access verified.")
    } catch (failure) {
      setError(failure instanceof ApiError && failure.status === 401 ? "Operator credentials were not accepted." : getErrorMessage(failure, "Unable to verify access."))
    } finally {
      setChecking(false)
    }
  }

  return <section className="settings-panel" aria-label="Settings">
    <div className="history-header"><div><span className="workspace-eyebrow">PREFERENCES & ACCESS</span><h2>Settings</h2></div><ShieldCheck size={24} aria-hidden="true" /></div>
    <section className="settings-section" aria-labelledby="appearance-heading">
      <div><h3 id="appearance-heading">Appearance</h3></div>
      <div className="mode-segments" role="group" aria-label="Color scheme">
        <button type="button" className={`mode-option${theme === "light" ? " is-active" : ""}`} aria-pressed={theme === "light"} onClick={() => onThemeChange("light")}><Sun size={16} aria-hidden="true" /> Light</button>
        <button type="button" className={`mode-option${theme === "dark" ? " is-active" : ""}`} aria-pressed={theme === "dark"} onClick={() => onThemeChange("dark")}><Moon size={16} aria-hidden="true" /> Dark</button>
      </div>
    </section>
    <form className="settings-section settings-credentials" onSubmit={event => { event.preventDefault(); save() }}>
      <div><h3><KeyRound size={18} aria-hidden="true" /> Session credentials</h3><span className="field-caption">This browser tab only</span></div>
      <div className="settings-fields">
        <label>Write API token<input type="password" value={writeToken} onChange={event => setWriteToken(event.target.value)} autoComplete="off" spellCheck={false} /></label>
        <label>Internal operator token<input type="password" value={internalToken} onChange={event => setInternalToken(event.target.value)} autoComplete="off" spellCheck={false} /></label>
        <div className="settings-actions"><button type="submit" className="upload-primary-btn"><Save size={16} aria-hidden="true" /> Save session credentials</button><button type="button" className="icon-btn" onClick={() => save(true)}><Trash2 size={16} aria-hidden="true" /> Clear credentials</button><button type="button" className="icon-btn" onClick={() => void checkAccess()} disabled={checking}><RefreshCw size={16} aria-hidden="true" /> {checking ? "Checking..." : "Check operator access"}</button></div>
        {message && <p className="inline-success" role="status">{message}</p>}
        {error && <p className="inline-error" role="alert">{error}</p>}
      </div>
    </form>
  </section>
}