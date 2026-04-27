#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/ubuntu-common.sh
source "${SCRIPT_DIR}/lib/ubuntu-common.sh"

DOMAIN="${DOMAIN:-}"
LETSENCRYPT_EMAIL="${LETSENCRYPT_EMAIL:-}"
PROXY="${PROXY:-}"
STORAGE_MODE="${STORAGE_MODE:-}"
DEFAULT_REPO_URL="${DEFAULT_REPO_URL:-https://github.com/glipz-project/glipz}"
REPO_URL="${REPO_URL:-${DEFAULT_REPO_URL}}"
POSTGRES_USER="${POSTGRES_USER:-glipz}"
POSTGRES_DB="${POSTGRES_DB:-glipz}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-}"
REDIS_PASSWORD="${REDIS_PASSWORD:-}"
FORCE_ENV="false"
NON_INTERACTIVE="false"

S3_ENDPOINT="${S3_ENDPOINT:-}"
S3_PUBLIC_ENDPOINT="${S3_PUBLIC_ENDPOINT:-}"
S3_REGION="${S3_REGION:-}"
S3_ACCESS_KEY="${S3_ACCESS_KEY:-}"
S3_SECRET_KEY="${S3_SECRET_KEY:-}"
S3_BUCKET="${S3_BUCKET:-}"
S3_USE_PATH_STYLE="${S3_USE_PATH_STYLE:-false}"

usage() {
  cat <<'EOF'
Usage: sudo scripts/install-ubuntu.sh [options]

Run without options for an interactive installer.

Options:
  --domain DOMAIN              Public domain, for example example.com
  --email EMAIL                Let's Encrypt account email
  --proxy nginx|caddy|none     Reverse proxy to configure
  --storage local|s3           Media storage mode
  --repo-url URL               Repository to clone, default https://github.com/glipz-project/glipz
  --install-dir PATH           Install directory, default /opt/glipz
  --env-file PATH              Environment file, default /etc/glipz/glipz.env
  --force-env                  Overwrite an existing environment file
  --non-interactive            Fail instead of prompting for missing required values

S3 options when --storage s3:
  --s3-endpoint URL
  --s3-public-endpoint URL
  --s3-region REGION
  --s3-access-key KEY
  --s3-secret-key SECRET
  --s3-bucket BUCKET
  --s3-use-path-style true|false
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --domain) DOMAIN="$2"; shift 2 ;;
      --email) LETSENCRYPT_EMAIL="$2"; shift 2 ;;
      --proxy) PROXY="$2"; shift 2 ;;
      --storage) STORAGE_MODE="$2"; shift 2 ;;
      --repo-url) REPO_URL="$2"; shift 2 ;;
      --install-dir) GLIPZ_INSTALL_DIR="$2"; shift 2 ;;
      --env-file) GLIPZ_ENV_FILE="$2"; shift 2 ;;
      --force-env) FORCE_ENV="true"; shift ;;
      --non-interactive) NON_INTERACTIVE="true"; shift ;;
      --s3-endpoint) S3_ENDPOINT="$2"; shift 2 ;;
      --s3-public-endpoint) S3_PUBLIC_ENDPOINT="$2"; shift 2 ;;
      --s3-region) S3_REGION="$2"; shift 2 ;;
      --s3-access-key) S3_ACCESS_KEY="$2"; shift 2 ;;
      --s3-secret-key) S3_SECRET_KEY="$2"; shift 2 ;;
      --s3-bucket) S3_BUCKET="$2"; shift 2 ;;
      --s3-use-path-style) S3_USE_PATH_STYLE="$2"; shift 2 ;;
      -h|--help) usage; exit 0 ;;
      *) die "unknown option: $1" ;;
    esac
  done
}

prompt_or_die() {
  local var_name="$1"
  local message="$2"
  if [[ "${NON_INTERACTIVE}" == "true" ]]; then
    [[ -n "${!var_name:-}" ]] || die "missing required option: ${message}"
  else
    prompt_required "${var_name}" "${message}"
  fi
}

default_or_die() {
  local var_name="$1"
  local message="$2"
  local default_value="$3"
  if [[ "${NON_INTERACTIVE}" == "true" ]]; then
    printf -v "${var_name}" '%s' "${!var_name:-${default_value}}"
  else
    prompt_default "${var_name}" "${message}" "${default_value}"
  fi
}

validate_inputs() {
  prompt_or_die DOMAIN "Public domain"
  if [[ "${NON_INTERACTIVE}" == "true" ]]; then
    default_or_die PROXY "Reverse proxy (nginx/caddy/none)" "nginx"
    default_or_die STORAGE_MODE "Media storage mode (local/s3)" "local"
  else
    choose_option PROXY "Choose reverse proxy" "1" "1) nginx" "2) caddy" "3) none"
    choose_option STORAGE_MODE "Choose media storage" "1" "1) local" "2) s3"
    prompt_default REPO_URL "Git repository URL to clone into ${GLIPZ_INSTALL_DIR}" "${DEFAULT_REPO_URL}"
  fi

  case "${PROXY}" in
    nginx|caddy|none) ;;
    *) die "--proxy must be nginx, caddy, or none" ;;
  esac

  case "${STORAGE_MODE}" in
    local|s3) ;;
    *) die "--storage must be local or s3" ;;
  esac

  if [[ "${PROXY}" != "none" ]]; then
    prompt_or_die LETSENCRYPT_EMAIL "Let's Encrypt email"
  fi

  if [[ "${NON_INTERACTIVE}" != "true" ]]; then
    prompt_default GLIPZ_INSTALL_DIR "Install directory" "${GLIPZ_INSTALL_DIR}"
    prompt_default GLIPZ_ENV_FILE "Environment file" "${GLIPZ_ENV_FILE}"
    prompt_default POSTGRES_USER "PostgreSQL user" "${POSTGRES_USER}"
    prompt_default POSTGRES_DB "PostgreSQL database" "${POSTGRES_DB}"
    if confirm "Use generated PostgreSQL and Redis passwords?" "yes"; then
      POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-$(random_password)}"
      REDIS_PASSWORD="${REDIS_PASSWORD:-$(random_password)}"
    else
      prompt_required POSTGRES_PASSWORD "PostgreSQL password"
      prompt_required REDIS_PASSWORD "Redis password"
    fi
  fi

  if [[ ! "${POSTGRES_USER}" =~ ^[a-z_][a-z0-9_]*$ ]]; then
    die "POSTGRES_USER must be a PostgreSQL identifier using lowercase letters, numbers, and underscores"
  fi
  if [[ ! "${POSTGRES_DB}" =~ ^[a-z_][a-z0-9_]*$ ]]; then
    die "POSTGRES_DB must be a PostgreSQL identifier using lowercase letters, numbers, and underscores"
  fi

  if [[ "${STORAGE_MODE}" == "s3" ]]; then
    prompt_or_die S3_ENDPOINT "S3 endpoint"
    prompt_or_die S3_PUBLIC_ENDPOINT "S3 public endpoint"
    prompt_or_die S3_REGION "S3 region"
    prompt_or_die S3_ACCESS_KEY "S3 access key"
    prompt_or_die S3_SECRET_KEY "S3 secret key"
    prompt_or_die S3_BUCKET "S3 bucket"
  fi
}

confirm_install() {
  [[ "${NON_INTERACTIVE}" == "true" ]] && return

  cat <<EOF

Glipz will be installed with:
  Domain:          ${DOMAIN}
  Reverse proxy:   ${PROXY}
  Storage:         ${STORAGE_MODE}
  Install dir:     ${GLIPZ_INSTALL_DIR}
  Environment:     ${GLIPZ_ENV_FILE}
  PostgreSQL DB:   ${POSTGRES_DB}
  PostgreSQL user: ${POSTGRES_USER}
  Repository:      ${REPO_URL:-current directory}

This will install packages, configure PostgreSQL/Redis, build a Docker image,
and start the Glipz container.
EOF

  confirm "Continue with installation?" "no" || die "installation cancelled"
}

check_existing_env() {
  if [[ ! -f "${GLIPZ_ENV_FILE}" || "${FORCE_ENV}" == "true" ]]; then
    return
  fi

  if [[ "${NON_INTERACTIVE}" == "true" ]]; then
    die "${GLIPZ_ENV_FILE} already exists; pass --force-env to overwrite it"
  fi

  if ! confirm "${GLIPZ_ENV_FILE} already exists. Overwrite it?" "no"; then
    die "stopping to avoid mismatching existing secrets; use update-ubuntu.sh for installed systems"
  fi
}

install_base_packages() {
  export DEBIAN_FRONTEND=noninteractive
  apt-get update
  apt-get install -y ca-certificates curl gnupg lsb-release openssl git rsync iproute2 tar gzip sudo
}

check_ubuntu() {
  [[ -f /etc/os-release ]] || die "/etc/os-release not found"
  # shellcheck disable=SC1091
  source /etc/os-release
  [[ "${ID:-}" == "ubuntu" ]] || die "this installer is intended for Ubuntu Server"

  case "${VERSION_ID:-}" in
    22.04|24.04) ;;
    *) warn "Ubuntu ${VERSION_ID:-unknown} is not explicitly tested; continuing" ;;
  esac
}

install_docker() {
  if command -v docker >/dev/null 2>&1; then
    log "Docker is already installed"
    systemctl enable --now docker
    return
  fi

  log "installing Docker from the official apt repository"
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL "https://download.docker.com/linux/ubuntu/gpg" | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
  chmod a+r /etc/apt/keyrings/docker.gpg

  local codename arch
  codename="$(. /etc/os-release && printf '%s' "${VERSION_CODENAME}")"
  arch="$(dpkg --print-architecture)"

  cat >/etc/apt/sources.list.d/docker.list <<EOF
deb [arch=${arch} signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu ${codename} stable
EOF

  apt-get update
  apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
  systemctl enable --now docker
}

install_postgres_redis() {
  log "installing PostgreSQL 16 and Redis"
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL "https://www.postgresql.org/media/keys/ACCC4CF8.asc" | gpg --dearmor -o /etc/apt/keyrings/postgresql.gpg
  chmod a+r /etc/apt/keyrings/postgresql.gpg

  local codename
  codename="$(. /etc/os-release && printf '%s' "${VERSION_CODENAME}")"

  cat >/etc/apt/sources.list.d/pgdg.list <<EOF
deb [signed-by=/etc/apt/keyrings/postgresql.gpg] https://apt.postgresql.org/pub/repos/apt ${codename}-pgdg main
EOF

  apt-get update
  apt-get install -y postgresql-16 postgresql-client-16 redis-server
  systemctl enable --now postgresql redis-server
}

configure_postgres() {
  log "configuring PostgreSQL for Glipz"
  local gateway cidr escaped_password
  gateway="$(docker_gateway_ip)"
  cidr="$(docker_bridge_cidr)"
  escaped_password="${POSTGRES_PASSWORD//\'/\'\'}"

  sudo -u postgres psql -v ON_ERROR_STOP=1 <<SQL
ALTER SYSTEM SET listen_addresses TO 'localhost,${gateway}';
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '${POSTGRES_USER}') THEN
    CREATE ROLE ${POSTGRES_USER} LOGIN PASSWORD '${escaped_password}';
  ELSE
    ALTER ROLE ${POSTGRES_USER} WITH LOGIN PASSWORD '${escaped_password}';
  END IF;
END
\$\$;
SELECT 'CREATE DATABASE ${POSTGRES_DB} OWNER ${POSTGRES_USER}'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = '${POSTGRES_DB}')\\gexec
GRANT ALL PRIVILEGES ON DATABASE ${POSTGRES_DB} TO ${POSTGRES_USER};
SQL

  local hba_file
  hba_file="$(sudo -u postgres psql -tAc "SHOW hba_file;" | xargs)"
  if ! grep -Eq "^[[:space:]]*host[[:space:]]+${POSTGRES_DB}[[:space:]]+${POSTGRES_USER}[[:space:]]+${cidr//./\\.}[[:space:]]+" "${hba_file}"; then
    cat >>"${hba_file}" <<EOF
# Allow the Glipz Docker container to reach the local PostgreSQL server.
host    ${POSTGRES_DB}    ${POSTGRES_USER}    ${cidr}    scram-sha-256
EOF
  fi

  systemctl restart postgresql
}

configure_redis() {
  log "configuring Redis for Glipz"
  local gateway redis_conf
  gateway="$(docker_gateway_ip)"
  redis_conf="/etc/redis/redis.conf"

  cp -a "${redis_conf}" "${redis_conf}.glipz.bak.$(date -u +%Y%m%d%H%M%S)"
  sed -i -E "s/^bind .*/bind 127.0.0.1 ::1 ${gateway}/" "${redis_conf}"
  sed -i -E "s/^protected-mode .*/protected-mode yes/" "${redis_conf}"

  if grep -Eq '^[#[:space:]]*requirepass ' "${redis_conf}"; then
    sed -i -E "s|^[#[:space:]]*requirepass .*|requirepass ${REDIS_PASSWORD}|" "${redis_conf}"
  else
    printf '\nrequirepass %s\n' "${REDIS_PASSWORD}" >>"${redis_conf}"
  fi

  systemctl restart redis-server
}

prepare_source() {
  log "preparing application source in ${GLIPZ_INSTALL_DIR}"
  ensure_dir "${GLIPZ_INSTALL_DIR}" 0755

  if [[ -n "${REPO_URL}" ]]; then
    if [[ -d "${GLIPZ_INSTALL_DIR}/.git" ]]; then
      git -C "${GLIPZ_INSTALL_DIR}" fetch --prune
      git -C "${GLIPZ_INSTALL_DIR}" pull --ff-only
    else
      rm -rf "${GLIPZ_INSTALL_DIR:?}/"*
      git clone "${REPO_URL}" "${GLIPZ_INSTALL_DIR}"
    fi
    return
  fi

  if [[ -f "${PWD}/backend/Dockerfile" && "${PWD}" != "${GLIPZ_INSTALL_DIR}" ]]; then
    rsync -a --delete \
      --exclude ".git/" \
      --exclude ".jj/" \
      --exclude "node_modules/" \
      --exclude "web/node_modules/" \
      --exclude "backend/tmp/" \
      "${PWD}/" "${GLIPZ_INSTALL_DIR}/"
  elif [[ ! -f "${GLIPZ_INSTALL_DIR}/backend/Dockerfile" ]]; then
    die "no source found; run from the repository root or pass --repo-url"
  fi
}

prepare_data_dirs() {
  log "preparing persistent directories"
  ensure_dir "$(dirname "${GLIPZ_MEDIA_DIR}")" 0755
  ensure_dir "${GLIPZ_MEDIA_DIR}" 0750
  ensure_dir "${GLIPZ_LEGAL_DOCS_DIR}" 0755
  chown -R 10001:10001 "${GLIPZ_MEDIA_DIR}" "${GLIPZ_LEGAL_DOCS_DIR}"

  if [[ -d "${GLIPZ_INSTALL_DIR}/legal-docs.example" && -z "$(ls -A "${GLIPZ_LEGAL_DOCS_DIR}" 2>/dev/null)" ]]; then
    cp -a "${GLIPZ_INSTALL_DIR}/legal-docs.example/." "${GLIPZ_LEGAL_DOCS_DIR}/"
    chown -R 10001:10001 "${GLIPZ_LEGAL_DOCS_DIR}"
  fi
}

write_glipz_env() {
  POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-$(random_password)}"
  REDIS_PASSWORD="${REDIS_PASSWORD:-$(random_password)}"

  local jwt_secret db_url redis_url frontend_origin public_origin media_public_base
  jwt_secret="$(random_secret)"
  db_url="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@host.docker.internal:5432/${POSTGRES_DB}?sslmode=disable"
  redis_url="redis://:${REDIS_PASSWORD}@host.docker.internal:6379/0"
  frontend_origin="https://${DOMAIN}"
  public_origin="${frontend_origin}"
  media_public_base="${public_origin}/api/v1/media/object"

  if [[ "${PROXY}" == "none" ]]; then
    frontend_origin="http://${DOMAIN}:${GLIPZ_HOST_PORT}"
    public_origin="${frontend_origin}"
    media_public_base="${public_origin}/api/v1/media/object"
  fi

  if [[ "${STORAGE_MODE}" == "local" ]]; then
    write_env_file <<EOF
JWT_SECRET=${jwt_secret}
GLIPZ_VERSION=$(current_git_ref)
DATABASE_URL=${db_url}
REDIS_URL=${redis_url}
GLIPZ_STORAGE_MODE=local
GLIPZ_LOCAL_STORAGE_PATH=/app/data/media
LEGAL_DOCS_DIR=/app/data/legal-docs
FRONTEND_ORIGIN=${frontend_origin}
GLIPZ_PROTOCOL_PUBLIC_ORIGIN=${public_origin}
GLIPZ_PROTOCOL_HOST=${DOMAIN}
GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE=${media_public_base}
GLIPZ_MEDIA_PROXY_MODE=proxy
GLIPZ_TRUST_PROXY_HEADERS=true
EOF
  else
    write_env_file <<EOF
JWT_SECRET=${jwt_secret}
GLIPZ_VERSION=$(current_git_ref)
DATABASE_URL=${db_url}
REDIS_URL=${redis_url}
GLIPZ_STORAGE_MODE=s3
S3_ENDPOINT=${S3_ENDPOINT}
S3_PUBLIC_ENDPOINT=${S3_PUBLIC_ENDPOINT}
S3_REGION=${S3_REGION}
S3_ACCESS_KEY=${S3_ACCESS_KEY}
S3_SECRET_KEY=${S3_SECRET_KEY}
S3_BUCKET=${S3_BUCKET}
S3_USE_PATH_STYLE=${S3_USE_PATH_STYLE}
LEGAL_DOCS_DIR=/app/data/legal-docs
FRONTEND_ORIGIN=${frontend_origin}
GLIPZ_PROTOCOL_PUBLIC_ORIGIN=${public_origin}
GLIPZ_PROTOCOL_HOST=${DOMAIN}
GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE=${media_public_base}
GLIPZ_MEDIA_PROXY_MODE=proxy
GLIPZ_TRUST_PROXY_HEADERS=true
EOF
  fi
}

install_nginx_proxy() {
  log "installing and configuring Nginx"
  apt-get install -y nginx certbot python3-certbot-nginx

  cat >/etc/nginx/sites-available/glipz <<EOF
server {
    listen 80;
    server_name ${DOMAIN};

    client_max_body_size 100m;

    location ~ ^/api/v1/(posts/feed/stream|notifications/stream|dm/stream|public/posts/feed/stream|public/federation/incoming/stream)$ {
        proxy_pass http://127.0.0.1:${GLIPZ_HOST_PORT};
        proxy_http_version 1.1;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 1h;
        proxy_send_timeout 1h;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$remote_addr;
        proxy_set_header X-Forwarded-Proto \$scheme;
        add_header X-Accel-Buffering no;
    }

    location / {
        proxy_pass http://127.0.0.1:${GLIPZ_HOST_PORT};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$remote_addr;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF

  ln -sf /etc/nginx/sites-available/glipz /etc/nginx/sites-enabled/glipz
  rm -f /etc/nginx/sites-enabled/default
  nginx -t
  systemctl enable --now nginx
  systemctl reload nginx
  certbot --nginx -d "${DOMAIN}" --email "${LETSENCRYPT_EMAIL}" --agree-tos --non-interactive --redirect
}

install_caddy_proxy() {
  log "installing and configuring Caddy"
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL "https://dl.cloudsmith.io/public/caddy/stable/gpg.key" | gpg --dearmor -o /etc/apt/keyrings/caddy-stable-archive-keyring.gpg
  chmod a+r /etc/apt/keyrings/caddy-stable-archive-keyring.gpg
  curl -fsSL "https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt" >/etc/apt/sources.list.d/caddy-stable.list
  apt-get update
  apt-get install -y caddy

  cat >/etc/caddy/Caddyfile <<EOF
{
    email ${LETSENCRYPT_EMAIL}
}

${DOMAIN} {
    request_body {
        max_size 100MB
    }

    @sse path_regexp ^/api/v1/(posts/feed/stream|notifications/stream|dm/stream|public/posts/feed/stream|public/federation/incoming/stream)$
    reverse_proxy @sse 127.0.0.1:${GLIPZ_HOST_PORT} {
        flush_interval -1
    }

    reverse_proxy 127.0.0.1:${GLIPZ_HOST_PORT}
}
EOF

  systemctl enable --now caddy
  systemctl reload caddy
}

configure_proxy() {
  case "${PROXY}" in
    nginx) install_nginx_proxy ;;
    caddy) install_caddy_proxy ;;
    none) log "skipping reverse proxy configuration" ;;
  esac
}

main() {
  parse_args "$@"
  require_root
  check_ubuntu
  validate_inputs
  confirm_install
  check_existing_env

  POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-$(random_password)}"
  REDIS_PASSWORD="${REDIS_PASSWORD:-$(random_password)}"

  install_base_packages
  install_docker
  install_postgres_redis
  configure_postgres
  configure_redis
  prepare_source
  prepare_data_dirs
  write_glipz_env

  build_image "${GLIPZ_IMAGE_TAG}"
  run_glipz_container "${GLIPZ_IMAGE_TAG}"
  wait_for_health "http://127.0.0.1:${GLIPZ_HOST_PORT}/health" 45 2 || {
    docker logs --tail 120 "${GLIPZ_CONTAINER_NAME}" >&2 || true
    die "Glipz container did not become healthy"
  }

  configure_proxy

  log "installation completed"
  log "local health: http://127.0.0.1:${GLIPZ_HOST_PORT}/health"
  if [[ "${PROXY}" != "none" ]]; then
    log "public URL: https://${DOMAIN}"
  fi
  log "environment file: ${GLIPZ_ENV_FILE}"
}

main "$@"
