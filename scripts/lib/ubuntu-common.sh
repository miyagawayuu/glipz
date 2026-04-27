#!/usr/bin/env bash

set -Eeuo pipefail

GLIPZ_APP_NAME="${GLIPZ_APP_NAME:-glipz}"
GLIPZ_CONTAINER_NAME="${GLIPZ_CONTAINER_NAME:-glipz}"
GLIPZ_ENV_FILE="${GLIPZ_ENV_FILE:-/etc/glipz/glipz.env}"
GLIPZ_INSTALL_DIR="${GLIPZ_INSTALL_DIR:-/opt/glipz}"
GLIPZ_MEDIA_DIR="${GLIPZ_MEDIA_DIR:-/var/lib/glipz/media}"
GLIPZ_LEGAL_DOCS_DIR="${GLIPZ_LEGAL_DOCS_DIR:-/var/lib/glipz/legal-docs}"
GLIPZ_BACKUP_DIR="${GLIPZ_BACKUP_DIR:-/var/backups/glipz}"
GLIPZ_HOST_PORT="${GLIPZ_HOST_PORT:-8080}"
GLIPZ_IMAGE_TAG="${GLIPZ_IMAGE_TAG:-glipz:latest}"
GLIPZ_PREVIOUS_IMAGE_TAG="${GLIPZ_PREVIOUS_IMAGE_TAG:-glipz:previous}"

log() {
  printf '[glipz] %s\n' "$*"
}

warn() {
  printf '[glipz] WARN: %s\n' "$*" >&2
}

die() {
  printf '[glipz] ERROR: %s\n' "$*" >&2
  exit 1
}

require_root() {
  if [[ "${EUID}" -ne 0 ]]; then
    die "run this script as root, for example: sudo $0"
  fi
}

require_cmd() {
  local cmd="$1"
  command -v "${cmd}" >/dev/null 2>&1 || die "required command not found: ${cmd}"
}

ensure_dir() {
  local path="$1"
  local mode="${2:-0755}"
  install -d -m "${mode}" "${path}"
}

random_secret() {
  openssl rand -base64 48 | tr -d '\n'
}

random_password() {
  openssl rand -base64 32 | tr -d '\n=' | tr '/+' '_-'
}

prompt_required() {
  local var_name="$1"
  local prompt="$2"
  local value="${!var_name:-}"

  while [[ -z "${value}" ]]; do
    read -r -p "${prompt}: " value
  done

  printf -v "${var_name}" '%s' "${value}"
}

prompt_default() {
  local var_name="$1"
  local prompt="$2"
  local default_value="$3"
  local value="${!var_name:-}"

  if [[ -z "${value}" ]]; then
    read -r -p "${prompt} [${default_value}]: " value
    value="${value:-${default_value}}"
  fi

  printf -v "${var_name}" '%s' "${value}"
}

prompt_optional() {
  local var_name="$1"
  local prompt="$2"
  local value="${!var_name:-}"

  if [[ -z "${value}" ]]; then
    read -r -p "${prompt}: " value
  fi

  printf -v "${var_name}" '%s' "${value}"
}

choose_option() {
  local var_name="$1"
  local prompt="$2"
  local default_value="$3"
  shift 3

  local current="${!var_name:-}"
  local option choice

  if [[ -n "${current}" ]]; then
    printf -v "${var_name}" '%s' "${current}"
    return
  fi

  printf '%s\n' "${prompt}"
  for option in "$@"; do
    printf '  %s\n' "${option}"
  done

  while true; do
    read -r -p "Select [${default_value}]: " choice
    choice="${choice:-${default_value}}"

    for option in "$@"; do
      if [[ "${choice}" == "${option%%)*}" || "${choice}" == "${option#*) }" ]]; then
        printf -v "${var_name}" '%s' "${option#*) }"
        return
      fi
    done

    warn "invalid selection: ${choice}"
  done
}

confirm() {
  local prompt="$1"
  local default="${2:-no}"
  local suffix="[y/N]"
  local answer

  if [[ "${default}" == "yes" ]]; then
    suffix="[Y/n]"
  fi

  read -r -p "${prompt} ${suffix}: " answer
  answer="${answer:-${default}}"
  [[ "${answer}" =~ ^[Yy](es)?$ ]]
}

write_env_file() {
  local tmp
  tmp="$(mktemp)"
  umask 077
  cat >"${tmp}"
  install -d -m 0750 "$(dirname "${GLIPZ_ENV_FILE}")"
  install -m 0600 "${tmp}" "${GLIPZ_ENV_FILE}"
  rm -f "${tmp}"
}

load_env_file() {
  [[ -f "${GLIPZ_ENV_FILE}" ]] || die "environment file not found: ${GLIPZ_ENV_FILE}"
  set -a
  # shellcheck disable=SC1090
  source "${GLIPZ_ENV_FILE}"
  set +a
}

env_value() {
  local key="$1"
  local value
  value="$(grep -E "^${key}=" "${GLIPZ_ENV_FILE}" | tail -n 1 | cut -d= -f2- || true)"
  printf '%s' "${value}"
}

docker_gateway_ip() {
  local gateway
  gateway="$(ip -4 addr show docker0 2>/dev/null | awk '/inet / {print $2}' | cut -d/ -f1 | head -n 1 || true)"
  printf '%s' "${gateway:-172.17.0.1}"
}

docker_bridge_cidr() {
  local cidr
  cidr="$(docker network inspect bridge --format '{{(index .IPAM.Config 0).Subnet}}' 2>/dev/null || true)"
  if [[ -z "${cidr}" ]]; then
    cidr="$(ip -4 addr show docker0 2>/dev/null | awk '/inet / {print $2}' | head -n 1 || true)"
  fi
  printf '%s' "${cidr:-172.17.0.0/16}"
}

current_git_ref() {
  if git -C "${GLIPZ_INSTALL_DIR}" rev-parse --short HEAD >/dev/null 2>&1; then
    git -C "${GLIPZ_INSTALL_DIR}" rev-parse --short HEAD
  else
    date -u +%Y%m%d%H%M%S
  fi
}

build_image() {
  local tag="$1"
  require_cmd docker
  [[ -f "${GLIPZ_INSTALL_DIR}/backend/Dockerfile" ]] || die "backend/Dockerfile not found under ${GLIPZ_INSTALL_DIR}"
  log "building Docker image ${tag}"
  docker build -f "${GLIPZ_INSTALL_DIR}/backend/Dockerfile" -t "${tag}" "${GLIPZ_INSTALL_DIR}"
}

remove_container_if_exists() {
  local name="$1"
  if docker ps -a --format '{{.Names}}' | grep -Fxq "${name}"; then
    docker rm -f "${name}" >/dev/null
  fi
}

run_glipz_container() {
  local image="$1"
  local name="${2:-${GLIPZ_CONTAINER_NAME}}"
  local storage_mode

  load_env_file
  storage_mode="${GLIPZ_STORAGE_MODE:-local}"

  remove_container_if_exists "${name}"

  local args=(
    run
    -d
    --name "${name}"
    --restart unless-stopped
    --add-host=host.docker.internal:host-gateway
    --env-file "${GLIPZ_ENV_FILE}"
    -v "${GLIPZ_LEGAL_DOCS_DIR}:/app/data/legal-docs:ro"
    -p "127.0.0.1:${GLIPZ_HOST_PORT}:8080"
  )

  if [[ "${storage_mode}" == "local" ]]; then
    args+=(-v "${GLIPZ_MEDIA_DIR}:/app/data/media")
  fi

  args+=("${image}")
  docker "${args[@]}" >/dev/null
}

wait_for_health() {
  local url="${1:-http://127.0.0.1:${GLIPZ_HOST_PORT}/health}"
  local attempts="${2:-30}"
  local delay="${3:-2}"
  local i

  for ((i = 1; i <= attempts; i++)); do
    if curl -fsS "${url}" 2>/dev/null | grep -Fxq "ok"; then
      return 0
    fi
    sleep "${delay}"
  done

  return 1
}

backup_glipz() {
  local stamp backup_path db_url db_name
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  backup_path="${GLIPZ_BACKUP_DIR}/${stamp}"

  ensure_dir "${backup_path}" 0700
  load_env_file

  if [[ -f "${GLIPZ_ENV_FILE}" ]]; then
    cp -a "${GLIPZ_ENV_FILE}" "${backup_path}/glipz.env"
  fi

  db_url="${DATABASE_URL:-}"
  if [[ -n "${db_url}" ]]; then
    log "backing up PostgreSQL database"
    pg_dump "${db_url}" | gzip -9 >"${backup_path}/postgres.sql.gz"
  else
    warn "DATABASE_URL is empty; skipping PostgreSQL backup"
  fi

  if [[ "${GLIPZ_STORAGE_MODE:-local}" == "local" && -d "${GLIPZ_MEDIA_DIR}" ]]; then
    log "backing up local media"
    tar -C "$(dirname "${GLIPZ_MEDIA_DIR}")" -czf "${backup_path}/media.tar.gz" "$(basename "${GLIPZ_MEDIA_DIR}")"
  fi

  if [[ -d "${GLIPZ_LEGAL_DOCS_DIR}" ]]; then
    tar -C "$(dirname "${GLIPZ_LEGAL_DOCS_DIR}")" -czf "${backup_path}/legal-docs.tar.gz" "$(basename "${GLIPZ_LEGAL_DOCS_DIR}")"
  fi

  db_name="${POSTGRES_DB:-glipz}"
  log "backup completed at ${backup_path} for ${db_name}"
}
