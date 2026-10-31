#!/usr/bin/env bash
set -euo pipefail

ITEMGATHER_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="${ENV_FILE:-/etc/com-itemgather/com-itemgather.env}"
if [[ ! -r "$ENV_FILE" ]]; then
  printf '无法读取部署环境文件：%s\n请设置 ENV_FILE 或按 README 安装配置。\n' "$ENV_FILE" >&2
  exit 1
fi
set -a
source "$ENV_FILE"
set +a
cd "$ITEMGATHER_ROOT"

# Prompt without putting a password in shell history or the process arguments.
if [[ "${1:-}" == create-admin && -z "${ADMIN_PASSWORD:-}" && -t 0 ]]; then
  read -r -s -p '初始管理员密码（至少 10 个字符）：' ADMIN_PASSWORD
  printf '\n' >&2
  export ADMIN_PASSWORD
fi
if [[ "${1:-}" == reset-password && -z "${RESET_PASSWORD:-}" && -t 0 ]]; then
  read -r -s -p '新的账号密码（至少 10 个字符）：' RESET_PASSWORD
  printf '\n' >&2
  export RESET_PASSWORD
fi

if (( EUID == 0 )); then
  # runuser preserves the loaded application environment without exposing keys
  # as command-line arguments. Files are created by the same user as systemd.
  exec runuser -u "${ITEMGATHER_SERVICE_USER:-com-itemgather}" -- "$ITEMGATHER_ROOT/bin/com-itemgather" "$@"
fi
exec "$ITEMGATHER_ROOT/bin/com-itemgather" "$@"
