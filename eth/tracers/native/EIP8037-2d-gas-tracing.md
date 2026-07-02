# Two-Dimensional Gas (regular / state) in Execution Tracing

**Cross-client specification — Draft v0.3**
**Target fork:** Amsterdam (Glamsterdam) and later
**Depends on:** EIP-8037 (2D state/regular gas) · EIP-7778 (block accounting without refunds) · EIP-3529 (refunds)

> **Goal:** define a single, client-agnostic way for the `debug_trace*` methods to expose the **regular-gas / state-gas** breakdown introduced by EIP-8037, so block builders, gas profilers, wallets, explorers and L2s get the same fields with the same semantics from every client.

> **Status (v0.3):** the callTracer surface (§4) is implemented and live-validated in a geth patch — see §11.

---

## 1. Motivation

EIP-8037 splits gas into two independently-metered dimensions:

- **`regular_gas`** — all compute/memory/access costs, plus the "new regular cost" of state-creating ops.
- **`state_gas`** — the "new state cost" of state creation only: `SSTORE` into a new slot, account creation (`CREATE`/`CREATE2` and value-carrying `CALL` to a new account), code deposit, EIP-7702 auth to a new account. Priced at a fixed `CPSB = 1530` gas per new state byte.

Glamsterdam changes gas accounting with **three interacting EIPs**, and their combined effect is invisible over today's RPC:

- **EIP-8037** — the 2D split above.
- **EIP-7778** ("Block Gas Accounting without Refunds") — the **block** counters count **pre-refund (gross)** gas, so EIP-3529 refunds can't be used to push extra compute past the gas limit.
- **EIP-3529** — refunds still reduce what the **user pays** (the receipt), but no longer reduce block gas.

| quantity | refund? | formula |
|---|---|---|
| tx block contribution | **pre**-refund (gross) | `block_regular += tx_regular_gas; block_state += tx_state_gas` |
| `header.gasUsed` | pre-refund, bottleneck | `max(Σ block_regular, Σ block_state)` |
| `receipt.gasUsed` / `cumulativeGasUsed` | **post**-refund, post-floor | `max(tx_gas_used − gas_refund, floor)` |

Three distinct quantities exist — the **regular/state split**, the **pre-refund gross** gas, and the **refund** — and **none** is exposed by any endpoint. In particular `header.gasUsed` (pre-refund max) and `Σ receipt.gasUsed` (post-refund sum) are different quantities that **diverge in both directions** (measured on `glamsterdam-devnet-6`):

| block | header.gasUsed | Σ receipt.gasUsed | header − Σreceipt | bound by |
|---|--:|--:|--:|---|
| 38807 | 26,437,902 | 31,185,485 | −4,747,583 | state-heavy (Σstate > Σrefund) |
| 38541 | 55,882,773 | 63,946,345 | −8,063,572 | state-heavy |
| 38814 | 4,854,247 | 4,403,161 | +451,086 | refund-heavy (Σrefund > Σstate) |

For a regular-bottleneck block, `header − Σreceipt = Σ gas_refund − Σ state_gas` — which is why the sign flips with the block's refund/state balance, and why the two cannot be reconciled without the explicit fields this spec adds.

Who needs the split: **block builders/relays** (pack against two budgets), **gas profilers/wallets/simulators** ("compute vs permanent state?"), **fee estimators** (the bottleneck dimension drives the next base fee), **explorers/analytics** (reconcile header vs receipts).

## 2. Scope

Covers the tracer output of `debug_traceTransaction`, `debug_traceCall`, `debug_traceBlockByHash`, `debug_traceBlockByNumber`, `debug_traceBlock`, `debug_traceBlockFromFile`, `debug_traceBadBlock`, `debug_traceChain` — **only when the request selects `callTracer`** (§4) or the default struct/opcode logger (§5). Additive: no existing field changes meaning; new fields appear only for Amsterdam+ blocks. A companion receipt/header surface (§6) is recommended but is not part of `debug`.

## 3. Terminology

| EIP-8037 / -7778 | JSON | meaning |
|---|---|---|
| `tx_regular_gas` | `regularGasUsed` | pre-refund (gross) regular gas — EIP-7778 block contribution of the frame/tx |
| `tx_state_gas` | `stateGasUsed` | pre-refund (gross) state gas — EIP-7778 block contribution of the frame/tx |
| `gas_refund` | `gasRefund` | EIP-3529 refund; reduces the receipt (post-refund), not the block counters |
| `state_gas_reservoir` | `stateGasReservoir` | remaining reservoir (opcode logger only) |
| `gas_left` | existing `gas` | regular-gas remaining; the `GAS` opcode returns this only |

All values hex-encoded quantities (`0x`-prefixed, minimal), like existing gas fields.

## 4. `callTracer`

Each call-frame gains (Amsterdam+):

```jsonc
{
  "from": "0x…", "to": "0x…", "type": "CALL",
  "gas": "0x…",            // unchanged: gas provided to the frame
  "gasUsed": "0x…",        // unchanged: POST-refund gas (matches receipt for the top frame)
  "regularGasUsed": "0x…", // NEW: PRE-refund (gross) regular-gas — EIP-7778 block contribution, incl. children
  "stateGasUsed":   "0x…", // NEW: PRE-refund (gross) state-gas — EIP-7778 block contribution, incl. children
  "gasRefund":      "0x…", // NEW (SHOULD): EIP-3529 refund applied to this frame (0 if none)
  "calls": [ … ]
}
```

**Semantics.** `regularGasUsed`/`stateGasUsed` are the **pre-refund (gross) EIP-7778 block contributions** — the quantities that flow into `block_regular_gas_used`/`block_state_gas_used` and thus determine block fullness and the next base fee. They are deliberately **not** post-refund, because that is the information no other endpoint carries. `gasUsed` is unchanged (post-refund receipt value); the two are bridged by `gasRefund`.

- **Top frame — MUST.** `regularGasUsed`/`stateGasUsed` equal the tx's `tx_regular_gas`/`tx_state_gas` contributions to the block counters, and MUST satisfy `regularGasUsed + stateGasUsed == gasUsed + gasRefund` (the gross pre-refund tx gas; EIP-7623 floor is the only exception, see I4).
- **Nested frames — SHOULD.** Report the gross dimension gas charged in that frame **including children** (mirroring `gasUsed`). Only the top frame is refund-exact. Clients that cannot cheaply attribute per-frame MAY omit nested fields but MUST emit them on the top frame.
- **Before Amsterdam:** the three fields are omitted; `gasUsed` keeps today's meaning.

## 5. Default struct / opcode logger

Result object gains tx-level totals; each step MAY carry the state portion of its charge:

```jsonc
{
  "failed": false,
  "gas": "0x…",              // unchanged: tx gasUsed (POST-refund, post-floor = receipt)
  "regularGasUsed": "0x…",   // NEW: tx gross regular-gas (block contribution)
  "stateGasUsed":   "0x…",   // NEW: tx gross state-gas (block contribution)
  "gasRefund":      "0x…",   // NEW: EIP-3529 refund (bridges gas ↔ regular+state)
  "returnValue": "0x…",
  "structLogs": [
    { "pc": 1234, "op": "SSTORE", "gas": "0x…", "gasCost": "0x…",
      "stateGasCost": "0x…",      // NEW (SHOULD): state-gas portion of gasCost (0 for non-state ops)
      "stateGasReservoir": "0x…", // NEW (MAY): reservoir remaining before the op
      "depth": 2, "stack": [ … ] }
  ]
}
```

`gasCost` stays the **total** (`regular + state`); `gasCost − stateGasCost` is the regular portion. `gas` continues to report `gas_left` (per EIP-8037's redefinition of `GAS`), not the reservoir.

## 6. Companion receipt / header fields (recommended, non-`debug`)

- `eth_getTransactionReceipt`: add gross `regularGasUsed`, `stateGasUsed`, `gasRefund`, so `gasUsed == max(regularGasUsed + stateGasUsed − gasRefund, floor)`. **Implementation note:** these must be **persisted with the stored receipt**, not merely derived at execution — a receipt served from disk that only re-derives on trace re-execution reads zero. (This is the one gap in the reference geth patch: the callTracer, which re-executes, is correct; the served receipt is not yet persisted.)
- Block header / `eth_getBlockBy*`: add the two block totals so `gasUsed == max(regularGasUsed, stateGasUsed)` is checkable, and the header↔receipts divergence (§1) is explainable, without tracing.

## 7. Normative invariants

For any Amsterdam+ block:

- **I1** (top frame, gross identity): `regularGasUsed + stateGasUsed == gasUsed + gasRefund`. Exception: when the EIP-7623 calldata floor binds, `gasUsed == floor ≥ (regular+state) − gasRefund` (I4).
- **I2** (block reconciliation): `Σ_tx regularGasUsed == block_regular_gas_used`, `Σ_tx stateGasUsed == block_state_gas_used`, `header.gasUsed == max(Σ regularGasUsed, Σ stateGasUsed)`. (Pre-refund EIP-7778 counters — why `header.gasUsed` generally ≠ `Σ receipt.gasUsed`.)
- **I3**: `regularGasUsed ≥ 0`, `stateGasUsed ≥ 0`, `gasRefund ≥ 0`.
- **I4** (receipt bridge): `receipt.gasUsed == max(regularGasUsed + stateGasUsed − gasRefund, calldata_floor)`. `gasRefund` (EIP-3529, capped at 1/5 of pre-refund gas) reduces the receipt but not the block counters; state-specific refunds (e.g. `CREATE_ACCOUNT_STATE_GAS` on a reverted CREATE) are already netted inside `stateGasUsed`.
- **I5** (opcode logger, if `stateGasCost` emitted): `Σ_steps stateGasCost + intrinsic_state_gas − state refunds == top-frame stateGasUsed`.
- **I6** (pre-fork): before Amsterdam the new fields are absent and legacy behavior is byte-identical.

## 8. Reference test vectors (to ship)

Each gives a tx and expected `{gasUsed, regularGasUsed, stateGasUsed, gasRefund}` at the top frame; run against geth, reth, besu, nethermind, erigon, nimbus-eth1, ethrex.

1. **Pure compute** (no new state, no refund) → `stateGasUsed == 0`, `gasRefund == 0`, `regularGasUsed == gasUsed`.
2. **SSTORE fresh slot** → `stateGasUsed == 64 × 1530` + intrinsic-state.
3. **`CREATE`** (N code bytes) → `stateGasUsed` includes `(120 + N) × 1530`.
4. **Reverted/failed `CREATE`** → state refund applied; `stateGasUsed` excludes the reverted account.
5. **Block-level** — a block where `header.gasUsed = max` ≠ `Σ receipt.gasUsed` (I2).

## 9. Backward compatibility

Purely additive. Consumers ignoring the new fields are unaffected. The fields are the *only* way to recover the dimension split.

## 10. Open decisions

- **D1 — nested-frame attribution.** Recommend all-frames SHOULD, top-frame MUST. *Implementation reality:* the reference geth patch sets the split only on the top frame (in `OnTxEnd`, sourced from the settle-step totals so it matches the block counters exactly). Per-frame attribution needs additional plumbing and is not yet done — "top-frame MUST" is proven, "nested SHOULD" is not yet exercised.
- **D2 — companion receipt/header fields (§6).** Pursue in parallel; fixes an ambiguity pure tracing can't (explorers don't trace every tx). *Recommend: yes, sibling proposal.*
- **D3 — publication.** PR schema + examples into `ethereum/execution-apis` (`debug` namespace), ship §8 vectors as execution-spec-tests fixtures, cross-reference from EIP-8037.
- **D4 — naming.** `regularGasUsed`/`stateGasUsed` mirror EIP-8037's `block_regular_gas_used`/`block_state_gas_used`.
- **D5 — gross vs post-refund values.** This draft exposes the gross (pre-refund) split + `gasRefund` (the quantity that drives block fullness and that nothing else carries); `gasUsed` stays post-refund. *Recommend: gross + `gasRefund`.*

## 11. Implementation status & reference implementation

The model (§1–§7) is confirmed against **two independent execution clients**, and the callTracer surface (§4) is implemented and live-validated:

- **nimbus-eth1** (`transaction/call_common.nim`, `core/tx_pool/tx_packer.nim`) — same gross `tx_regular_gas`/`tx_state_gas`; `header.gasUsed = max(Σregular, Σstate)`; refund + floor affect only the receipt scalar.
- **go-ethereum (glamsterdam branch)** — same split in `core/state_transition.go` `settleGas`, with in-code comments citing EIP-8037/7778/3529. Where the reference patch surfaces the values.

**Reference geth patch (§4):** a 4-file, +28-line patch — `settleGas` captures `tx_regular_gas`/`tx_state_gas`/`gas_refund` → `ExecutionResult` → `MakeReceipt` → callTracer top frame emits `regularGasUsed`/`stateGasUsed`/`gasRefund`. Sourced from the settle step (the same computation that feeds the block counters), so it matches block accounting **by construction**, not re-derived from gas-change hooks.
Diff: https://github.com/qu0b/go-ethereum/commit/da5ea174247456b5c6b3d5d1e2bb46521a9b9a45

**Live validation on glamsterdam-devnet-6** — patched geth joined the network and traced live blocks. Block 38924 (92 txs, 27 with state gas):

```
callTracer top frame:  {"gasUsed":"0x34bfd","regularGasUsed":"0x7ecd","stateGasUsed":"0x2cd30"}
                       = 216,061 = regular 32,461 + state 183,600   (183,600 = 120 × 1530, one new account)

I1  regularGasUsed + stateGasUsed == gasUsed + gasRefund   →  holds 92/92 txs
I2  header.gasUsed 28,902,388 == max(Σregular 28,902,388, Σstate 5,263,200)   →  exact
    Σ receipt.gasUsed 33,843,380 ≠ header  →  the 4.9M gap is now explained by the exposed split
```

**Affected RPC surface:** the eight `debug_trace*` methods, **only** on the `callTracer` output path. Not `flatCallTracer` (Parity flat format re-projects to its own schema), not the struct logger (§5 not yet implemented), no `eth_*` change is required for §4.

**Known gap:** `eth_getTransactionReceipt` (§6) returns the split as `null` — the reference patch derives at execution but does not persist to the stored receipt. The callTracer is unaffected (it re-executes).

## Sources

- EIP-8037 — State Creation Gas Cost Increase (2D gas): https://eips.ethereum.org/EIPS/eip-8037
- EIP-7778 — Block Gas Accounting without Refunds: https://eips.ethereum.org/EIPS/eip-7778
- EIP-3529 — Reduction in refunds; EIP-7623 — calldata floor gas cost.
- Reference implementations cross-checked: nimbus-eth1 `call_common.nim` / `tx_packer.nim`; go-ethereum `core/state_transition.go` `settleGas`.
