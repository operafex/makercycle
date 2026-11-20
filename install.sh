#!/bin/sh
# Instalador do makercycle (macOS e Linux).
#   curl -fsSL https://raw.githubusercontent.com/operafex/makercycle/main/install.sh | sh
#
# Variáveis opcionais:
#   MAKERCYCLE_VERSION      versão fixa no formato vX.Y.Z[-pre] (padrão: latest)
#   MAKERCYCLE_INSTALL_DIR  diretório de instalação (padrão: $HOME/.local/bin)
#   MAKERCYCLE_DRY_RUN=1    só imprime a resolução (asset, URLs, diretório), sem rede
#   MAKERCYCLE_OS / MAKERCYCLE_ARCH  sobrepõem a detecção (uname -s / uname -m)
set -eu

REPO="operafex/makercycle"
BASE_URL="https://github.com/${REPO}/releases"

die() {
  echo "makercycle: $*" >&2
  exit 1
}

os_raw="${MAKERCYCLE_OS:-$(uname -s)}"
arch_raw="${MAKERCYCLE_ARCH:-$(uname -m)}"

case "$os_raw" in
  Darwin) os="darwin" ;;
  Linux) os="linux" ;;
  *) die "sistema/arquitetura não suportado: ${os_raw}/${arch_raw}" ;;
esac
case "$arch_raw" in
  x86_64 | amd64) arch="amd64" ;;
  arm64 | aarch64) arch="arm64" ;;
  *) die "sistema/arquitetura não suportado: ${os_raw}/${arch_raw}" ;;
esac

asset="makercycle-${os}-${arch}"

version="${MAKERCYCLE_VERSION:-}"
if [ -z "$version" ]; then
  release_path="latest/download"
else
  case "$version" in
    *[!0-9A-Za-z.v-]*) die "versão inválida: ${version} (use o formato vX.Y.Z)" ;;
  esac
  if ! printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then
    die "versão inválida: ${version} (use o formato vX.Y.Z)"
  fi
  release_path="download/${version}"
fi

url="${BASE_URL}/${release_path}/${asset}"
checksums_url="${BASE_URL}/${release_path}/checksums.txt"
INSTALL_DIR="${MAKERCYCLE_INSTALL_DIR:-$HOME/.local/bin}"

if [ "${MAKERCYCLE_DRY_RUN:-}" = "1" ]; then
  echo "asset=${asset}"
  echo "url=${url}"
  echo "checksums_url=${checksums_url}"
  echo "install_dir=${INSTALL_DIR}"
  exit 0
fi

download() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1"
  else
    die "curl ou wget é necessário para baixar o makercycle"
  fi
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    die "sha256sum ou shasum é necessário para verificar o download"
  fi
}

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
trap 'exit 1' INT TERM HUP

echo "Baixando ${asset} (${release_path})..."
download "$url" "${tmp_dir}/${asset}" || die "falha ao baixar ${url}"
download "$checksums_url" "${tmp_dir}/checksums.txt" || die "falha ao baixar ${checksums_url}"

expected="$(awk -v a="$asset" '$2 == a || $2 == "*" a { print $1; exit }' "${tmp_dir}/checksums.txt")"
[ -n "$expected" ] || die "checksum de ${asset} não encontrado em checksums.txt"
actual="$(sha256_of "${tmp_dir}/${asset}")"
if [ "$expected" != "$actual" ]; then
  die "checksum divergente para ${asset} (esperado ${expected}, obtido ${actual}); instalação abortada"
fi
echo "Checksum SHA-256 verificado."

mkdir -p "$INSTALL_DIR"
chmod +x "${tmp_dir}/${asset}"
mv -f "${tmp_dir}/${asset}" "${INSTALL_DIR}/makercycle"

path_note=""
case ":${PATH:-}:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    case "${SHELL:-}" in
      */zsh) rc="$HOME/.zshrc" ;;
      */bash) rc="$HOME/.bashrc" ;;
      *) rc="$HOME/.profile" ;;
    esac
    case "$INSTALL_DIR" in
      *[!A-Za-z0-9_./~+@:,=' '-]*)
        path_note="Adicione ${INSTALL_DIR} ao PATH manualmente (caracteres especiais no caminho)."
        ;;
      *)
        if [ -f "$rc" ] && grep -Fq "# makercycle" "$rc"; then
          path_note="PATH já configurado em ${rc}; abra um novo terminal."
        else
          printf '\n# makercycle\nexport PATH="%s:$PATH"\n' "$INSTALL_DIR" >> "$rc"
          path_note="PATH atualizado em ${rc}; abra um novo terminal."
        fi
        ;;
    esac
    ;;
esac

echo "Instalado: ${INSTALL_DIR}/makercycle — rode: makercycle -init"
if [ -n "$path_note" ]; then
  echo "$path_note"
fi
