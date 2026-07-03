#!/usr/bin/env python3
"""
Validates n8n workflow JSON files for structural correctness.

Checks performed per file:
  - Valid JSON (replaces the previous `jq empty` check)
  - Top-level required fields: name (non-empty string), nodes (non-empty array),
    connections (object)
  - Node-level required fields: id, name, type, typeVersion, position ([x, y])
  - No duplicate node IDs within a workflow
  - At least one trigger node (webhook, scheduleTrigger, etc.)
  - Webhook nodes have a non-empty path parameter
  - Webhook nodes reachable by our own signed Go backend (i.e. not fed by
    Prometheus Alertmanager, which cannot send our HMAC signature — see
    ALERTMANAGER_TRIGGERED_FILES) must feed a "Verify Signature" code node
    immediately after the trigger, with Raw Body enabled on the trigger. This
    prevents a regression back to the pre-fix state where any request that
    reached the webhook path (e.g. from a compromised sibling container) could
    forge a KYC/payout event and trigger a phishing email or a fraudulent
    manual-transfer instruction to an operator.

Cross-file checks:
  - Webhook paths are unique across all workflows — n8n silently overwrites the
    earlier registration when two active workflows share a path, causing one of
    them to stop receiving events.

Connection graph integrity:
  - Every connection source key matches an existing node name
  - Every connection destination node name matches an existing node name

Usage:
  python3 scripts/validate-n8n-workflows.py [workflow-dir]

  workflow-dir defaults to observability/n8n/workflows/
"""
import json
import pathlib
import sys

TRIGGER_TYPES = frozenset(
    {
        "n8n-nodes-base.webhook",
        "n8n-nodes-base.scheduleTrigger",
        "n8n-nodes-base.cronTrigger",
        "n8n-nodes-base.manualTrigger",
        "n8n-nodes-base.emailReadImap",
    }
)

# Workflows whose webhook is called by Prometheus Alertmanager (see
# observability/alertmanager.yml), not by our Go backend's signed dispatcher
# (internal/observability/notifier.go, internal/notification/dispatcher/admin.go).
# Alertmanager has no way to compute our HMAC-SHA256 X-Signature, so these are
# intentionally exempt from the signature-verification requirement below.
ALERTMANAGER_TRIGGERED_FILES = frozenset(
    {
        "circuit-breaker-alert.json",
        "payment-error-escalation.json",
        "prometheus-alert-relay.json",
    }
)

workflow_dir = (
    pathlib.Path(sys.argv[1])
    if len(sys.argv) > 1
    else pathlib.Path("observability/n8n/workflows")
)

errors: list[str] = []
# path → filename — used to detect cross-workflow webhook path collisions.
seen_webhook_paths: dict[str, str] = {}

workflow_files = sorted(workflow_dir.glob("*.json"))
if not workflow_files:
    print(f"No workflow files found in {workflow_dir}", file=sys.stderr)
    sys.exit(1)

for fpath in workflow_files:
    fname = fpath.name

    # ── JSON syntax ──────────────────────────────────────────────────────────
    try:
        data = json.loads(fpath.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        errors.append(f"{fname}: invalid JSON — {exc}")
        continue

    # ── Top-level structure ──────────────────────────────────────────────────
    name = data.get("name")
    if not isinstance(name, str) or not name.strip():
        errors.append(f"{fname}: 'name' must be a non-empty string")

    nodes = data.get("nodes")
    if not isinstance(nodes, list):
        errors.append(f"{fname}: 'nodes' must be an array")
        continue  # node-level checks require a valid array
    if len(nodes) == 0:
        errors.append(f"{fname}: 'nodes' array is empty — workflow will never execute")
        continue

    connections = data.get("connections")
    if not isinstance(connections, dict):
        errors.append(f"{fname}: 'connections' must be an object")
        connections = {}

    # ── Node-level checks ────────────────────────────────────────────────────
    node_names: set[str] = set()
    node_ids: set[str] = set()
    has_trigger = False

    for node in nodes:
        node_id = node.get("id", "")
        node_name = node.get("name", "")
        node_type = node.get("type", "")

        if not node_id:
            errors.append(
                f"{fname}: node '{node_name or '(unnamed)'}' is missing 'id' — "
                "n8n requires a unique ID per node"
            )
        elif node_id in node_ids:
            errors.append(
                f"{fname}: duplicate node id '{node_id}' — n8n will reject the import"
            )
        else:
            node_ids.add(node_id)

        if not node_name:
            errors.append(f"{fname}: node id '{node_id or '(no-id)'}' is missing 'name'")
        else:
            node_names.add(node_name)

        if not node_type:
            errors.append(f"{fname}: node '{node_name}' is missing 'type'")

        if "typeVersion" not in node:
            errors.append(
                f"{fname}: node '{node_name}' is missing 'typeVersion' — "
                "n8n uses this to select the correct node behaviour"
            )

        position = node.get("position")
        if not isinstance(position, list) or len(position) != 2:
            errors.append(
                f"{fname}: node '{node_name}' 'position' must be a two-element array [x, y]"
            )

        if node_type in TRIGGER_TYPES:
            has_trigger = True

        # Webhook-specific: path must be a non-empty string
        if node_type == "n8n-nodes-base.webhook":
            path_val = node.get("parameters", {}).get("path", "")
            if not isinstance(path_val, str) or not path_val.strip():
                errors.append(
                    f"{fname}: webhook node '{node_name}' has an empty or missing 'path' — "
                    "n8n will not register the endpoint"
                )
            elif path_val in seen_webhook_paths:
                errors.append(
                    f"{fname}: webhook path '{path_val}' collides with "
                    f"{seen_webhook_paths[path_val]} — n8n silently overwrites the earlier "
                    "registration; one workflow will stop receiving events"
                )
            else:
                seen_webhook_paths[path_val] = fname

    if not has_trigger:
        errors.append(
            f"{fname}: no trigger node found (webhook, scheduleTrigger, etc.) — "
            "the workflow will never execute automatically"
        )

    # ── Signature verification (backend-triggered webhooks only) ────────────
    if fname not in ALERTMANAGER_TRIGGERED_FILES:
        nodes_by_name = {n.get("name"): n for n in nodes}
        for node in nodes:
            if node.get("type") != "n8n-nodes-base.webhook":
                continue
            node_name = node.get("name", "")

            raw_body = (
                node.get("parameters", {}).get("options", {}).get("rawBody")
            )
            if raw_body is not True:
                errors.append(
                    f"{fname}: webhook node '{node_name}' must set "
                    "parameters.options.rawBody = true so the Verify Signature "
                    "node can validate the exact signed bytes"
                )

            first_targets = (
                connections.get(node_name, {}).get("main", [[]])[0]
                if isinstance(connections.get(node_name, {}).get("main"), list)
                and connections.get(node_name, {}).get("main")
                else []
            )
            first_target_name = first_targets[0].get("node") if first_targets else None
            first_target = nodes_by_name.get(first_target_name)

            if (
                first_target is None
                or first_target.get("name") != "Verify Signature"
                or first_target.get("type") != "n8n-nodes-base.code"
            ):
                errors.append(
                    f"{fname}: webhook node '{node_name}' must feed a "
                    "'Verify Signature' code node immediately after the trigger — "
                    "this workflow is reachable by anyone who can reach the "
                    "webhook path unless the HMAC signature is checked first"
                )

    # ── Connection graph integrity ────────────────────────────────────────────
    for src_name, outputs in connections.items():
        if src_name not in node_names:
            errors.append(
                f"{fname}: connection source '{src_name}' does not match any node name"
            )
        if not isinstance(outputs, dict):
            continue
        for _output_type, chains in outputs.items():
            if not isinstance(chains, list):
                continue
            for chain in chains:
                if not isinstance(chain, list):
                    continue
                for conn in chain:
                    dest = conn.get("node", "")
                    if dest and dest not in node_names:
                        errors.append(
                            f"{fname}: connection destination '{dest}' does not match "
                            "any node name — the wiring will be broken after import"
                        )

if errors:
    print(
        f"n8n workflow validation failed ({len(errors)} error(s)):",
        file=sys.stderr,
    )
    for err in errors:
        print(f"  {err}", file=sys.stderr)
    sys.exit(1)

print(f"n8n validation passed: {len(workflow_files)} workflow(s) OK")
sys.exit(0)
