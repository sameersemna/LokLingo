export type ReliabilityLevel = "normal" | "warn" | "critical"

export interface CheckpointThresholds {
  hitWarn: number
  hitCritical: number
  persistFailureWarn: number
  persistFailureCritical: number
}

export function safePercent(numerator: number, denominator: number): number {
  if (!Number.isFinite(numerator) || !Number.isFinite(denominator) || denominator <= 0) {
    return 0
  }
  return (numerator / denominator) * 100
}

export function formatPercent(value: number): string {
  const clamped = Math.max(0, Math.min(100, value))
  return `${clamped.toFixed(1)}%`
}

export function checkpointHitRateLevel(rate: number, thresholds: CheckpointThresholds): ReliabilityLevel {
  if (rate < thresholds.hitCritical) return "critical"
  if (rate < thresholds.hitWarn) return "warn"
  return "normal"
}

export function checkpointPersistFailureRateLevel(rate: number, thresholds: CheckpointThresholds): ReliabilityLevel {
  if (rate >= thresholds.persistFailureCritical) return "critical"
  if (rate >= thresholds.persistFailureWarn) return "warn"
  return "normal"
}

export function levelLabel(level: ReliabilityLevel): string {
  if (level === "critical") return "critical"
  if (level === "warn") return "warn"
  return "normal"
}

export function checkpointOperatorHint(
  hitRate: number,
  persistFailureRate: number,
  thresholds: CheckpointThresholds,
): { level: ReliabilityLevel; message: string; hitLevel: ReliabilityLevel; persistLevel: ReliabilityLevel } {
  const hitLevel = checkpointHitRateLevel(hitRate, thresholds)
  const persistLevel = checkpointPersistFailureRateLevel(persistFailureRate, thresholds)

  if (hitLevel === "critical") {
    return {
      level: "critical",
      hitLevel,
      persistLevel,
      message: `Critical: checkpoint hit rate ${formatPercent(hitRate)} is below ${thresholds.hitCritical}% threshold.`,
    }
  }
  if (persistLevel === "critical") {
    return {
      level: "critical",
      hitLevel,
      persistLevel,
      message: `Critical: checkpoint persist failure rate ${formatPercent(persistFailureRate)} is at or above ${thresholds.persistFailureCritical}%.`,
    }
  }
  if (hitLevel === "warn") {
    return {
      level: "warn",
      hitLevel,
      persistLevel,
      message: `Warn: checkpoint hit rate ${formatPercent(hitRate)} is below ${thresholds.hitWarn}%.`,
    }
  }
  if (persistLevel === "warn") {
    return {
      level: "warn",
      hitLevel,
      persistLevel,
      message: `Warn: checkpoint persist failure rate ${formatPercent(persistFailureRate)} is at or above ${thresholds.persistFailureWarn}%.`,
    }
  }
  return {
    level: "normal",
    hitLevel,
    persistLevel,
    message: `Normal: hit rate >= ${thresholds.hitWarn}% and persist failure rate < ${thresholds.persistFailureWarn}%.`,
  }
}
