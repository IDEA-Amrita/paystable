# Frontend UX Guide

Paystable can take longer than a normal gateway redirect because it is intentionally checking whether a payment state is safe to trust. That does not mean the customer should be forced to wait on a spinner until the backend reaches a terminal state.

Use Paystable as the backend source of truth and design the frontend around progressive feedback.

## Recommended Flow

1. Merchant backend creates a hold before redirecting the user to the gateway.
2. Gateway redirects the user back to the merchant result page.
3. Result page subscribes to Paystable SSE or polls status with the `read_token`.
4. For the first 8-15 seconds, show active verification.
5. If still `VERIFYING`, let the user leave and continue resolution in the background.
6. Fulfillment happens from Paystable's signed backend callback.

## Status Copy

| Paystable status | Customer-facing copy | Merchant behavior |
|---|---|---|
| `PENDING` | "Processing your payment..." | Keep inventory reserved. |
| `VERIFYING` | "Payment received. Verifying with the bank..." | Do not fulfill or release. |
| long `VERIFYING` | "You can close this page. We will update your order automatically." | Continue waiting for callback. |
| `CONFIRMED` | "Payment confirmed." | Fulfill from signed callback. |
| `FAILED` | "Payment did not go through." | Offer retry. |
| `MISMATCH` | "Payment needs review." | Stop automation and show support reference. |
| `INDETERMINATE` | "We are checking this manually." | Escalate to ops/support. |

## Product Rules

- Disable "Pay again" while status is `PENDING` or `VERIFYING`.
- Do not show a red failure screen for `VERIFYING`.
- Do not fulfill from frontend polling alone.
- Do not call a gateway redirect parameter "success" until Paystable confirms.
- Always show an order/reference ID for `MISMATCH` or `INDETERMINATE`.
- Send email/SMS or update the user's account page after final callback.

## React Sketch

```jsx
const terminal = new Set(["CONFIRMED", "FAILED", "MISMATCH", "INDETERMINATE"]);

function PaymentResult({ txnId, readToken }) {
  const [status, setStatus] = useState("PENDING");
  const [slow, setSlow] = useState(false);

  useEffect(() => {
    const timer = setTimeout(() => setSlow(true), 12000);
    const stream = new EventSource(
      `/api/v1/transactions/${txnId}/stream?token=${readToken}`
    );

    stream.addEventListener("status_change", (event) => {
      const next = JSON.parse(event.data).status;
      setStatus(next);
      if (terminal.has(next)) {
        stream.close();
        clearTimeout(timer);
      }
    });

    return () => {
      stream.close();
      clearTimeout(timer);
    };
  }, [txnId, readToken]);

  if (status === "CONFIRMED") return <Success />;
  if (status === "FAILED") return <RetryPayment />;
  if (status === "MISMATCH" || status === "INDETERMINATE") return <SupportReview />;

  return slow ? <SafeToLeave /> : <Verifying />;
}
```
