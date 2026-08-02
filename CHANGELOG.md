# changelog

## unreleased

- add `paystable init` to generate local secrets into `.env` (no placeholders secrets).
- installer runs `./paystable init` instead of copying `.env.example`.
- improve `paystable doctor` with Environment/Database/Migrations sections and exact next commands.
- explain local Postgres ident/peer auth failures in `paystable doctor`.
- add `paystable doctor` for env, Postgres, and migration checks.
- document local Postgres setup for binary installs.
- harden webhook persistence, deduplication, and early-event handling.
- align terminal-state handling with stable gateway observations.
- add checksum verification to the installer and release artifacts.
- add open-source contribution and security policy files.
