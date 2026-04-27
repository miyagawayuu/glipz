#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/ubuntu-common.sh
source "${SCRIPT_DIR}/lib/ubuntu-common.sh"

REPO_URL="${REPO_URL:-}"
SKIP_GIT_PULL="false"
SKIP_BACKUP="false"
ALLOW_DIRTY="false"
KEEP_IMAGES="${KEEP_IMAGES:-5}"
ASSUME_YES="false"

usage() {
  cat <<'EOF'
Usage: sudo scripts/update-ubuntu.sh [options]

Run without options for an interactive updater.

Options:
  --install-dir PATH     Install directory, default /opt/glipz
  --env-file PATH        Environment file, default /etc/glipz/glipz.env
  --repo-url URL         Clone or update this Git repository if install-dir is missing
  --skip-git-pull        Rebuild the current working tree without pulling
  --allow-dirty          Allow updating with uncommitted changes in install-dir
  --skip-backup          Skip pg_dump/media/env backup
  --keep-images N        Keep the newest N timestamped glipz images, default 5
  -y, --yes              Do not prompt for confirmation
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --install-dir) GLIPZ_INSTALL_DIR="$2"; shift 2 ;;
      --env-file) GLIPZ_ENV_FILE="$2"; shift 2 ;;
      --repo-url) REPO_URL="$2"; shift 2 ;;
      --skip-git-pull) SKIP_GIT_PULL="true"; shift ;;
      --allow-dirty) ALLOW_DIRTY="true"; shift ;;
      --skip-backup) SKIP_BACKUP="true"; shift ;;
      --keep-images) KEEP_IMAGES="$2"; shift 2 ;;
      -y|--yes) ASSUME_YES="true"; shift ;;
      -h|--help) usage; exit 0 ;;
      *) die "unknown option: $1" ;;
    esac
  done
}

interactive_update_options() {
  [[ "${ASSUME_YES}" == "true" ]] && return

  prompt_default GLIPZ_INSTALL_DIR "Install directory" "${GLIPZ_INSTALL_DIR}"
  prompt_default GLIPZ_ENV_FILE "Environment file" "${GLIPZ_ENV_FILE}"

  if confirm "Pull latest Git changes before rebuilding?" "yes"; then
    SKIP_GIT_PULL="false"
  else
    SKIP_GIT_PULL="true"
  fi

  if confirm "Create backup before updating?" "yes"; then
    SKIP_BACKUP="false"
  else
    SKIP_BACKUP="true"
  fi

  prompt_default KEEP_IMAGES "Timestamped Docker images to keep" "${KEEP_IMAGES}"

  if [[ ! -d "${GLIPZ_INSTALL_DIR}/.git" ]]; then
    prompt_optional REPO_URL "Git repository URL to clone if ${GLIPZ_INSTALL_DIR} is missing (blank: rebuild existing files)"
  fi
}

confirm_update() {
  [[ "${ASSUME_YES}" == "true" ]] && return

  cat <<EOF

Glipz will be updated with:
  Install dir:     ${GLIPZ_INSTALL_DIR}
  Environment:     ${GLIPZ_ENV_FILE}
  Git pull:        $([[ "${SKIP_GIT_PULL}" == "true" ]] && printf 'no' || printf 'yes')
  Backup:          $([[ "${SKIP_BACKUP}" == "true" ]] && printf 'no' || printf 'yes')
  Keep images:     ${KEEP_IMAGES}

The updater will build a new image, recreate the container, check /health,
and roll back to the previous image if the new container is unhealthy.
EOF

  confirm "Continue with update?" "no" || die "update cancelled"
}

prepare_source_for_update() {
  if [[ ! -d "${GLIPZ_INSTALL_DIR}/.git" && -n "${REPO_URL}" ]]; then
    log "cloning ${REPO_URL} into ${GLIPZ_INSTALL_DIR}"
    ensure_dir "$(dirname "${GLIPZ_INSTALL_DIR}")" 0755
    git clone "${REPO_URL}" "${GLIPZ_INSTALL_DIR}"
  fi

  [[ -f "${GLIPZ_INSTALL_DIR}/backend/Dockerfile" ]] || die "backend/Dockerfile not found under ${GLIPZ_INSTALL_DIR}"

  if [[ -d "${GLIPZ_INSTALL_DIR}/.git" ]]; then
    if [[ -n "$(git -C "${GLIPZ_INSTALL_DIR}" status --porcelain)" ]]; then
      if [[ "${ALLOW_DIRTY}" != "true" && "${SKIP_GIT_PULL}" != "true" ]]; then
        die "${GLIPZ_INSTALL_DIR} has uncommitted changes; commit them, pass --allow-dirty, or use --skip-git-pull"
      fi
      warn "building with uncommitted changes"
    fi

    if [[ "${SKIP_GIT_PULL}" != "true" ]]; then
      log "updating Git working tree"
      git -C "${GLIPZ_INSTALL_DIR}" fetch --prune
      git -C "${GLIPZ_INSTALL_DIR}" pull --ff-only
    else
      log "skipping git pull"
    fi
  else
    log "install directory is not a Git repository; rebuilding current files"
  fi
}

tag_previous_image() {
  if docker image inspect "${GLIPZ_IMAGE_TAG}" >/dev/null 2>&1; then
    log "tagging current image as ${GLIPZ_PREVIOUS_IMAGE_TAG}"
    docker image tag "${GLIPZ_IMAGE_TAG}" "${GLIPZ_PREVIOUS_IMAGE_TAG}"
  fi
}

deploy_new_image() {
  local stamp new_tag
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  new_tag="glipz:${stamp}"

  tag_previous_image
  build_image "${new_tag}"
  docker image tag "${new_tag}" "${GLIPZ_IMAGE_TAG}"

  log "starting updated container"
  run_glipz_container "${GLIPZ_IMAGE_TAG}"

  if wait_for_health "http://127.0.0.1:${GLIPZ_HOST_PORT}/health" 45 2; then
    log "updated container is healthy"
    cleanup_old_images
    return
  fi

  warn "updated container failed health checks"
  docker logs --tail 160 "${GLIPZ_CONTAINER_NAME}" >&2 || true
  rollback_container
  die "update failed and rollback was attempted"
}

rollback_container() {
  if ! docker image inspect "${GLIPZ_PREVIOUS_IMAGE_TAG}" >/dev/null 2>&1; then
    die "no previous image is available for rollback"
  fi

  warn "rolling back to ${GLIPZ_PREVIOUS_IMAGE_TAG}"
  docker image tag "${GLIPZ_PREVIOUS_IMAGE_TAG}" "${GLIPZ_IMAGE_TAG}"
  run_glipz_container "${GLIPZ_IMAGE_TAG}"

  if wait_for_health "http://127.0.0.1:${GLIPZ_HOST_PORT}/health" 30 2; then
    warn "rollback container is healthy"
  else
    docker logs --tail 160 "${GLIPZ_CONTAINER_NAME}" >&2 || true
    die "rollback container did not become healthy"
  fi
}

cleanup_old_images() {
  if ! [[ "${KEEP_IMAGES}" =~ ^[0-9]+$ ]]; then
    warn "KEEP_IMAGES is not numeric; skipping image cleanup"
    return
  fi

  mapfile -t old_images < <(docker images "glipz" --format '{{.Repository}}:{{.Tag}} {{.CreatedAt}}' \
    | awk '$1 ~ /^glipz:[0-9]{8}T[0-9]{6}Z$/ {print $1}' \
    | sort -r \
    | tail -n +"$((KEEP_IMAGES + 1))")

  if [[ "${#old_images[@]}" -eq 0 ]]; then
    return
  fi

  log "removing old timestamped images"
  docker image rm "${old_images[@]}" >/dev/null 2>&1 || true
}

main() {
  parse_args "$@"
  require_root
  interactive_update_options
  confirm_update
  require_cmd docker
  require_cmd git
  require_cmd curl
  require_cmd pg_dump

  [[ -f "${GLIPZ_ENV_FILE}" ]] || die "environment file not found: ${GLIPZ_ENV_FILE}"

  if [[ "${SKIP_BACKUP}" != "true" ]]; then
    backup_glipz
  else
    warn "skipping backup by request"
  fi

  prepare_source_for_update
  deploy_new_image

  log "update completed"
  log "local health: http://127.0.0.1:${GLIPZ_HOST_PORT}/health"
}

main "$@"
