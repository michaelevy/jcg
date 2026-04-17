# Middleware Package

Last verified: 2026-04-17

## Purpose
Session management and route protection. In-memory session store backs cookie-based auth.

## Contracts
- **Exposes**: `CreateSession(w, username)`, `DeleteSession(w, r)`, `RequireAuth(next) http.Handler`, `UsernameFromContext(r) string`, `InjectUsername(ctx, username)` (test only), `StoreTestSession`/`ResetStore` (test only)
- **Guarantees**: Sessions expire after 24h. RequireAuth redirects to /login on missing/expired session. Session IDs are 32 bytes from crypto/rand, base64url-encoded. Cookie is HttpOnly + SameSite=Lax.
- **Expects**: Nothing external; self-contained with sync.Map store.

## Dependencies
- **Uses**: Nothing from internal/
- **Used by**: handlers (session create/delete, context username), cmd/server (RequireAuth wrapping)

## Key Decisions
- sync.Map over database sessions: acceptable for single-instance deployment, avoids extra DB round-trips
- Exported test helpers (StoreTestSession, ResetStore, InjectUsername): pragmatic choice to keep handler tests isolated without real login flows

## Invariants
- Cookie name is always `jcg_session`
- CtxKeyUsername is the only context key; typed as `contextKey` string to avoid collisions
- Expired sessions are deleted on access (lazy cleanup), not swept in background (known TODO)
