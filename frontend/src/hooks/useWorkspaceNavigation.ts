import { useCallback } from "react"
import { useLocation, useNavigate } from "react-router"
import { WORKSPACE_PATHS, type InputWorkflow, type WorkspaceView } from "../constants"

export function useWorkspaceNavigation() {
  const location = useLocation()
  const navigate = useNavigate()
  const pathname = location.pathname.replace(/\/+$/, "") || "/"
  const translationMatch = /^\/translate\/(image|pdf|text)$/.exec(pathname)
  const jobMatch = /^\/jobs\/([^/]+)$/.exec(pathname)
  const routeState = location.state as { workflow?: string } | null
  const previousWorkflow = routeState?.workflow
  const workflow: InputWorkflow = translationMatch
    ? translationMatch[1] as InputWorkflow
    : previousWorkflow === "text" || previousWorkflow === "pdf" ? previousWorkflow : "image"
  const view: WorkspaceView = pathname === "/" || translationMatch
    ? "translate"
    : jobMatch ? "job-detail"
    : Object.entries(WORKSPACE_PATHS).find(([, path]) => path === pathname)?.[0] as WorkspaceView ?? "not-found"

  const openView = useCallback((nextView: Exclude<WorkspaceView, "not-found" | "job-detail">, nextWorkflow = workflow) => {
    const nextPath = nextView === "translate" ? `/translate/${nextWorkflow}` : WORKSPACE_PATHS[nextView]
    if (nextPath === location.pathname) return
    void navigate({ pathname: nextPath, search: location.search }, { state: { workflow: nextWorkflow } })
  }, [location.pathname, location.search, navigate, workflow])

  const setWorkflow = useCallback((nextWorkflow: InputWorkflow) => openView("translate", nextWorkflow), [openView])
  const openJob = useCallback((jobId: string) => {
    void navigate({ pathname: `/jobs/${encodeURIComponent(jobId)}`, search: location.search }, { state: { workflow } })
  }, [location.search, navigate, workflow])

  return { view, workflow, openView, setWorkflow, openJob, jobId: jobMatch?.[1] ?? null }
}