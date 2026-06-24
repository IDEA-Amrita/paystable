# security policy

paystable handles payment state, webhook signatures, and merchant callbacks. please report suspected vulnerabilities privately.

## reporting

email the maintainers or open a private GitHub security advisory for this repository. include:

- affected version or commit
- exact endpoint or component
- reproduction steps
- impact and any logs that do not contain live secrets

## expectations

- do not include real merchant secrets, callback secrets, customer data, or gateway payloads in public issues.
- do not test against systems you do not own.
- we prioritize signature bypasses, callback forgery, state-machine safety bugs, and release/install integrity issues.
