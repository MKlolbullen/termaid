# Termaid Workflow v3

Termaid v3 separates **what the workflow means** from **where it is drawn**.

The matrix (`layer`, `position`, subgraphs) remains useful for the TUI and Mermaid layout, but it is no longer the execution scheduler. Runtime ordering is driven by explicit DAG dependencies.

## Why v3

The v2 format could describe a node as `parallel: true` while also chaining it to another supposedly parallel node. Runtime execution was additionally grouped by visual layers, which made layout act as an accidental barrier.

v3 fixes that by making these concepts first-class:

- dependency-driven scheduling
- fan-out / fan-in
- typed artifacts
- data edges and control-only edges
- merge nodes
- transform nodes
- scope / policy gates
- approval boundaries
- conditional dispatch
- retry / timeout policy
- durable checkpoints and resume
- provenance
- finding correlation
- explicit verification and reporting stages

v2 workflows remain loadable. Empty type declarations are treated as untyped compatibility contracts.

---

## Node kinds

| Kind | Purpose |
| --- | --- |
| `source` | Initial target/artifact source. Termaid supplies the implicit `input` source. |
| `worker` | Execute an external CLI tool. |
| `merge` | Fan-in multiple compatible artifacts, normalize, sort and deduplicate. |
| `transform` | Convert one artifact representation to another. |
| `gate` | Enforce scope, authorization, approval or other workflow policy. |
| `checkpoint` | Explicit durable workflow boundary. The runtime also checkpoints after every execution wave. |
| `manual` | Human verification / approval checkpoint. |
| `sink` | Final correlation/report/evidence consumer. |

Example:

```json
{
  "id": "surface-merge",
  "kind": "merge",
  "tool": "merge",
  "inputs": ["url"],
  "outputs": ["url"],
  "layer": 7,
  "position": 0
}
```

## Artifact contracts

Supported built-in artifact types:

- `domain`
- `host`
- `service`
- `url`
- `parameter`
- `technology`
- `finding`
- `evidence`
- `report`
- `any`

A normal data edge must connect compatible output/input types. For example:

```text
Host[] -> Host[]       valid
URL[]  -> Finding[]    valid only when the child INPUT is URL[]
Host[] -> URL[]        invalid without a transform
```

Use a transform node when the representation actually changes:

```json
{
  "id": "service-to-url",
  "kind": "transform",
  "transform": "host-to-url",
  "inputs": ["service"],
  "outputs": ["url"]
}
```

`termaid validate` rejects incompatible typed data edges before a run starts.

---

## Data edges vs control edges

A **data edge** contributes its parent artifact to the child's input.

```json
{"from": "surface-merge", "to": "wordpress-nuclei"}
```

A **control edge** participates in dependency readiness and conditions but does not contribute data to the child.

```json
{
  "from": "technology-merge",
  "to": "wordpress-nuclei",
  "condition": "contains:wordpress",
  "label": "WordPress detected",
  "control": true
}
```

This matters for adaptive workflows. The WordPress worker can consume canonical `URL[]` from one parent while technology evidence from another parent only decides whether that worker should run.

Without control edges, fingerprint text would be incorrectly merged into the URL corpus.

---

## True fan-out / fan-in

Parallel workers should be siblings, not a fake sequence.

Bad:

```text
ffuf -> gobuster -> feroxbuster -> dirsearch
```

Correct model:

```text
                -> worker A -\
URL corpus -----+-> worker B ---+-> merge -> next stage
                -> worker C -/
```

The v3 scheduler starts every ready node up to the configured concurrency limit. A merge node becomes ready when its dependencies have reached terminal state.

Visual `layer` values do not determine dependency order.

---

## Conditions

Conditions are intentionally small and deterministic. Supported expressions:

| Condition | Meaning |
| --- | --- |
| `always` | Always eligible. Empty condition means the same thing. |
| `nonempty` | At least one input artifact has data. |
| `contains:text` | Input data contains `text`, case-insensitive. |
| `has_type:url` | Parsed input contains the requested inferred record type. |
| `approved:name` | Named approval was supplied to the run. |
| `A && B` | Both conditions must match. |
| `A || B` | At least one condition must match. |

Conditions may live on edges or nodes.

A false control-edge condition prunes the target branch rather than reporting a tool failure.

---

## Scope policy

Workflow-wide policy example:

```json
{
  "policy": {
    "allowed_roots": ["{{domain}}"],
    "excluded": ["admin.example.com"],
    "allow_intrusive": false,
    "max_concurrency": 12
  }
}
```

A scope gate can enforce the artifact stream itself:

```json
{
  "id": "scope-gate",
  "kind": "gate",
  "tool": "scope",
  "transform": "scope",
  "inputs": ["domain"],
  "outputs": ["domain"],
  "tags": ["scope"]
}
```

Scope checks are engine semantics, not something each individual tool adapter has to reimplement.

---

## Intrusive testing and approvals

Discovery and low-impact analysis can run automatically while active validation is separated behind an explicit gate.

```json
{
  "id": "active-validation-gate",
  "kind": "gate",
  "inputs": ["parameter"],
  "outputs": ["parameter"],
  "policy": {
    "requires_approval": true,
    "approval": "active-validation"
  }
}
```

Downstream workers can additionally mark themselves intrusive:

```json
{
  "id": "sqlmap-1",
  "kind": "worker",
  "tool": "sqlmap",
  "policy": {
    "intrusive": true,
    "approval": "active-validation"
  }
}
```

CLI examples:

```bash
# Passive / discovery branches only. Approval-gated branches are skipped.
termaid run -d example.com -w templates/webapp-comprehensive-scan.json

# Approve the focused validation boundary.
termaid run -d example.com \
  -w templates/webapp-comprehensive-scan.json \
  --approve active-validation

# Broadly authorize nodes marked intrusive for an already-authorized engagement.
termaid run -d example.com \
  -w workflow.json \
  --approve-intrusive

# Multiple named gates.
termaid run -d example.com \
  -w workflow.json \
  --approve active-validation,manual-verification
```

Approval is a runtime input; it is not inferred from a workflow merely containing an intrusive tool.

---

## Reliability controls

Worker nodes can specify:

```json
{
  "execution": {
    "timeout_seconds": 900,
    "retries": 1,
    "retry_backoff_ms": 750
  }
}
```

The global concurrency value comes from `termaid run -c N` and may be capped by `policy.max_concurrency`.

Cancellation propagates through the run context. External commands are started with `exec.CommandContext` so cancellation terminates active workers.

---

## Checkpoints and resume

Termaid writes:

```text
workdir/<run-id>/checkpoint.json
```

after each execution wave.

The checkpoint contains:

- global node states
- output artifact locations
- execution statistics
- per-node output metadata
- provenance needed to resume completed nodes

Resume with:

```bash
termaid run \
  -d example.com \
  -w workflow.json \
  -o workdir \
  --resume run-1787320000
```

If a node was recorded complete but its output artifact no longer exists, it is demoted to pending and reproduced rather than blindly trusted.

---

## Provenance

Every completed node output records metadata including:

- node kind
- upstream parent node IDs
- attempt count
- node condition
- declared input artifact types
- declared output artifact types
- original tool / command identity
- timestamps and exit code

This allows later evidence/report code to answer not only **what was found**, but **where the observation came from**.

---

## Correlation

`CorrelateRecords` groups observations using a stable fingerprint based on structured fields when available:

```text
asset + location + weakness + evidence
```

Legacy/unstructured records fall back to a normalized-value fingerprint.

The highest-confidence observation becomes the representative record while `metadata.sources` retains every contributing tool.

This prevents the report layer from turning the same issue reported by several scanners into several vulnerabilities.

---

## Verification model

The recommended final stages are:

```text
candidate findings
      |
      v
checkpoint
   /      \
  /        \
automated  manual verification
  \        /
   \      /
 evidence merge
      |
      v
correlation/report sink
```

A scanner result is a **candidate observation**. Verification/evidence is a separate lifecycle state.

The reference `templates/webapp-comprehensive-scan.json` implements this model.

---

## v2 compatibility

Termaid continues to load v2 workflows:

- `children` is promoted into explicit edges
- missing `kind` defaults to `worker`
- missing artifact declarations are treated as untyped
- visual matrix fields remain supported

Saving/exporting through the v3 graph serializer emits `version: "3.0"` and explicit semantic edges.

---

## Recommended workflow design rules

1. Treat the matrix as layout, never as the source of truth for scheduling.
2. Fan independent workers out from a common artifact.
3. Fan results back into a merge node before expensive downstream work.
4. Type every mature workflow boundary.
5. Use transforms instead of lying about artifact types.
6. Use control edges for evidence that changes *whether* a branch runs but is not branch input.
7. Keep intrusive workers behind explicit policy/approval boundaries.
8. Correlate candidates before reporting.
9. Preserve raw evidence and provenance.
10. Treat verification as a separate phase from discovery.
