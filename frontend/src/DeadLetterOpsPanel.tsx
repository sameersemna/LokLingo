import { useCallback, useEffect, useMemo, useState } from "react"
import { listDeadJobs, replayDeadJob, type DeadLetterJobItem } from "./api/deadletter"
import { getErrorMessage } from "./utils/errors"

type ToastFn = (msg: string, type: "success" | "error") => void

interface DeadLetterOpsPanelProps {
  onToast: ToastFn
}

const TOKEN_KEY = "loklingo-internal-token"
const AUTO_REFRESH_KEY = "loklingo-dead-ops-auto-refresh"

function shortTimestamp(raw?: string): string {
  if (!raw) return "-"
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleString()
}

function jobBadgeClass(status?: string): string {
  if (!status) return "dead-status"
  if (status === "failed") return "dead-status dead-status-failed"
  if (status === "pending") return "dead-status dead-status-pending"
  if (status === "processing") return "dead-status dead-status-processing"
  return "dead-status"
}

function safeParseTime(raw?: string): number {
  if (!raw) return 0
  const parsed = Date.parse(raw)
  return Number.isNaN(parsed) ? 0 : parsed
}

export function DeadLetterOpsPanel({ onToast }: DeadLetterOpsPanelProps) {
  const [token, setToken] = useState<string>(() => localStorage.getItem(TOKEN_KEY) ?? "")
  const [limit, setLimit] = useState(25)
  const [items, setItems] = useState<DeadLetterJobItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [replaying, setReplaying] = useState<string | null>(null)
  const [bulkReplayBusy, setBulkReplayBusy] = useState(false)
  const [bulkReplayProgress, setBulkReplayProgress] = useState<string | null>(null)
  const [statusFilter, setStatusFilter] = useState("all")
  const [typeFilter, setTypeFilter] = useState("all")
  const [query, setQuery] = useState("")
  const [sortOrder, setSortOrder] = useState<"newest" | "oldest">("newest")
  const [autoRefresh, setAutoRefresh] = useState<boolean>(() => localStorage.getItem(AUTO_REFRESH_KEY) !== "0")

  const hasToken = useMemo(() => token.trim().length > 0, [token])

  useEffect(() => {
    localStorage.setItem(TOKEN_KEY, token)
  }, [token])

  useEffect(() => {
    localStorage.setItem(AUTO_REFRESH_KEY, autoRefresh ? "1" : "0")
  }, [autoRefresh])

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await listDeadJobs(limit, token)
      setItems(data.jobs)
    } catch (err) {
      setError(getErrorMessage(err, "Failed to load dead-letter jobs"))
    } finally {
      setLoading(false)
    }
  }, [limit, token])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    if (!autoRefresh) {
      return
    }
    const interval = setInterval(() => {
      void load()
    }, 30_000)
    return () => clearInterval(interval)
  }, [autoRefresh, load])

  const visibleItems = useMemo(() => {
    const q = query.trim().toLowerCase()
    const filtered = items.filter(item => {
      if (statusFilter !== "all" && (item.status ?? "") !== statusFilter) {
        return false
      }
      if (typeFilter !== "all" && (item.type ?? "") !== typeFilter) {
        return false
      }
      if (!q) {
        return true
      }
      const haystack = [
        item.job_id,
        item.reason ?? "",
        item.status ?? "",
        item.type ?? "",
        item.mode ?? "",
        item.source ?? "",
        item.target ?? "",
      ].join(" ").toLowerCase()
      return haystack.includes(q)
    })

    return filtered.sort((a, b) => {
      const left = safeParseTime(a.dead_lettered_at)
      const right = safeParseTime(b.dead_lettered_at)
      return sortOrder === "newest" ? right - left : left - right
    })
  }, [items, query, sortOrder, statusFilter, typeFilter])

  const statusCounts = useMemo(() => {
    const counts = new Map<string, number>()
    for (const item of items) {
      const key = item.status ?? "unknown"
      counts.set(key, (counts.get(key) ?? 0) + 1)
    }
    return counts
  }, [items])

  const typeCounts = useMemo(() => {
    const counts = new Map<string, number>()
    for (const item of items) {
      const key = item.type ?? "unknown"
      counts.set(key, (counts.get(key) ?? 0) + 1)
    }
    return counts
  }, [items])

  const refresh = useCallback(() => {
    void load()
  }, [load])

  const handleReplayMany = useCallback(async (jobIds: string[]) => {
    if (jobIds.length === 0) {
      return
    }
    const proceed = window.confirm(
      `Replay ${jobIds.length} filtered dead-letter job${jobIds.length === 1 ? "" : "s"}?`,
    )
    if (!proceed) {
      return
    }
    setBulkReplayBusy(true)
    setBulkReplayProgress(`0/${jobIds.length}`)

    let successCount = 0
    let failureCount = 0
    const failedIds: string[] = []

    for (let index = 0; index < jobIds.length; index++) {
      const jobId = jobIds[index]
      setBulkReplayProgress(`${index + 1}/${jobIds.length}`)
      try {
        await replayDeadJob(jobId, token)
        successCount++
        setItems(prev => prev.filter(item => item.job_id !== jobId))
      } catch {
        failureCount++
        failedIds.push(jobId)
      }
    }

    if (successCount > 0 && failureCount === 0) {
      onToast(`Replayed ${successCount} dead-letter jobs`, "success")
    } else if (successCount > 0) {
      onToast(`Replayed ${successCount} jobs, ${failureCount} failed`, "success")
    } else {
      onToast("Bulk replay failed", "error")
    }
    if (failedIds.length > 0) {
      setError(`Failed to replay: ${failedIds.join(", ")}`)
    }

    setBulkReplayBusy(false)
    setBulkReplayProgress(null)
  }, [onToast, token])

  const handleReplay = useCallback(async (jobId: string) => {
    setReplaying(jobId)
    try {
      const replayed = await replayDeadJob(jobId, token)
      onToast(`Replayed ${replayed.job_id}`, "success")
      setItems(prev => prev.filter(item => item.job_id !== jobId))
    } catch (err) {
      onToast(getErrorMessage(err, "Replay failed"), "error")
    } finally {
      setReplaying(null)
    }
  }, [onToast, token])

  return (
    <section className="dead-ops-panel" aria-live="polite">
      <div className="dead-ops-header">
        <div>
          <h2>Recovery queue</h2>
          <p>Inspect failed jobs and replay them safely.</p>
        </div>
        <div className="dead-ops-actions">
          <button
            type="button"
            className={`icon-btn dead-auto-refresh-btn${autoRefresh ? " is-active" : ""}`}
            onClick={() => setAutoRefresh(v => !v)}
            aria-pressed={autoRefresh}
            title="Toggle auto-refresh every 30 seconds"
          >
            {autoRefresh ? "⟳ Auto-refresh on" : "⟳ Auto-refresh off"}
          </button>
          <label>
            Limit
            <select value={limit} onChange={e => setLimit(Number(e.target.value))}>
              <option value={10}>10</option>
              <option value={25}>25</option>
              <option value={50}>50</option>
              <option value={100}>100</option>
            </select>
          </label>
          <button type="button" className="icon-btn" onClick={refresh} disabled={loading || replaying !== null}>
            {loading ? "Refreshing..." : "Refresh"}
          </button>
          <button
            type="button"
            className="icon-btn"
            onClick={() => void handleReplayMany(visibleItems.map(item => item.job_id))}
            disabled={loading || bulkReplayBusy || replaying !== null || visibleItems.length === 0}
            title="Replay all currently filtered dead-letter jobs"
          >
            {bulkReplayBusy
              ? `Replaying ${bulkReplayProgress ?? "..."}`
              : `Replay filtered (${visibleItems.length})`}
          </button>
        </div>
      </div>

      <div className="dead-ops-token-row">
        <label htmlFor="dead-ops-token">X-Internal-Token</label>
        <input
          id="dead-ops-token"
          type="password"
          placeholder="Optional in dev, required in secured env"
          value={token}
          onChange={e => setToken(e.target.value)}
          autoComplete="off"
          spellCheck={false}
        />
        <span className={`dead-token-state ${hasToken ? "dead-token-set" : "dead-token-empty"}`}>
          {hasToken ? "Token set" : "No token"}
        </span>
      </div>

      <div className="dead-ops-filters">
        <label>
          Search
          <input
            type="search"
            value={query}
            onChange={e => setQuery(e.target.value)}
            placeholder="job id, reason, type, source, target"
          />
        </label>
        <label>
          Status
          <select value={statusFilter} onChange={e => setStatusFilter(e.target.value)}>
            <option value="all">All ({items.length})</option>
            <option value="failed">Failed ({statusCounts.get("failed") ?? 0})</option>
            <option value="pending">Pending ({statusCounts.get("pending") ?? 0})</option>
            <option value="processing">Processing ({statusCounts.get("processing") ?? 0})</option>
            <option value="unknown">Unknown ({statusCounts.get("unknown") ?? 0})</option>
          </select>
        </label>
        <label>
          Type
          <select value={typeFilter} onChange={e => setTypeFilter(e.target.value)}>
            <option value="all">All ({items.length})</option>
            <option value="translate_pdf">PDF ({typeCounts.get("translate_pdf") ?? 0})</option>
            <option value="translate_image">Image ({typeCounts.get("translate_image") ?? 0})</option>
            <option value="text">Text ({typeCounts.get("text") ?? 0})</option>
            <option value="unknown">Unknown ({typeCounts.get("unknown") ?? 0})</option>
          </select>
        </label>
        <label>
          Sort
          <select value={sortOrder} onChange={e => setSortOrder(e.target.value as "newest" | "oldest")}> 
            <option value="newest">Newest first</option>
            <option value="oldest">Oldest first</option>
          </select>
        </label>
      </div>

      <div className="dead-ops-summary">
        Showing {visibleItems.length} of {items.length} jobs
        {query.trim() ? ` matching “${query.trim()}”` : ""}
      </div>

      {error && <p className="dead-ops-error">{error}</p>}
      {!error && items.length === 0 && !loading && (
        <p className="dead-ops-empty">No dead-letter jobs found for this limit.</p>
      )}
      {!error && items.length > 0 && visibleItems.length === 0 && !loading && (
        <p className="dead-ops-empty">No dead-letter jobs match the current filters.</p>
      )}

      {visibleItems.length > 0 && (
        <div className="dead-ops-table-wrap">
          <table className="dead-ops-table">
            <thead>
              <tr>
                <th>Job</th>
                <th>Status</th>
                <th>Type</th>
                <th>Lang</th>
                <th>Attempt</th>
                <th>Reason</th>
                <th>Dead lettered</th>
                <th>Action</th>
              </tr>
            </thead>
            <tbody>
              {visibleItems.map(item => {
                const replayBusy = replaying === item.job_id
                return (
                  <tr key={item.job_id}>
                    <td className="dead-job-id" title={item.job_id}>{item.job_id}</td>
                    <td><span className={jobBadgeClass(item.status)}>{item.status ?? "-"}</span></td>
                    <td>{item.type ?? "-"}</td>
                    <td>{item.source && item.target ? `${item.source} -> ${item.target}` : "-"}</td>
                    <td>{item.attempt ?? 0}/{item.max_attempts ?? 0}</td>
                    <td className="dead-reason" title={item.reason ?? ""}>{item.reason ?? "-"}</td>
                    <td>{shortTimestamp(item.dead_lettered_at)}</td>
                    <td>
                      <button
                        type="button"
                        className="icon-btn"
                        onClick={() => handleReplay(item.job_id)}
                        disabled={replayBusy || replaying !== null}
                      >
                        {replayBusy ? "Replaying..." : "Replay"}
                      </button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
