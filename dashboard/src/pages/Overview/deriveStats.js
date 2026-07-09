// Pure mapping from raw API responses to the numbers/flags the Overview page renders.
// Kept separate from the component so the field mapping (e.g. last_7_days) can be tested
// without rendering React.
export function deriveOverviewStats(overviewStats, mismatchStats, deliveryStats) {
  const activeHolds    = overviewStats?.active_holds?.value ?? 0
  const pendingDel      = overviewStats?.pending_deliveries?.value ?? deliveryStats?.pending ?? 0
  const exhaustedDel    = overviewStats?.exhausted_deliveries?.value ?? deliveryStats?.exhausted ?? 0
  const rejectedHooks   = overviewStats?.rejected_webhooks?.value ?? 0
  const mismatchCount   = mismatchStats?.last_7_days ?? 0
  const deliveredCount  = deliveryStats?.delivered_today ?? 0
  const totalDelivered  = deliveryStats?.delivered_today ?? 0
  const totalAttempted  = (deliveryStats?.delivered_today ?? 0) + (deliveryStats?.exhausted ?? 0)
  const deliveryRate    = totalAttempted > 0 ? Math.round((totalDelivered / totalAttempted) * 100) : 100
  const hasDeliveryIssues = exhaustedDel > 0 || rejectedHooks > 5

  return {
    activeHolds,
    pendingDel,
    exhaustedDel,
    rejectedHooks,
    mismatchCount,
    deliveredCount,
    deliveryRate,
    hasDeliveryIssues,
  }
}
