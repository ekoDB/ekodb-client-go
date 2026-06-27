# Changelog

All notable changes to ekodb-client-go will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.23.0] - 2026-06-27

### Added

- **Batch insert/update/delete now forward `transaction_id` to support
  transactional batch writes.** `BatchInsertOptions` / `BatchUpdateOptions` /
  `BatchDeleteOptions` already declared a `TransactionId` field, but the three
  batch methods never sent it — so batch writes staged into an MVCC transaction
  silently executed outside it. The field is now appended as the
  `transaction_id` query parameter to `/api/batch/{insert,update,delete}/…` (the
  server batch handlers already honor it), mirroring the single-record ops.
  Additive/opt-in — callers that don't set `TransactionId` are unaffected.
  Covered by `TestBatchOpsWithTransactionIDQueryParam`. Brings batch-transaction
  parity with the Rust/Python/TypeScript/Kotlin clients.

### Security

- **`ListFunctions` / `ListUserFunctions` now percent-encode the `tags` query
  value.** The tag list was concatenated raw into `?tags=…`, so a tag containing
  query-reserved characters (`&`, `=`) could split into extra query parameters
  (e.g. `a&injected=1` smuggling an `injected=1` param) and produce a malformed
  URL. Both methods now wrap the joined value in `url.QueryEscape`. Defense in
  depth and cross-client consistency with the matching `tags` fix in the other
  `ekodb-client` SDKs. Covered by
  `TestListUserFunctionsEncodesReservedCharsInTags` and
  `TestListFunctionsEncodesReservedCharsInTags`, which assert the value
  round-trips intact and no smuggled param leaks.

## [0.22.0] - 2026-06-23

### Added

- **`SubmitChatToolKeepalive` for in-flight client tools (resets the server-side
  per-tool wait deadline; pairs with ekoDB#530).**
- **Metadata pre-filter for text, vector, and hybrid search (#475).**
  `SearchQuery` gained a `Filters` field and `SearchQueryBuilder` a
  `Filters(filter interface{})` method carrying a canonical `QueryExpression`
  (the same shape produced under `QueryBuilder.Build()`'s `"filter"` key). Only
  records matching the filter are considered as candidates before ranking;
  serialized as `filters` and omitted when unset. The `examples/` search example
  now demonstrates a filtered search. Tests: `TestSearchQueryBuilderFilters`,
  `TestSearchQueryBuilderFiltersOmittedWhenUnset`.

### Tests

- **Regression test for cancelable WebSocket dial (#42, acceptance a).** Added
  `TestWebSocketCloseAbortsStuckDial`, asserting that a dial blocked in the TCP
  connect phase (a non-routable RFC 5737 address) is aborted promptly by
  `Close()` via context cancellation, rather than blocking for the OS connect
  timeout. The test documents the scope boundary: a dial hung in the WebSocket
  upgrade handshake remains bounded by `DefaultDialer.HandshakeTimeout` (45s),
  which gorilla applies as a static read deadline (no `ctx.Done()` watcher in
  that phase) — unchanged before and after the dial fix.

## [0.21.0] - 2026-06-09

### Added

- **Buffered-transaction read-your-writes + savepoints.** ekoDB transactions are
  now enforced and buffered server-side: writes carrying a `transaction_id` are
  staged and applied atomically at commit. `Find`/`FindByID` now accept a
  transaction id so reads see the transaction's own staged writes
  (read-your-writes):
  `FindByID(collection, id, FindByIDOptions{ TransactionId, SelectFields, ExcludeFields })`
  and `Find(collection, query, FindOptions{ TransactionId })`. Added savepoint
  methods `CreateSavepoint`, `RollbackToSavepoint`, and `ReleaseSavepoint`.
  `CommitTransaction` may return an HTTP 409 conflict (retry the transaction)
  when a record it read or wrote was changed by another committed transaction;
  documented on the method.

- **`Client.KVClear()`** — clears the entire KV store via
  `DELETE /api/kv/clear`, filling a client-parity gap (the endpoint was
  previously unreachable from the Go client).

- **`Client.ListUserCollections()`** — lists only user-created collections
  (passing the server's `exclude_internal=true` filter), excluding internal
  chat/system collections. Closes a parity gap: the other clients
  (Rust/TypeScript/Python/Kotlin) already exposed `list_user_collections`.

- **`Client.RefreshToken()`** — eagerly fetches a fresh auth token (bypassing
  the cached one) and returns it. Closes a parity gap: the other clients expose
  a public `refresh_token`, while Go previously only had `ClearTokenCache()`
  (which defers the fetch to the next request).

- **WebSocket msgpack negotiation (transparent binary transport).** On every WS
  (re)connect the client now performs an additive `Hello`/`Welcome` handshake:
  it offers msgpack and, if the server welcomes it, transparently switches that
  connection to binary msgpack frames for both requests and responses; otherwise
  it stays on JSON text. The negotiation is internal, so there are no public API
  changes. Fully back-compatible — a server that does not welcome msgpack (or an
  older server that never answers) leaves the connection on JSON. Incoming
  binary frames are decoded value-identically to JSON (including binary fields,
  which stay number arrays, not base64), so the decoded data is the same
  regardless of negotiated transport.

### Fixed

- **Percent-encode all URL path segments.** `client.go` already escaped its
  CRUD/KV paths, but the chat, functions, goals/tasks/agents, schedules, schema,
  and search endpoints interpolated caller-supplied segments (ids, function
  labels, chat model names like `anthropic/claude-3`, agent names, …) into the
  path without escaping — so a reserved character (`/`, space, `#`, `?`)
  produced a malformed URL the server 404'd. All 60 caller-segment sites now use
  `url.PathEscape`. Behavior is unchanged for segments without reserved
  characters. Covered by new encoding tests (`path_escape_test.go`). Part of the
  cross-client parity fix (ekodb-client #153).

- **`connect()` initializes `ctx`/`cancel` whenever EITHER is nil.** A manually
  constructed `WebSocketClient` that set only one of the pair would panic:
  `cancel` nil → `Close()`/`reconnect()` (which call `ws.cancel()`
  unconditionally); `ctx` nil → `sendRequest()`/subscribe loops (which deref
  `ws.ctx` via `context.WithTimeout` / `ws.ctx.Done()`). `connect()` now derives
  a cancelable context whenever either field is unset, covering all manual-
  construction combinations. Regression tests
  `TestWebSocketConnectInitsCancelWhenCtxSetWithoutCancel` and
  `TestWebSocketConnectInitsCtxWhenCancelSetWithoutCtx`.

- **WebSocket response routing no longer risks misrouting an unmatched ack.**
  The single-pending-request fallback in `routeRequestResponse` keyed off "no
  top-level message-id field" — but a messageId extracted from the payload did
  not set that flag, so a present-but-unmatched id (e.g. a best-effort
  `Unsubscribe` ack, or a late response for an already-settled request) could be
  delivered to whatever single request was still pending. It now suppresses the
  fallback whenever a message-id field is present anywhere (top level or
  payload) with a usable value — including a present-but-unparseable id —
  matching the TypeScript client's truthiness check. Only a response with no
  usable id at all falls through to the sequential-request heuristic.
- **WebSocket dial is now cancelable.** `connect()` uses
  `DialContext(ws.ctx, …)` instead of `DefaultDialer.Dial`, so `Close()`/context
  cancellation can abort an in-flight dial (a reconnect-loop dial no longer
  blocks a clean shutdown until the handshake timeout).
- **Reconnect no longer revives a zombie connection.** If every subscription is
  removed while the reconnect loop is backing off (e.g. an in-flight `Subscribe`
  failed and deleted its subscription after the drop spawned the loop),
  `reconnect()` now exits before dialing instead of reconnecting with nothing to
  replay.
- **`Unsubscribe` now tells the server to stop streaming.** It sends a
  best-effort WebSocket `Unsubscribe` frame (the server already handles it) in
  addition to the local teardown, so the server stops pushing mutations for the
  collection instead of streaming until the connection drops.

### Documentation

- Clarified the `QueryBuilder.Page` doc comment: values below 1 clamp to page 1,
  unlike `Client.Paginate`, which returns an error for `page < 1` (only the
  offset calculation matches).

## [0.20.0] - 2026-06-04

### Removed

- **Removed the query-builder `regex()` filter** from all clients — the server
  has no regex filter operator, so it 400'd (or, in Rust, silently fell back to
  substring `Contains`). Removed until server-side regex filtering is available
  (tracked internally). Breaking: callers using `regex()` should switch to
  `contains` / `startsWith` / `endsWith`.

### Changed

- **BREAKING: `QueryBuilder.Page` is now 1-indexed** (#38). `Page(1, n)` is the
  first page (skip 0), matching `Client.Paginate`; previously `Page` was
  0-indexed (`page * pageSize`), so the two pagination APIs disagreed. Page
  numbers below 1 clamp to the first page. Callers using the old 0-indexed
  `Page` must add 1 to their page numbers.
- **BREAKING: `ChatMessageStream` now takes a `context.Context`** (#34). The
  signature is now `ChatMessageStream(ctx, sessionID, request)`, matching the
  ctx-first convention already used by `SubscribeSSE`. The streaming goroutine
  select-sends on `ctx.Done()`, so cancelling the context releases the goroutine
  and the underlying connection even when the consumer stops draining the
  channel — previously it could block forever on a full buffer and leak.
  Regression test `TestChatMessageStreamCancellation`. Callers must pass a
  context (e.g. `context.Background()`).

### Fixed

- **WebSocket `connect()` no longer leaks a socket on a close/dial race** (#41).
  `Dial` takes no context, so a `Close()` that ran while a dial was in flight
  could tear down the previous connection and then have the completed dial
  overwrite `ws.conn` with a freshly-opened socket that was never closed.
  `connect()` now re-checks `closing` (atomically against `Close()`, which sets
  it under `ws.mu` before nil'ing `ws.conn`) after dialing: if the client is
  closing it closes the new connection and returns an error instead of storing
  it. Regression test added.
- **WebSocket subscriptions now auto-reconnect** (#37). On an unexpected
  disconnect the client no longer closes every subscription channel and gives
  up. It now reconnects with capped exponential backoff + jitter (200ms → ~5s),
  re-sends the `Subscribe` request for every tracked subscription, and resumes
  delivery on the SAME caller-held channels. Each (re)connect reads a FRESH
  token from the parent client, so a since-expired JWT is refreshed
  transparently instead of permanently locking the connection out. In-flight
  request and chat-stream callers are failed with an error on the drop (instead
  of hanging), and an intentional `Close()` cleanly stops the reconnect loop.
  Previously a transient drop permanently closed all subscriptions and a stale
  token could never recover. Covered by
  `TestWebSocketReconnectResumesSubscription`,
  `TestWebSocketReconnectDialsWithToken`, and
  `TestWebSocketCloseStopsReconnection`.
- **Network-error retries use exponential backoff with full jitter** (#36).
  Replaced the fixed 3s delay with a capped exponential schedule (200ms → 5s)
  plus full jitter, so concurrent clients don't retry in lockstep. Covered by
  `TestRetryBackoffBase` / `TestRetryBackoffJitterBounds`. (Making the retry
  sleep itself context-cancellable requires threading a `context.Context`
  through the request path and is tracked as follow-up work.)
- **Data race on `rateLimitInfo`** (#33). `extractRateLimitInfo` wrote the field
  on every response while `GetRateLimitInfo` / `IsNearRateLimit` read it without
  synchronization. Added a dedicated `sync.RWMutex` guarding all access; the
  write now publishes a fully-built value under the lock. Regression test
  `TestExtractRateLimitInfoNoDataRace` runs clean under `-race`.
- **`GetValue` no longer corrupts user objects** (#35). It previously unwrapped
  any object containing a `value` key, so a legitimate object like
  `{"value": 1, "currency": "USD"}` lost its sibling fields on read. It now only
  unwraps a genuine typed wrapper (both `type` and `value` present) and passes
  every other object through untouched. Covered by
  `TestGetValuePassesThroughUserObjectWithValueKey`.
- **Corrected the serialization-default docstring** (#38). The
  `NewClientWithConfig` comment claimed "Default is JSON" although the zero
  value selects MessagePack.

## [0.19.0] - 2026-06-02

### Added

- **`CompactChat(chatID string, keepRecent *int)` for on-demand chat history
  compaction** (ekodb #43). Calls the new `POST /api/chat/{id}/compact`
  endpoint, which folds older messages into a summary and marks the originals
  forgotten to reclaim context-window budget. Returns a `CompactChatResponse`
  (`Folded`, `KeptRecent`, `SummaryChars`, `SummaryMessageID *string`,
  `AlreadyCompact`). `keepRecent` is a pointer so it's omitted when nil (server
  defaults to the session's `max_context_messages`). Mirrors the existing chat
  methods; covered by `TestCompactChat` / `TestCompactChatAlreadyCompact`.

### Changed

- **Bumped Go toolchain pin from 1.24.0 to 1.25.0** in `go.mod` and dropped the
  separate `toolchain go1.24.2` directive. Aligns with wavescd and currentcs,
  which both already require Go 1.25. No code changes — 355 unit tests still
  pass.

## [0.18.1] - 2026-04-29

### Added — Server-side chat cancel (WebSocket)

- **`WebSocketClient.CancelChat(chatID string) error`** — new method that sends
  a `CancelChat` WS message with
  `{ "type": "CancelChat", "payload": { "chat_id": "<id>" } }`. The ekoDB server
  fires the matching `CancellationToken`, drops the in-flight LLM HTTP call, and
  skips persisting the assistant message. Brings the Go client to parity with
  the Rust client's `WebSocketClient::cancel_chat` (ekodb_client 0.18.1).
- **No-op semantics**: cancelling a `chat_id` with no in-flight stream is safe
  server-side and does not error — callers can fire it speculatively.
- **Wire-format test** — `TestWebSocketCancelChat` pins the `type: "CancelChat"`
  tag and `payload.chat_id` field name (both load-bearing on the server) and
  asserts the payload carries exactly `chat_id` and no extra fields. Catches
  accidental rename / shape drift across version boundaries.

## [0.18.0] - 2026-04-18

### Added (2026-04-26 — crypto + concurrency stage builders)

- **11 new crypto stage builders** matching ekoDB 0.42.0's new stored-function
  stages:
  - `StageHmacSign(input, secret, outputField, algorithm, encoding)` and
    `StageHmacVerify(...)` (HMAC-SHA256/384/512). Pass `""` for `algorithm` /
    `encoding` to use the server defaults.
  - `StageAesEncrypt(plaintext, key, outputField, keyEncoding)` and
    `StageAesDecrypt(...)` (AES-256-GCM, fail-closed decrypt).
  - `StageUuidGenerate(outputField)` (RFC 4122 v4).
  - `StageTotpGenerate(secret, outputField, *opts)` and
    `StageTotpVerify(code, secret, outputField, *opts)` (RFC 6238). New
    `TotpOptions` struct carries the optional digits / period / algorithm / skew
    fields.
  - `StageBase64Encode(input, outputField, *urlSafe)` and
    `StageBase64Decode(...)`.
  - `StageHexEncode(input, outputField)` and `StageHexDecode(...)`.
  - `StageSlugify(input, outputField)`.
- **4 new concurrency stage builders** wrapping ekoDB's atomic KV primitives:
  - `StageIdempotencyClaim(key, ttlSecs, outputField)`.
  - `StageRateLimit(key, limit, windowSecs, outputField, onExceed)` — pass `""`
    for `onExceed` to use the default `"fail"` mode.
  - `StageLockAcquire(key, ttlSecs, outputField)` and
    `StageLockRelease(key, token, outputField)`.
- **Tests** — 7 new test functions covering shape + optional-field omission for
  HMAC, AES+UUID, TOTP, Base64+Hex+Slugify, the four concurrency stages, and
  JSON round-trip for all 11 new types.

**Requires ekoDB >= 0.42.0** for these stages to execute on the server.

### Added (2026-04-26 — JWT + EmailSend + path-routed function fields)

- **`StageJwtSign(claims, secret, outputField, *expiresInSecs, algorithm)` and
  `StageJwtVerify(tokenField, secret, outputField, algorithm)`** — Go bindings
  for ekoDB's HMAC JWT stages (HS256 / HS384 / HS512). Claims marshal to JSON;
  pass `""` for `algorithm` to default to HS256.
- **`StageEmailSend(to, subject, body, from, apiKey, *opts)` +
  `EmailSendOptions` struct** — Go binding for the SendGrid v3 `mail/send`
  integration stage. Optional `ReplyTo`, `Provider`, `HTML`, and `OutputField`
  fields on `EmailSendOptions` for the full surface.
- **`HTTPMethod` + `HTTPPath` on `UserFunction`** (both `*string`, `omitempty`).
  Lets a stored function answer to `GET /api/route/users/:id` via the ekoDB
  server's path-routed dispatcher.
- **Tests** — 4 JWT builder-shape / JSON round-trip cases and 3 EmailSend shape
  cases. Server-side reject paths (wrong secret, expired token, unsupported
  algorithm) are covered by server-side integration tests. Two new
  `TestUserFunction_jsonIncludesHTTPFieldsWhenSet` /
  `_jsonOmitsHTTPFieldsWhenNil` tests guard the `http_method` / `http_path` JSON
  tags + omitempty behavior for the path-routed dispatcher.

## [0.17.0] - 2026-04-12

### Added

- **Five new function stage builders: `StageTryCatch`, `StageParallel`,
  `StageSleep`, `StageReturn`, `StageValidate`** — Feature-parity bindings for
  ekoDB's error-handling, control-flow, response-formatting, and validation
  stages. Previously only the server supported these; any stored function using
  them would fail to round-trip through the Go client.
  - `StageTryCatch(tryFunctions, catchFunctions, outputErrorField)` — try/catch
    for graceful failure recovery.
  - `StageParallel(functions, waitForAll)` — concurrent execution of multiple
    stages.
  - `StageSleep(durationMs)` — delay execution (accepts int or placeholder
    string).
  - `StageReturn(fields, statusCode)` — shape the final response object.
  - `StageValidate(schema, dataField, onError)` — JSON schema validation before
    processing.

- **Three new crypto-primitive stage builders: `StageBcryptHash`,
  `StageBcryptVerify`, and `StageRandomToken`** — Feature-parity bindings for
  ekoDB 0.41.0's new stored-function crypto stages. Lets Go apps build a pure
  stored-function auth flow (`users_register = Validate + BcryptHash + Insert`,
  `users_login = FindOne + BcryptVerify + If`) without hosting a bcrypt or
  CSPRNG dependency at the app layer.
  - `StageBcryptHash(plain, outputField string, cost *int)` — hashes a plaintext
    (typically `"{{password}}"`) with bcrypt; `cost` defaults to 12 when nil.
  - `StageBcryptVerify(plain, hashField, outputField string)` — reads the stored
    hash from `working_data[0][hashField]`, verifies, and writes a boolean into
    `outputField`. Pair with `StageIf` for login.
  - `StageRandomToken(bytes int, encoding, outputField string)` — draws `bytes`
    bytes (1..=1024) from the OS CSPRNG and encodes as `"hex"` (default),
    `"base64"`, or `"base64url"`. Pass `""` for the server default.

  Sensitive call-time inputs (the password the user just typed) flow through the
  normal text-level `"{{password}}"` placeholder. Operator secrets (peppers,
  data keys) continue to use `"{{env.NAME}}"` sourced from the ekoDB config's
  `environment_vars` whitelist.

  Requires ekoDB >= 0.41.0. 6 new tests in `functions_test.go`
  (`TestStageBcryptHash_withExplicitCost`,
  `TestStageBcryptHash_omitsCostWhenNil`,
  `TestStageBcryptVerify_wiresAllFields`,
  `TestStageRandomToken_withExplicitEncoding`,
  `TestStageRandomToken_omitsEncodingWhenEmpty`,
  `TestCryptoStages_jsonRoundTrip`) — full Go suite: 327/327 passing.

- **`Parameter(name)` helper for structural placeholder construction** — New
  top-level function in `functions.go` that returns the
  `map[string]interface{}{"type": "Parameter", "name": name}` shape ekoDB's
  `resolve_json_parameters` recognizes inside `StageInsert`'s `record`,
  `StageUpdate` / `StageUpdateById` / `StageFindOneAndUpdate` `updates`,
  `StageBatchInsert`'s per-record entries, and any `QueryExpression` filter
  value. This is the structural alternative to text-level `"{{name}}"`
  placeholders — use it when the parameter is a whole-object record or a value
  whose type would otherwise be lost on a raw-JSON round-trip (Binary, DateTime,
  UUID, Decimal, Duration, Number, Set, Vector). Requires ekoDB >= 0.41.0 for
  the mutation-stage parameter-resolution consistency fix. New tests in
  `functions_test.go` cover Insert, UpdateById, Update (filter-based), and
  BatchInsert including JSON round-trip verification.

## [0.16.0] - 2026-04-01

### Added

- **`agent_id` field on chat sessions** — `CreateChatSessionRequest` and
  `ChatSession` structs now include an optional `AgentID` field to associate a
  session with a named agent.

- **Client tool fields on `ChatMessageRequest`** — Added `ClientTools`,
  `ConfirmTools`, and `ExcludeTools` optional fields plus `ClientToolDef` struct
  for HTTP/SSE chat message requests. Mirrors the WebSocket `ChatSendOptions`
  fields.

- **`SubmitChatToolResult()`** — New HTTP method to submit a client tool result
  for an in-flight SSE chat stream, unblocking ekoDB's tool loop.

- **`SubscribeSSE()`** — Subscribe to collection mutations via SSE (Server-Sent
  Events). Accepts a `context.Context` for cancellation and returns an
  `*SSESubscription` with `Events` and `Err` channels. Use when WebSocket
  connections aren't available. Supports `FilterField`/`FilterValue` options.

## [0.15.2] - 2026-03-28

### Fixed

- **Search score injection** — `HybridSearch()` and `TextSearch()` now inject
  `_score` into returned records. Previously scores were stripped when mapping
  search results to records, causing all scores to read as 0.

## [0.15.1] - 2026-03-25

### Added

- **Full WebSocket CRUD parity** — 14 new methods on `WebSocketClient`:
  `Insert`, `Query`, `FindByID`, `Update`, `Delete`, `BatchInsert`,
  `BatchUpdate`, `BatchDelete`, `TextSearch`, `DistinctValues`,
  `UpdateWithAction`, `CreateCollection`, `ListCollections`, `DeleteCollection`.
  All use `messageId` for concurrent request correlation via `sendCRUD` helper.

- **Schema cache** (`schema_cache.go`) — `SchemaCache` struct with LRU eviction,
  TTL expiry, and realtime invalidation via WS `SchemaChanged` events. Enable
  with `client.EnableSchemaCache(5*time.Minute, 100)`.

- **`SchemaChanged` event handling** — WS dispatcher routes `SchemaChanged`
  events to the schema cache for automatic invalidation when schema/primary_key
  changes.

- **`ConnectWS()`** — Convenience method on `Client` that derives the WS URL
  from the base URL (http→ws, https→wss) and attaches the schema cache
  automatically.

- **`ExtractRecordID(collection, record)`** — On both `Client` and
  `SchemaCache`. Uses cached `primary_key_alias` first, then falls back to
  `"id"` / `"_id"`. Handles typed wrapper format.

- **`QueryOptions`** — Struct for WS `Query` method with `Filter`, `Sort`,
  `Limit`, `Skip` fields.

## [0.15.0] - 2026-03-25

### Added

- **Server-side `ExecuteTool`** — `Client.ExecuteTool(toolName, params, chatID)`
  delegates to ekoDB's `POST /api/chat/tools/execute` endpoint. All collection
  filtering, permission enforcement, and internal collection blocking happen
  server-side. Accepts optional `chatID` for memory-tool context. Returns
  `nil, nil` on 404/405 so callers can fall back to chat/LLM routing. New
  `ExecuteToolRequest`/`ExecuteToolResult` types.

### Fixed

- **`ExecuteTool` 404/405 fallback used fragile string matching** — Was using
  `strings.Contains(err.Error(), "404")` which could false-positive on unrelated
  error bodies. Now uses `errors.As` to type-assert `*HTTPError` and checks
  `StatusCode` directly.

## [0.14.0] - 2026-03-23

### Added

- **JWT expiry-based token caching** — The client now extracts the `exp` claim
  from JWT tokens and proactively refreshes 60 seconds before expiry, matching
  the Rust client's `AuthManager` behavior. Previously tokens were cached
  indefinitely until a 401 triggered a reactive refresh. New
  `extractJWTExpiry()` function decodes the JWT payload (URL-safe base64 no-pad)
  without signature verification. Falls back to a 1-hour TTL if JWT decoding
  fails.

- **`ClearTokenCache()` exported method** — Clears the cached token and expiry,
  forcing a fresh token fetch on the next request.

- **Goal template CRUD methods** — New client methods for managing goal
  templates: `GoalTemplateCreate`, `GoalTemplateList`, `GoalTemplateGet`,
  `GoalTemplateUpdate`, `GoalTemplateDelete`. Calls `/api/chat/goal-templates`
  endpoints on the ekoDB server.

- **`ContextWindow` field on `ChatStreamEvent`** — The `end` event now includes
  `ContextWindow uint32` with the model's context window size in tokens. Allows
  clients to display context usage and warn when approaching limits.

- **SSE chat message streaming** — New `ChatMessageStream()` method on `Client`
  streams chat responses via Server-Sent Events over plain HTTP. Returns a
  `chan ChatStreamEvent` (same type as WebSocket `ChatSend`). Calls
  `POST /api/chat/{id}/messages/stream`. Simpler alternative to WebSocket
  streaming that works behind reverse proxies without upgrade support.

### Removed

- **Query index management methods** — Removed `CreateQueryIndex`,
  `ListQueryIndexes`, `DeleteQueryIndex`, `ExplainQuery` (deleted `indexes.go`).
  These endpoints require admin auth (`admin_filter`) and do not belong in the
  client library.

- **Search index management methods** — Removed `CreateSearchIndex`,
  `ExplainTextSearch`, `ExplainVectorSearch`, `ExplainHybridSearch` (deleted
  `search_indexes.go`). These endpoints require admin auth (`admin_filter`) and
  do not belong in the client library.

### Fixed

- **HTTP client timeout no longer kills SSE streams** — Replaced
  `http.Client.Timeout` (which aborts entire requests including streaming
  response bodies) with a `net.Dialer.Timeout` on a custom `http.Transport`. The
  dial timeout still protects against unresponsive servers during connection
  setup, but SSE streams like `RawCompletionStream` can now run indefinitely
  without being killed.

### Added

- **Search index management methods** — `CreateSearchIndex`,
  `ExplainTextSearch`, `ExplainVectorSearch`, `ExplainHybridSearch` for creating
  search indexes and explaining search query execution plans.

- **KV document linking methods** — `KVGetLinks`, `KVLink`, `KVUnlink` for
  linking and unlinking documents to KV keys.

- **Schedule management methods** — `CreateSchedule`, `ListSchedules`,
  `GetSchedule`, `UpdateSchedule`, `DeleteSchedule`, `PauseSchedule`,
  `ResumeSchedule` for full CRUD and lifecycle management of scheduled tasks.

- **`RawCompletionStreamWithProgress` streaming callback** — New method that
  works like `RawCompletionStream` but accepts an `onToken func(string)`
  callback invoked for each token as it arrives. Allows callers to display
  real-time incremental output during long-running LLM calls.

- **Goal CRUD & lifecycle methods** — `GoalCreate`, `GoalList`, `GoalGet`,
  `GoalUpdate`, `GoalDelete`, `GoalSearch`, `GoalComplete`, `GoalApprove`,
  `GoalReject`, `GoalStepStart`, `GoalStepComplete`, `GoalStepFail`. Full
  coverage of the `/api/chat/goals` endpoints including step-level lifecycle
  transitions.

- **Task CRUD & lifecycle methods** — `TaskCreate`, `TaskList`, `TaskGet`,
  `TaskUpdate`, `TaskDelete`, `TaskDue`, `TaskStart`, `TaskSucceed`, `TaskFail`,
  `TaskPause`, `TaskResume`. Full coverage of the `/api/chat/tasks` endpoints
  including due-task polling and lifecycle transitions.

- **Agent CRUD methods** — `AgentCreate`, `AgentList`, `AgentGet`,
  `AgentGetByName`, `AgentUpdate`, `AgentDelete`, `AgentsByDeployment`. Full
  coverage of the `/api/chat/agents` endpoints including lookup by name and by
  deployment ID.

## [0.13.0] - 2026-03-18

### Added

- **SSE streaming raw completion** — New `RawCompletionStream()` method. Calls
  `POST /api/chat/complete/stream` and parses SSE events. Keeps the connection
  alive with heartbeat events, preventing reverse proxy timeouts on deployed
  instances.

### Fixed

- **Auth in RawCompletionStream** — Uses proper JWT token exchange via
  `getToken()`/`refreshToken()` instead of the raw API key.

- **`GetIntValue` now accepts `json.Number`, numeric strings, and all integer
  types** — Previously only handled `int`, `int64`, and `float64`. Now accepts
  `int8-64`, `uint8-64`, `float32`, `json.Number`, and numeric strings (e.g.,
  `"42"`). This fixes silent zero-value returns when ekoDB records contained
  values in these types.
- **`GetFloatValue` now accepts `json.Number`, numeric strings, and all numeric
  types** — Previously only handled `float64`, `int`, and `int64`. Now accepts
  `float32`, `int8-64`, `uint8-64`, `json.Number`, and numeric strings.
- **`GetBoolValue` now accepts string and numeric representations** — Previously
  only handled `bool`. Now accepts `"true"/"false"`, `"1"/"0"`,
  `"yes"/"no"/"y"/"n"/"on"/"off"`, and numeric values (non-zero = true). This
  fixes silent false returns for boolean fields stored as strings.

### Added

- **Atomic field actions** — New `UpdateWithAction()` and
  `UpdateWithActionSequence()` methods for safe concurrent field modifications:
  increment/decrement counters, push/pop/shift/unshift arrays,
  multiply/divide/modulo arithmetic, append strings, remove array items, and
  clear fields. Sequence variant applies multiple actions atomically in a single
  request. 5 new unit tests.

- **Full WebSocket dispatcher** — Rewrote `WebSocketClient` with a
  goroutine-based read loop that routes incoming messages by type. New methods:
  `Subscribe()` (returns `<-chan MutationNotification` for real-time collection
  change notifications with optional filter), `ChatSend()` (returns
  `<-chan ChatStreamEvent` with chunk/end/toolCall/error events for streaming
  chat responses), `RegisterClientTools()` (registers client-side tool
  definitions), and `SendToolResult()` (returns tool execution results to the
  server). New types: `MutationNotification`, `ChatStreamEvent`,
  `ClientToolDefinition`, `ChatSendOptions`, `SubscribeOptions`. Channel-based
  concurrency with `sync.Mutex` protection. 11 new unit tests with httptest
  WebSocket server.

- **`GetChatTools()` method** — Returns all built-in server-side ekoDB chat tool
  definitions via `GET /api/chat/tools`. Returns `[]map[string]interface{}` with
  `name`, `description`, and `parameters` per tool. Used by planning agents to
  dynamically discover available tools.

- **`RawCompletion()` method** — Stateless raw LLM completion via
  `POST /api/chat/complete`. Accepts a `RawCompletionRequest` with
  `SystemPrompt`, `Message`, and optional `Provider`, `Model`, `MaxTokens`
  fields. Returns a `*RawCompletionResponse` with a `Content` string. Use this
  for structured-output tasks that must be parsed programmatically without
  session or history overhead.

- **`RawCompletionRequest` and `RawCompletionResponse` types** — New types in
  `chat.go` for the raw completion API.

- **`DistinctValues()` method** — New method for retrieving all unique values
  for a specific field across records in a collection. Accepts a
  `DistinctValuesQuery` with optional filter, `BypassRipple`, and `BypassCache`
  flags. Returns a `*DistinctValuesResponse` with `Collection`, `Field`,
  `Values` (sorted), and `Count`.

- **`DistinctValuesQuery` and `DistinctValuesResponse` types** — New types for
  the distinct values API in `search.go`.

## [0.12.0] - 2026-03-11

### Added

- **`POST /api/embed` direct endpoint support** — `Embed()` now calls the
  server's `/api/embed` endpoint directly instead of creating temporary
  collections and scripts. Much faster and cleaner.

- **`EmbedBatch()` method** — Generate embeddings for multiple texts in a single
  request.

- **`EmbedRequest` and `EmbedResponse` types** — New types for the embed API.

- **`ToolConfig` and `ToolChoice` types** — New types for configuring tool
  calling in chat sessions. `ToolConfig` controls enabled tools, allowed
  collections, max iterations, write permissions, and tool choice strategy.

- **`MaxTokens`, `Temperature`, `ToolConfig` on `CreateChatSessionRequest`** —
  Control LLM generation parameters and tool calling per session.

- **`MaxIterations`, `ToolConfig` on `ChatMessageRequest`** — Override tool
  config and max iterations on a per-message basis.

- **`MaxContextMessages`, `BypassRipple`, `Memory` on `UpdateSessionRequest`** —
  Allow updating context window size, ripple sync, and memory on existing
  sessions.

- **`MergeStrategyInterleaved`** — Added `Interleaved` merge strategy for
  round-robin message merging from source sessions.

- **`BypassRipple` on `MergeSessionsRequest`** — Control ripple replication
  during session merge operations.

- **`BoolPtr`, `Float32Ptr`, `Int32Ptr` helpers** — Convenience functions for
  creating pointers to bool, float32, and int32 values.

### Fixed

- **`EmbedBatch()` missing input validation** — Added early return with error
  when `texts` slice is empty, preventing unnecessary HTTP requests. Added
  response length validation to catch server-side mismatches.

### Testing

- **`Embed()` / `EmbedBatch()` unit tests** — Added tests for successful embed,
  batch embed, empty input validation, and response count mismatch.

## [0.11.1] - 2026-02-14

### Fixed

- **Thread-safe token management** — Added `sync.RWMutex` to protect the `token`
  field on `Client`, eliminating a data race where concurrent goroutines could
  read stale tokens indefinitely after a server restart
- **Double-check token refresh** — `refreshTokenIfStale()` skips redundant HTTP
  refresh calls when another goroutine has already refreshed the token,
  preventing thundering herd 401 errors on the server after instance restarts
- **WebSocket token read** — `WebSocket()` now reads the token via the
  thread-safe `getToken()` accessor instead of directly accessing the field

## [0.11.0] - 2026-02-08

### Added

- **Chat Models API** — Query available AI models across providers:
  - `GetChatModels()` — Retrieve all available chat models from all providers
    (OpenAI, Anthropic, Perplexity)
  - `GetChatModel(providerName string)` — Retrieve available models for a
    specific provider
  - `GetChatMessage(sessionID, messageID string)` — Get a specific chat message
    by ID
  - `ChatModels` struct — Contains lists of available models by provider
- **User Functions API** — Reusable function sequences with lifecycle
  management:
  - `SaveUserFunction(userFunction UserFunction)` — Create a new reusable user
    function
  - `GetUserFunction(label string)` — Retrieve a user function by label
  - `ListUserFunctions(tags []string)` — List all user functions, optionally
    filtered by tags
  - `UpdateUserFunction(label string, userFunction UserFunction)` — Update an
    existing user function
  - `DeleteUserFunction(label string)` — Delete a user function
  - `UserFunction` struct — Label, Name, Description, Version, Parameters,
    Functions, Tags, ID, CreatedAt, UpdatedAt
- **Collection utilities**:
  - `CollectionExists(collection string)` — Check if a collection exists
    (returns bool)
  - `CountDocuments(collection string)` — Count all documents in a collection

### Changed

- Updated README with `CountDocuments` return type and `GetChatModels` signature

## [0.10.0] - 2026-01-27

### Changed

- **Breaking**: `StageKvGet` signature simplified — removed `outputField`
  parameter
  - Old: `StageKvGet(key string, outputField *string)`
  - New: `StageKvGet(key string)`
  - Returns `{value: <data>}` on hit, `{value: null}` on miss

### Fixed

- **KVBatchSet value handling** — Fixed value wrapping: now directly uses the
  entry value map instead of double-wrapping in `{"value": ...}`. Added
  validation that value is a `map[string]interface{}` and not nil

## [0.9.0] - 2026-01-27

### Added

- **Field Projection** — Control which fields are returned in query results:
  - `FindByIDWithProjection(collection, id string, selectFields, excludeFields []string)`
    — Find by ID with field whitelist/blacklist
  - `SelectFields()` / `ExcludeFields()` on `QueryBuilder` — Projection methods
    for query builder
- **KV Batch Operations** — Efficient multi-key access in single requests:
  - `KVBatchGet(keys []string)` — Retrieve multiple keys
  - `KVBatchSet(entries []map[string]interface{})` — Set multiple key-value
    pairs with optional TTL
  - `KVBatchDelete(keys []string)` — Delete multiple keys
- **StageSWR** — Stale-While-Revalidate function stage for external API caching:
  - Automatic workflow: KV cache check → HTTP request → KV cache set → optional
    audit storage
  - Supports parameter substitution (e.g., `"user:{{user_id}}"`)
  - TTL accepts duration strings (`"15m"`, `"1h"`), integers (seconds), or ISO
    timestamps
- **ScriptCondition types** — Recursive condition system for function If/control
  flow:
  - `ConditionHasRecords()`, `ConditionFieldExists(field)`,
    `ConditionFieldEquals(field, value)`
  - `ConditionCountEquals(count)`, `ConditionCountGreaterThan(count)`,
    `ConditionCountLessThan(count)`
  - `ConditionAnd(conditions)`, `ConditionOr(conditions)`,
    `ConditionNot(condition)`
  - Custom `MarshalJSON()` for adjacently-tagged serialization matching Rust
    server's serde format

### Changed

- **Breaking**: `ScriptCondition` JSON serialization now uses adjacently-tagged
  format
  - Old: `{"type": "FieldEquals", "field": "x", "value": "y"}`
  - New: `{"type": "FieldEquals", "value": {"field": "x", "value": "y"}}`
  - `HasRecords` remains simple: `{"type": "HasRecords"}`

### Testing

- Added `projection_test.go` — QueryBuilder projection and
  FindByIDWithProjection tests (294 lines)
- Added `client_kv_batch_test.go` — KV batch operation tests (205 lines)
- Added `condition_test.go` — ScriptCondition serialization tests (330 lines)
- Added `swr_test.go` — StageSWR serialization and format tests (243 lines)

## [0.8.0] - 2026-01-06

### Added

- **Options structs** — Variadic options pattern for cleaner, extensible method
  signatures:
  - `InsertOptions` — TTL, BypassRipple, TransactionId, BypassCache
  - `UpdateOptions` — BypassRipple, TransactionId, BypassCache, SelectFields,
    ExcludeFields
  - `DeleteOptions` — BypassRipple, TransactionId
  - `FindOptions` — Filter, Sort, Limit, Skip, Join, BypassCache, BypassRipple,
    SelectFields, ExcludeFields
  - `UpsertOptions` — TTL, BypassRipple, TransactionId, BypassCache
  - `BatchInsertOptions`, `BatchUpdateOptions`, `BatchDeleteOptions`
- **Convenience methods** for ergonomic API usage:
  - `Upsert(collection, id string, record Record, opts ...UpsertOptions)` —
    Atomic insert-or-update (tries update first, falls back to insert on 404)
  - `FindOne(collection, field string, value interface{})` — Find single record
    by field value
  - `Exists(collection, id string)` — Check if record exists by ID (returns
    bool)
  - `Paginate(collection string, page, pageSize int)` — Paginated retrieval
    (1-indexed pages)
  - `RestoreRecord(collection, id string)` — Restore a deleted record from trash
  - `RestoreCollection(collection string)` — Restore all deleted records in a
    collection
- **Search projection** — Added `SelectFields` and `ExcludeFields` to
  `SearchQuery` and `SearchQueryBuilder`

### Changed

- **Breaking**: All CRUD method signatures now accept variadic options structs
  instead of positional parameters:
  - `Insert(collection, record, opts ...InsertOptions)` (was `...string` for
    TTL)
  - `Update(collection, id, record, opts ...UpdateOptions)`
  - `Delete(collection, id, opts ...DeleteOptions)`
  - `Find(collection, query, opts ...FindOptions)`
  - `BatchInsert(collection, records, opts ...BatchInsertOptions)`
  - `BatchUpdate(collection, updates, opts ...BatchUpdateOptions)`
  - `BatchDelete(collection, ids, opts ...BatchDeleteOptions)`

### Testing

- Added `convenience_test.go` — Tests for Upsert, FindOne, Exists, Paginate (184
  lines)
- Comprehensive client tests for all new option structs and convenience methods
  (800+ lines added to `client_test.go`)

## [0.7.1] - 2026-01-03

### Added

- Comprehensive unit tests across all client methods
- Test coverage for CRUD operations, batch operations, collections, KV store,
  transactions, search, functions, and chat operations
- Unit test suite in `client_test.go` and `query_builder_test.go`

### Fixed

- Standardized isolation level constants and validation
- Error handling improvements across all operations
- Transaction isolation level type consistency

## [0.7.0] - 2026-01-03

### Added

- Transaction support with full CRUD operations
- Transaction isolation levels (ReadUncommitted, ReadCommitted, RepeatableRead,
  Serializable)
- Savepoint support for nested transactions
- KV store utilities:
  - `KVExists()` - Check if key exists
  - `KVIncrement()` - Atomic counter increment
  - `KVDecrement()` - Atomic counter decrement
  - `KVAppend()` - Append to list values
- Dependabot configuration for automated dependency updates

### Changed

- Enhanced transaction API with better error handling
- Improved documentation for transaction methods

## [0.6.1] - 2026-01-02

### Added

- Type-specific getValue helpers for extracting values from ekoDB responses
- `getStringValue()`, `getIntValue()`, `getBoolValue()`, `getFloat64Value()`,
  `getMapValue()`, `getSliceValue()` utility functions
- Simplified value extraction from nested field structures

### Changed

- Updated examples to use new type utility functions
- Improved type safety in example code

## [0.6.0] - 2025-12-31

### Added

- **Functions and Scripts** support
  - `CreateFunction()` - Register server-side functions
  - `ExecuteFunction()` - Execute registered functions
  - `ListFunctions()` - List all available functions
  - `GetFunction()` - Get function details
  - `UpdateFunction()` - Update existing functions
  - `DeleteFunction()` - Remove functions
- Function versioning support (optional tags)
- Dynamic function examples with runtime variables
- Standardized inter-stage function composition

### Changed

- Enhanced function execution with better variable handling
- Improved function stage configuration

## [0.5.0] - 2025-12-21

### Added

- Self-improving RAG (Retrieval-Augmented Generation) helper functions
- RAG utilities in `rag_helpers.go`:
  - `CreateRAGPipeline()` - Set up RAG workflows
  - `QueryRAG()` - Execute RAG queries
  - `OptimizeRAGEmbeddings()` - Improve embeddings over time
- Enhanced documentation for RAG patterns

### Changed

- Removed example files (consolidated in main ekodb-client repository)
- Updated dependencies
- Improved README formatting

## [0.4.0] - 2025-12-20

### Added

- **Functions and Scripts** - Initial implementation
- Server-side function execution support
- Script management operations
- Function examples and documentation
- Example count tracking in README

## [0.2.0] - 2025-11-05

### Added

- **MessagePack serialization** support for binary data transfer
- **Gzip compression** for reduced bandwidth usage
- Configurable serialization format (JSON or MessagePack)
- Compression toggle for all requests
- Performance improvements with binary protocol

### Changed

- Updated client to support MessagePack + Gzip
- Enhanced test suite with compression benchmarks
- Updated dependencies:
  - Added `github.com/vmihailenco/msgpack/v5`

### Fixed

- Code formatting and linting improvements

## [0.1.4] - 2025-10-14

### Changed

- Updated Makefile with improved commands
- Enhanced README with better formatting and examples
- Documentation improvements

## [0.1.3] - 2025-10-14

### Fixed

- Removed incorrect repository references
- Corrected package documentation

## [0.1.2] - 2025-10-14

### Added

- `Regex()` query method for pattern matching
- Enhanced README with more examples

### Changed

- Improved query builder documentation

## [0.1.1] - 2025-10-14

### Fixed

- Updated `publish.sh` script for standalone repository
- Corrected publishing workflow

## [0.1.0] - 2025-10-14

### Added

- Initial Go client library release
- Core CRUD operations:
  - `Insert()`, `Find()`, `FindByID()`, `Update()`, `Delete()`
- Batch operations:
  - `BatchInsert()`, `BatchUpdate()`, `BatchDelete()`
- Collection management:
  - `CreateCollection()`, `ListCollections()`, `DeleteCollection()`
- Query builder with fluent API:
  - Filters: `Eq()`, `Ne()`, `Gt()`, `Gte()`, `Lt()`, `Lte()`, `In()`,
    `Contains()`, `StartsWith()`, `EndsWith()`
  - Sorting: `SortBy()`, `SortDesc()`
  - Pagination: `Skip()`, `Limit()`
  - Projection: `Fields()`
  - Logical operators: `And()`, `Or()`, `Not()`
- Search operations with BM25 scoring
- Schema management and validation
- WebSocket support for real-time queries
- KV store operations:
  - `KVGet()`, `KVSet()`, `KVDelete()`, `KVList()`
- TTL support for automatic document expiration
- Comprehensive error handling
- Rate limit tracking
- Retry logic with exponential backoff
- API key authentication
- Full documentation and examples

### Dependencies

- `github.com/gorilla/websocket` v1.5.3
- Go 1.24.0+
