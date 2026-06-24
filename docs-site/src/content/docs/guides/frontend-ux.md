---
title: Frontend UX
description: What to show customers while Paystable verifies a payment asynchronously.
---

Payment verification can take longer than a normal redirect page. That is not a reason to trap the customer in a spinner.

Paystable's job is to produce a safe backend state. Your frontend's job is to keep the customer informed without promising too much too early.

## Recommended Pattern

1. Show a normal verifying state for the first few seconds.
2. Keep "Pay again" disabled while the hold is `PENDING` or `VERIFYING`.
3. If verification is still running after roughly 8-15 seconds, let the customer leave.
4. Continue resolving the order through Paystable's backend callback.
5. Update the account page, email, SMS, or ticket page when the callback arrives.

## Status Copy

| Paystable status | Screen copy | Backend behavior |
|---|---|---|
| `PENDING` | "Processing your payment..." | Keep the order reserved. |
| `VERIFYING` | "Payment received. Verifying with the bank..." | Do not fulfill or release yet. |
| long `VERIFYING` | "You can close this page. We will update your order automatically." | Continue waiting for callback. |
| `CONFIRMED` | "Payment confirmed." | Fulfill from the signed callback. |
| `FAILED` | "Payment did not go through." | Offer retry. |
| `MISMATCH` | "Payment needs review." | Stop automation and show support reference. |
| `INDETERMINATE` | "We are checking this manually." | Escalate to support/ops. |

## Example React Flow

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

## Product Rules

- Do not call the gateway redirect result "success" until Paystable confirms.
- Do not show a red failure screen for `VERIFYING`.
- Do not let the user start another payment for the same order while Paystable is verifying.
- Do not fulfill from frontend polling alone.
- Do provide a support reference for review states.

For digital goods and wallet credits, wait for `CONFIRMED`. For physical goods and tickets, reserve inventory while verifying and release only after `FAILED`.
