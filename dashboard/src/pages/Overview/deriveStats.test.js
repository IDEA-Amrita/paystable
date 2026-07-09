import { describe, it, expect } from 'vitest'
import { deriveOverviewStats } from './deriveStats'

describe('deriveOverviewStats', () => {
  it('reads mismatch count from last_7_days, matching the admin API field name', () => {
    const { mismatchCount } = deriveOverviewStats({}, { last_7_days: 12 }, {})
    expect(mismatchCount).toBe(12)
  })

  it('does not read the old total_7d field name', () => {
    const { mismatchCount } = deriveOverviewStats({}, { total_7d: 12, last_7_days: 0 }, {})
    expect(mismatchCount).toBe(0)
  })

  it('defaults mismatch count to 0 when mismatchStats is missing', () => {
    const { mismatchCount } = deriveOverviewStats({}, null, {})
    expect(mismatchCount).toBe(0)
  })

  it('flags delivery issues when there are exhausted deliveries', () => {
    const { hasDeliveryIssues } = deriveOverviewStats(
      { exhausted_deliveries: { value: 1 } },
      {},
      {}
    )
    expect(hasDeliveryIssues).toBe(true)
  })

  it('flags delivery issues when webhook rejections exceed the threshold', () => {
    const { hasDeliveryIssues } = deriveOverviewStats(
      { rejected_webhooks: { value: 6 } },
      {},
      {}
    )
    expect(hasDeliveryIssues).toBe(true)
  })

  it('reports no delivery issues when both signals are within bounds', () => {
    const { hasDeliveryIssues } = deriveOverviewStats(
      { exhausted_deliveries: { value: 0 }, rejected_webhooks: { value: 2 } },
      {},
      {}
    )
    expect(hasDeliveryIssues).toBe(false)
  })

  it('computes delivery rate from delivered vs attempted counts', () => {
    const { deliveryRate } = deriveOverviewStats({}, {}, { delivered_today: 90, exhausted: 10 })
    expect(deliveryRate).toBe(90)
  })
})
