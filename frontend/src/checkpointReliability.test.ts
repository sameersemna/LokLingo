import { describe, expect, it } from "vitest"
import { renderToStaticMarkup } from "react-dom/server"
import React from "react"

import {
  checkpointHitRateLevel,
  checkpointOperatorHint,
  checkpointPersistFailureRateLevel,
  formatPercent,
  safePercent,
  type CheckpointThresholds,
} from "./checkpointReliability"

const thresholds: CheckpointThresholds = {
  hitWarn: 70,
  hitCritical: 40,
  persistFailureWarn: 5,
  persistFailureCritical: 15,
}

describe("checkpointReliability", () => {
  it("safePercent returns 0 when denominator is non-positive", () => {
    expect(safePercent(5, 0)).toBe(0)
    expect(safePercent(5, -1)).toBe(0)
  })

  it("formatPercent clamps and formats to one decimal", () => {
    expect(formatPercent(12.345)).toBe("12.3%")
    expect(formatPercent(123)).toBe("100.0%")
    expect(formatPercent(-3)).toBe("0.0%")
  })

  it("classifies hit rate levels using thresholds", () => {
    expect(checkpointHitRateLevel(39.9, thresholds)).toBe("critical")
    expect(checkpointHitRateLevel(40, thresholds)).toBe("warn")
    expect(checkpointHitRateLevel(69.9, thresholds)).toBe("warn")
    expect(checkpointHitRateLevel(70, thresholds)).toBe("normal")
  })

  it("classifies persist failure levels using thresholds", () => {
    expect(checkpointPersistFailureRateLevel(14.9, thresholds)).toBe("warn")
    expect(checkpointPersistFailureRateLevel(15, thresholds)).toBe("critical")
    expect(checkpointPersistFailureRateLevel(5, thresholds)).toBe("warn")
    expect(checkpointPersistFailureRateLevel(4.9, thresholds)).toBe("normal")
  })

  it("prioritizes hit-rate critical over persist critical in operator hint", () => {
    const hint = checkpointOperatorHint(35, 20, thresholds)
    expect(hint.level).toBe("critical")
    expect(hint.hitLevel).toBe("critical")
    expect(hint.persistLevel).toBe("critical")
    expect(hint.message).toContain("hit rate")
  })

  it("returns normal hint when both signals are healthy", () => {
    const hint = checkpointOperatorHint(88, 1, thresholds)
    expect(hint.level).toBe("normal")
    expect(hint.hitLevel).toBe("normal")
    expect(hint.persistLevel).toBe("normal")
    expect(hint.message).toContain("Normal")
  })

  it("renders severity badge and hint classes consistent with helper levels", () => {
    function SeverityPreview(props: { hitRate: number; persistFailureRate: number }) {
      const hint = checkpointOperatorHint(props.hitRate, props.persistFailureRate, thresholds)
      return React.createElement(
        "div",
        null,
        React.createElement("span", { className: `reliability-rate-badge reliability-rate-level-${hint.hitLevel}` }, "hit"),
        React.createElement("span", { className: `reliability-rate-badge reliability-rate-level-${hint.persistLevel}` }, "persist"),
        React.createElement("p", { className: `reliability-operator-hint reliability-operator-hint-${hint.level}` }, hint.message),
      )
    }

    const warnMarkup = renderToStaticMarkup(React.createElement(SeverityPreview, { hitRate: 55, persistFailureRate: 1 }))
    expect(warnMarkup).toContain("reliability-rate-level-warn")
    expect(warnMarkup).toContain("reliability-rate-level-normal")
    expect(warnMarkup).toContain("reliability-operator-hint-warn")

    const criticalMarkup = renderToStaticMarkup(React.createElement(SeverityPreview, { hitRate: 92, persistFailureRate: 20 }))
    expect(criticalMarkup).toContain("reliability-rate-level-normal")
    expect(criticalMarkup).toContain("reliability-rate-level-critical")
    expect(criticalMarkup).toContain("reliability-operator-hint-critical")
  })
})
