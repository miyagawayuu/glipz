#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/ubuntu-common.sh
source "${SCRIPT_DIR}/lib/ubuntu-common.sh"

REMOVE_IMAGES="false"
REMOVE_INSTALL_DIR="false"
REMOVE_ENV="false"
REMOVE_DATA="false"
REMOVE_BACKUPS="false"
ASSUME_YES="false"

usage() {
  cat <<'EOF'
Usage: sudo bash scripts/uninstall-ubuntu.sh [options]

Stops and removes the Glipz Docker container. Data, configuration, images,
source checkout, and backups are preserved unless explicitly requested.

Options:
  --install-dir PATH     Install directory, default /opt/glipz
  --env-file PATH        Environment file, default /etc/glipz/glipz.env
  --container NAME       Container name, default glipz
  --remove-images        Remove local glipz Docker images
  --remove-install-dir   Remove the install/source directory
  --remove-env           Remove the environment file
  --remove-data          Remove local media and legal-docs directories
  --remove-backups       Remove backup directory
  --purge                Remove images, install dir, env file, data, and backups
  -y, --yes              Do not prompt for confirmation
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --install-dir) GLIPZ_INSTALL_DIR="$2"; shift 2 ;;
      --env-file) GLIPZ_ENV_FILE="$2"; shift 2 ;;
      --container) GLIPZ_CONTAINER_NAME="$2"; shift 2 ;;
      --remove-images) REMOVE_IMAGES="true"; shift ;;
      --remove-install-dir) REMOVE_INSTALL_DIR="true"; shift ;;
      --remove-env) REMOVE_ENV="true"; shift ;;
      --remove-data) REMOVE_DATA="true"; shift ;;
      --remove-backups) REMOVE_BACKUPS="true"; shift ;;
      --purge)
        REMOVE_IMAGES="true"
        REMOVE_INSTALL_DIR="true"
        REMOVE_ENV="true"
        REMOVE_DATA="true"
        REMOVE_BACKUPS="true"
        shift
        ;;
      -y|--yes) ASSUME_YES="true"; shift ;;
      -h|--help) usage; exit 0 ;;
      *) die "unknown option: $1" ;;
    esac
  done
}

print_plan() {
  cat <<EOF

Glipz will be uninstalled with:
  Container:       ${GLIPZ_CONTAINER_NAME}
  Install dir:     ${GLIPZ_INSTALL_DIR} ($([[ "${REMOVE_INSTALL_DIR}" == "true" ]] && printf 'remove' || printf 'keep'))
  Environment:     ${GLIPZ_ENV_FILE} ($([[ "${REMOVE_ENV}" == "true" ]] && printf 'remove' || printf 'keep'))
  Media dir:       ${GLIPZ_MEDIA_DIR} ($([[ "${REMOVE_DATA}" == "true" ]] && printf 'remove' || printf 'keep'))
  Legal docs dir:  ${GLIPZ_LEGAL_DOCS_DIR} ($([[ "${REMOVE_DATA}" == "true" ]] && printf 'remove' || printf 'keep'))
  Backup dir:      ${GLIPZ_BACKUP_DIR} ($([[ "${REMOVE_BACKUPS}" == "true" ]] && printf 'remove' || printf 'keep'))
  Docker images:   $([[ "${REMOVE_IMAGES}" == "true" ]] && printf 'remove glipz:*' || printf 'keep')
EOF
}

confirm_uninstall() {
  print_plan
  [[ "${ASSUME_YES}" == "true" ]] && return

  confirm "Continue with uninstall?" "no" || die "uninstall cancelled"

  if [[ "${REMOVE_DATA}" == "true" || "${REMOVE_ENV}" == "true" || "${REMOVE_BACKUPS}" == "true" ]]; then
    warn "destructive file removal was requested"
    confirm "Permanently remove the selected files and directories?" "no" || die "uninstall cancelled"
  fi
}

stop_and_remove_container() {
  require_cmd docker

  if docker ps -a --format '{{.Names}}' | grep -Fxq "${GLIPZ_CONTAINER_NAME}"; then
    log "removing container ${GLIPZ_CONTAINER_NAME}"
    docker rm -f "${GLIPZ_CONTAINER_NAME}" >/dev/null
  else
    log "container ${GLIPZ_CONTAINER_NAME} not found"
  fi
}

remove_glipz_images() {
  [[ "${REMOVE_IMAGES}" == "true" ]] || return

  mapfile -t images < <(docker images "glipz" --format '{{.Repository}}:{{.Tag}}' | sort -u)
  if [[ "${#images[@]}" -eq 0 ]]; then
    log "no glipz Docker images found"
    return
  fi

  log "removing Docker images: ${images[*]}"
  docker image rm -f "${images[@]}" >/dev/null 2>&1 || true
}

remove_path_if_requested() {
  local enabled="$1"
  local path="$2"
  local label="$3"

  [[ "${enabled}" == "true" ]] || return
  if [[ -e "${path}" ]]; then
    log "removing ${label}: ${path}"
    rm -rf -- "${path}"
  else
    log "${label} not found: ${path}"
  fi
}

remove_empty_parent_dir() {
  local path="$1"
  local parent

  parent="$(dirname "${path}")"
  if [[ -d "${parent}" ]]; then
    rmdir --ignore-fail-on-non-empty "${parent}" 2>/dev/null || true
  fi
}

main() {
  parse_args "$@"
  require_root
  confirm_uninstall

  stop_and_remove_container
  remove_glipz_images

  remove_path_if_requested "${REMOVE_INSTALL_DIR}" "${GLIPZ_INSTALL_DIR}" "install directory"
  remove_path_if_requested "${REMOVE_ENV}" "${GLIPZ_ENV_FILE}" "environment file"
  remove_path_if_requested "${REMOVE_DATA}" "${GLIPZ_MEDIA_DIR}" "media directory"
  remove_path_if_requested "${REMOVE_DATA}" "${GLIPZ_LEGAL_DOCS_DIR}" "legal docs directory"
  remove_path_if_requested "${REMOVE_BACKUPS}" "${GLIPZ_BACKUP_DIR}" "backup directory"

  if [[ "${REMOVE_ENV}" == "true" ]]; then
    remove_empty_parent_dir "${GLIPZ_ENV_FILE}"
  fi
  if [[ "${REMOVE_DATA}" == "true" ]]; then
    remove_empty_parent_dir "${GLIPZ_MEDIA_DIR}"
    remove_empty_parent_dir "${GLIPZ_LEGAL_DOCS_DIR}"
  fi

  log "uninstall completed"
}

main "$@"
