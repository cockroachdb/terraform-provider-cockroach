#!/usr/bin/env bash
set -euo pipefail

# GitHub Actions log annotations.
log_error()    { echo "::error title=${2:-Error}::$1"; }
log_warning()  { echo "::warning title=${2:-Warning}::$1"; }
log_notice()   { echo "::notice title=${2:-Notice}::$1"; }
log_group()    { echo "::group::$*"; }
log_endgroup() { echo "::endgroup::"; }

# Write to the GitHub Actions step summary (no-op outside CI).
write_summary() {
  if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
    echo "$1" >> "$GITHUB_STEP_SUMMARY"
  fi
}

# Verify required commands are available.
for cmd in jq curl; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    log_error "Required command not found: ${cmd}" "Missing Dependency"
    write_summary "### Cluster Cleanup"$'\n'"**Error:** Required command \`${cmd}\` not found."
    exit 1
  fi
done

TRACKING_FILE="${CLEANUP_TRACKING_FILE:-}"
API_URL="${COCKROACH_SERVER:-https://cockroachlabs.cloud}"
DRY_RUN="${DRY_RUN:-0}"
MAX_RETRIES=3
RETRY_DELAY=5
STREAM_CANCEL_POLLS=30
STREAM_CANCEL_DELAY=10

if [[ -z "${COCKROACH_API_KEY:-}" ]]; then
  log_error "COCKROACH_API_KEY is not set" "Configuration"
  write_summary "### Cluster Cleanup"$'\n'"**Error:** COCKROACH_API_KEY is not set."
  exit 1
fi

auth_header="Authorization: Bearer ${COCKROACH_API_KEY}"
response_file=$(mktemp)

cleanup_temp_files() {
  rm -f "$response_file"
  if [[ -n "$TRACKING_FILE" && -f "$TRACKING_FILE" ]]; then
    rm -f "$TRACKING_FILE"
    echo "Removed tracking file: ${TRACKING_FILE}"
  fi
}
trap cleanup_temp_files EXIT

echo "=== Cleanup Orphaned Test Clusters ==="
echo "API URL: ${API_URL}"
echo "Tracking file: ${TRACKING_FILE:-<not set>}"
echo "Dry run: ${DRY_RUN}"
echo ""

# delete_cluster deletes a single cluster by ID with retries.
# Arguments: $1=id $2=name
# Returns: 0 on success, 1 on failure.
delete_cluster() {
  local id="$1"
  local name="$2"

  for attempt in $(seq 1 "$MAX_RETRIES"); do
    echo "Deleting ${name} (${id}) [attempt ${attempt}/${MAX_RETRIES}]..."

    local http_code
    http_code=$(curl -s -o "$response_file" -w "%{http_code}" \
      -X DELETE \
      -H "$auth_header" \
      -H "Content-Type: application/json" \
      "${API_URL}/api/v1/clusters/${id}")

    if [[ "$http_code" == "200" || "$http_code" == "404" ]]; then
      echo "  OK (HTTP ${http_code})"
      return 0
    else
      log_error "Failed to delete ${name} (${id}): HTTP ${http_code}" "Cluster Deletion"
      cat "$response_file" 2>/dev/null || true
      echo ""
      if [[ "$attempt" -lt "$MAX_RETRIES" ]]; then
        echo "  Retrying in ${RETRY_DELAY}s..."
        sleep "$RETRY_DELAY"
      fi
    fi
  done

  log_error "Giving up on ${name} (${id}) after ${MAX_RETRIES} attempts" "Cluster Deletion"
  return 1
}

# fetch_cluster fetches a single cluster by ID.
# Arguments: $1=id
# Outputs the cluster JSON to stdout. Returns non-zero on failure.
fetch_cluster() {
  local id="$1"
  curl -sf -H "$auth_header" "${API_URL}/api/v1/clusters/${id}"
}

# list_active_streams prints the IDs of replication streams involving the given
# cluster that have not reached a terminal status. The cluster_id filter matches
# either side of a pair, and the API rejects an unfiltered list request.
# Arguments: $1=cluster id
list_active_streams() {
  local cluster_id="$1"
  curl -sf -H "$auth_header" \
    "${API_URL}/api/v1/physical-replication-streams?cluster_id=${cluster_id}" \
    | jq -r '.physical_replication_streams[]?
             | select(.status != "COMPLETED" and .status != "CANCELED")
             | .id'
}

# cancel_stream cancels a replication stream and waits for it to settle.
# Cancelling stops replication without promoting the standby, which failover
# would do.
# Arguments: $1=stream id
# Returns: 0 once the stream is terminal or gone, 1 if it never settles.
cancel_stream() {
  local stream_id="$1"

  echo "  Canceling replication stream ${stream_id}..."

  local http_code
  http_code=$(curl -s -o "$response_file" -w "%{http_code}" \
    -X PATCH \
    -H "$auth_header" \
    -H "Content-Type: application/json" \
    -d '{"status":"CANCELED"}' \
    "${API_URL}/api/v1/physical-replication-streams/${stream_id}")

  # A rejected cancel is still worth reporting, but the poll below decides the
  # outcome: the stream may already be terminal, which also rejects the PATCH.
  if [[ "$http_code" != "200" ]]; then
    log_warning "Cancel request for stream ${stream_id} returned HTTP ${http_code}" "Stream Cancellation"
    cat "$response_file" 2>/dev/null || true
    echo ""
  fi

  local attempt status
  for attempt in $(seq 1 "$STREAM_CANCEL_POLLS"); do
    status=$(curl -sf -H "$auth_header" \
      "${API_URL}/api/v1/physical-replication-streams/${stream_id}" \
      | jq -r '.status // empty' || echo "")
    if [[ -z "$status" ]]; then
      echo "  Stream ${stream_id} is no longer readable, continuing"
      return 0
    fi
    if [[ "$status" == "COMPLETED" || "$status" == "CANCELED" ]]; then
      echo "  Stream ${stream_id} is ${status}"
      return 0
    fi
    sleep "$STREAM_CANCEL_DELAY"
  done

  log_warning "Stream ${stream_id} is still ${status} after waiting" "Stream Cancellation"
  return 1
}

# cancel_streams_for_cluster tears down every active stream holding a cluster.
# CCAPI refuses to delete a cluster that is part of an active stream.
# Arguments: $1=cluster id
cancel_streams_for_cluster() {
  local cluster_id="$1"

  local stream_ids
  stream_ids=$(list_active_streams "$cluster_id" || echo "")
  if [[ -z "$stream_ids" ]]; then
    return 0
  fi

  local stream_id
  while read -r stream_id; do
    [[ -n "$stream_id" ]] || continue
    cancel_stream "$stream_id" || true
  done <<< "$stream_ids"
}

# Exit early if no tracking file is configured or present.
if [[ -z "$TRACKING_FILE" ]]; then
  echo "No tracking file configured, nothing to clean up."
  write_summary "### Cluster Cleanup"$'\n'"No tracking file configured. Nothing to clean up."
  exit 0
fi

if [[ ! -f "$TRACKING_FILE" ]]; then
  echo "Tracking file not found, nothing to clean up."
  write_summary "### Cluster Cleanup"$'\n'"Tracking file not found. Nothing to clean up."
  exit 0
fi

target_ids=()
while IFS=' ' read -r id _; do
  if [[ -n "$id" ]]; then
    target_ids+=("$id")
  fi
done < "$TRACKING_FILE"

if [[ ${#target_ids[@]} -eq 0 ]]; then
  echo "Tracking file is empty, nothing to clean up."
  write_summary "### Cluster Cleanup"$'\n'"Tracking file is empty. No orphaned clusters."
  exit 0
fi

log_notice "Found ${#target_ids[@]} tracked cluster(s) to check" "Cluster Cleanup"
echo ""

# Fetch details for each tracked cluster.
log_group "Fetching cluster details"
printf "%-40s %-38s %-12s %s\n" "NAME" "ID" "STATE" "CREATED_AT"
printf "%-40s %-38s %-12s %s\n" "----" "--" "-----" "----------"

clusters_to_delete=()
summary_rows=()

for id in "${target_ids[@]}"; do
  cluster_json=$(fetch_cluster "$id" 2>/dev/null || echo "")
  if [[ -z "$cluster_json" ]]; then
    log_warning "Could not fetch cluster ${id} (may already be deleted)" "Cluster Not Found"
    summary_rows+=("| (unknown) | \`${id}\` | — | — | Skipped (not found) |")
    continue
  fi

  name=$(echo "$cluster_json" | jq -r '.name')
  state=$(echo "$cluster_json" | jq -r '.state')
  created=$(echo "$cluster_json" | jq -r '.created_at')

  printf "%-40s %-38s %-12s %s\n" "$name" "$id" "$state" "$created"

  if [[ "$state" == "DELETED" ]]; then
    echo "  (already deleted, skipping)"
    summary_rows+=("| \`${name}\` | \`${id}\` | ${state} | ${created} | Already deleted |")
    continue
  fi

  clusters_to_delete+=("${id}|${name}|${state}|${created}")
done
log_endgroup

echo ""

if [[ ${#clusters_to_delete[@]} -eq 0 ]]; then
  echo "No clusters need deletion."
  write_summary "### Cluster Cleanup"$'\n'"Checked ${#target_ids[@]} tracked cluster(s). No orphans found."
  exit 0
fi

if [[ "$DRY_RUN" == "1" ]]; then
  echo "[DRY RUN] Would delete ${#clusters_to_delete[@]} cluster(s). Exiting."
  write_summary "### Cluster Cleanup (Dry Run)"$'\n'"Would delete ${#clusters_to_delete[@]} cluster(s). No action taken."
  exit 0
fi

# Delete orphaned clusters.
log_group "Deleting ${#clusters_to_delete[@]} cluster(s)"

success_count=0
fail_count=0

for entry in "${clusters_to_delete[@]}"; do
  id="${entry%%|*}"
  rest="${entry#*|}"
  name="${rest%%|*}"
  rest="${rest#*|}"
  state="${rest%%|*}"
  created="${rest#*|}"

  cancel_streams_for_cluster "$id"

  if delete_cluster "$id" "$name"; then
    success_count=$((success_count + 1))
    summary_rows+=("| \`${name}\` | \`${id}\` | ${state} | ${created} | Deleted |")
  else
    fail_count=$((fail_count + 1))
    summary_rows+=("| \`${name}\` | \`${id}\` | ${state} | ${created} | **Failed** |")
  fi
done
log_endgroup

echo ""
echo "=== Summary ==="
echo "Deleted: ${success_count}"
echo "Failed:  ${fail_count}"

# Write step summary.
{
  write_summary "### Cluster Cleanup"
  write_summary ""
  write_summary "| Name | ID | State | Created | Outcome |"
  write_summary "|------|-----|-------|---------|---------|"
  for row in "${summary_rows[@]}"; do
    write_summary "$row"
  done
  write_summary ""
  write_summary "**Deleted:** ${success_count} | **Failed:** ${fail_count}"
}

if [[ "$fail_count" -gt 0 ]]; then
  log_error "Failed to delete ${fail_count} cluster(s)" "Cluster Cleanup"
  exit 1
fi
