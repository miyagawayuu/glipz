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

ensure_host_docker_access() {
  local gateway cidr db_url pg_user pg_db pg_password hba_file redis_conf
  gateway="$(docker_gateway_ip)"
  cidr="$(docker_bridge_cidr)"

  if ! grep -Eq '(^|[[:space:]])host\.docker\.internal([[:space:]]|$)' /etc/hosts; then
    log "adding host.docker.internal host alias for local maintenance commands"
    printf '127.0.0.1 host.docker.internal\n' >>/etc/hosts
  fi

  db_url="${DATABASE_URL:-}"
  pg_user="${POSTGRES_USER:-glipz}"
  pg_db="${POSTGRES_DB:-glipz}"
  pg_password="${POSTGRES_PASSWORD:-}"
  if [[ -z "${pg_password}" && "${db_url}" =~ ^postgres://([^:]+):([^@]+)@ ]]; then
    pg_user="${BASH_REMATCH[1]}"
    pg_password="${BASH_REMATCH[2]}"
  fi
  if [[ -z "${pg_db}" && "${db_url}" =~ /([^/?]+) ]]; then
    pg_db="${BASH_REMATCH[1]}"
  fi

  if command -v psql >/dev/null 2>&1 && systemctl list-unit-files postgresql.service >/dev/null 2>&1; then
    log "ensuring PostgreSQL listens on Docker bridge ${gateway}"
    sudo -u postgres psql -v ON_ERROR_STOP=1 <<SQL
ALTER SYSTEM SET listen_addresses TO 'localhost,${gateway}';
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '${pg_user}') THEN
    CREATE ROLE ${pg_user} LOGIN PASSWORD '${pg_password}';
  ELSIF '${pg_password}' <> '' THEN
    ALTER ROLE ${pg_user} WITH LOGIN PASSWORD '${pg_password}';
  END IF;
END
\$\$;
SELECT 'CREATE DATABASE ${pg_db} OWNER ${pg_user}'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = '${pg_db}')\\gexec
GRANT ALL PRIVILEGES ON DATABASE ${pg_db} TO ${pg_user};
SQL
    hba_file="$(sudo -u postgres psql -tAc "SHOW hba_file;" | xargs)"
    if ! grep -Eq "^[[:space:]]*host[[:space:]]+${pg_db}[[:space:]]+${pg_user}[[:space:]]+${cidr//./\\.}[[:space:]]+" "${hba_file}"; then
      cat >>"${hba_file}" <<EOF
# Allow the Glipz Docker container to reach the local PostgreSQL server.
host    ${pg_db}    ${pg_user}    ${cidr}    scram-sha-256
EOF
    fi
    systemctl restart postgresql
  fi

  redis_conf="/etc/redis/redis.conf"
  if [[ -f "${redis_conf}" ]]; then
    log "ensuring Redis listens on Docker bridge ${gateway}"
    sed -i -E "s/^bind .*/bind 127.0.0.1 ::1 ${gateway}/" "${redis_conf}"
    sed -i -E "s/^protected-mode .*/protected-mode yes/" "${redis_conf}"
    if [[ -n "${REDIS_PASSWORD:-}" ]]; then
      if grep -Eq '^[#[:space:]]*requirepass ' "${redis_conf}"; then
        sed -i -E "s|^[#[:space:]]*requirepass .*|requirepass ${REDIS_PASSWORD}|" "${redis_conf}"
      else
        printf '\nrequirepass %s\n' "${REDIS_PASSWORD}" >>"${redis_conf}"
      fi
    fi
    systemctl restart redis-server
  fi

  if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q "Status: active"; then
    log "allowing Docker bridge access to local PostgreSQL and Redis through UFW"
    ufw allow in on docker0 from "${cidr}" to "${gateway}" port 5432 proto tcp >/dev/null || true
    ufw allow in on docker0 from "${cidr}" to "${gateway}" port 6379 proto tcp >/dev/null || true
  fi
}

initialize_database_schema() {
  local init_sql="${GLIPZ_INSTALL_DIR}/infra/postgres/init.sql"
  local has_users

  [[ -f "${init_sql}" ]] || die "database init SQL not found: ${init_sql}"
  has_users="$(sudo -u postgres psql -d "${POSTGRES_DB:-glipz}" -tAc "SELECT to_regclass('public.users') IS NOT NULL")"
  if [[ "${has_users}" == "t" ]]; then
    log "ensuring PostgreSQL schema ownership for ${POSTGRES_USER:-glipz}"
  else
    log "initializing PostgreSQL schema from ${init_sql}"
    sudo -u postgres psql -v ON_ERROR_STOP=1 -d "${POSTGRES_DB:-glipz}" -f "${init_sql}"
  fi

  sudo -u postgres psql -v ON_ERROR_STOP=1 -d "${POSTGRES_DB:-glipz}" <<SQL
ALTER DATABASE ${POSTGRES_DB:-glipz} OWNER TO ${POSTGRES_USER:-glipz};
ALTER SCHEMA public OWNER TO ${POSTGRES_USER:-glipz};
GRANT USAGE, CREATE ON SCHEMA public TO ${POSTGRES_USER:-glipz};
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO ${POSTGRES_USER:-glipz};
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO ${POSTGRES_USER:-glipz};
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO ${POSTGRES_USER:-glipz};
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO ${POSTGRES_USER:-glipz};
DO \$\$
DECLARE
  r RECORD;
BEGIN
  FOR r IN
    SELECT schemaname, tablename FROM pg_tables WHERE schemaname = 'public'
  LOOP
    EXECUTE format('ALTER TABLE %I.%I OWNER TO %I', r.schemaname, r.tablename, '${POSTGRES_USER:-glipz}');
  END LOOP;
  FOR r IN
    SELECT schemaname, sequencename FROM pg_sequences WHERE schemaname = 'public'
  LOOP
    EXECUTE format('ALTER SEQUENCE %I.%I OWNER TO %I', r.schemaname, r.sequencename, '${POSTGRES_USER:-glipz}');
  END LOOP;
END
\$\$;
SQL
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

print_container_diagnostics() {
  local name="${1:-${GLIPZ_CONTAINER_NAME}}"
  local port="${2:-${GLIPZ_HOST_PORT}}"

  warn "container diagnostics for ${name}"
  docker ps -a --filter "name=^/${name}$" || true
  docker inspect --format 'status={{.State.Status}} exit={{.State.ExitCode}} error={{.State.Error}} started={{.State.StartedAt}} finished={{.State.FinishedAt}}' "${name}" 2>/dev/null || true

  warn "recent container logs"
  docker logs --tail 200 "${name}" >&2 || true

  warn "host listeners"
  ss -ltnp 2>/dev/null | grep -E "(:${port}|:5432|:6379)[[:space:]]" >&2 || true

  warn "local health probe"
  curl -v --max-time 5 "http://127.0.0.1:${port}/health" >&2 || true
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
  db_url="${db_url//@host.docker.internal:/@127.0.0.1:}"
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
