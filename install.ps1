# Instalador do makercycle (Windows, PowerShell 5.1+).
#   irm https://raw.githubusercontent.com/operafex/makercycle/main/install.ps1 | iex
#
# Variaveis opcionais:
#   MAKERCYCLE_VERSION      versao fixa no formato vX.Y.Z[-pre] (padrao: latest)
#   MAKERCYCLE_INSTALL_DIR  diretorio de instalacao (padrao: %LOCALAPPDATA%\Programs\makercycle)
#   MAKERCYCLE_DRY_RUN=1    so imprime a resolucao (asset, URLs, diretorio), sem rede
#   MAKERCYCLE_ARCH         sobrepoe a deteccao (AMD64 ou ARM64)
# Obs.: mensagens sem acentos de proposito (o arquivo roda tambem via `irm | iex`, sem BOM/codificacao garantida).
$ErrorActionPreference = 'Stop'

$repo = 'operafex/makercycle'
$baseUrl = "https://github.com/$repo/releases"

# throw (e nao `exit`) para nao fechar o terminal do usuario quando executado via `iex`.
function Fail([string]$msg) { throw "makercycle: $msg" }

$archRaw = $env:MAKERCYCLE_ARCH
if (-not $archRaw) { $archRaw = $env:PROCESSOR_ARCHITEW6432 }
if (-not $archRaw) { $archRaw = $env:PROCESSOR_ARCHITECTURE }
switch ("$archRaw".ToUpperInvariant()) {
    'AMD64' { $arch = 'amd64' }
    'X86_64' { $arch = 'amd64' }
    'ARM64' { $arch = 'arm64' }
    default { Fail "arquitetura nao suportada: $archRaw" }
}
$asset = "makercycle-windows-$arch.exe"

$version = $env:MAKERCYCLE_VERSION
if (-not $version) {
    $releasePath = 'latest/download'
} else {
    if ($version -cnotmatch '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?\z') {
        Fail "versao invalida: $version (use o formato vX.Y.Z)"
    }
    $releasePath = "download/$version"
}

$url = "$baseUrl/$releasePath/$asset"
$checksumsUrl = "$baseUrl/$releasePath/checksums.txt"
$installDir = $env:MAKERCYCLE_INSTALL_DIR
if (-not $installDir) { $installDir = Join-Path $env:LOCALAPPDATA 'Programs\makercycle' }

if ($env:MAKERCYCLE_DRY_RUN -eq '1') {
    Write-Output "asset=$asset"
    Write-Output "url=$url"
    Write-Output "checksums_url=$checksumsUrl"
    Write-Output "install_dir=$installDir"
    return
}

try { [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 } catch { }

$tmpDir = Join-Path $env:TEMP ("makercycle-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tmpDir | Out-Null
try {
    $tmpBin = Join-Path $tmpDir $asset
    $tmpSums = Join-Path $tmpDir 'checksums.txt'

    Write-Output "Baixando $asset ($releasePath)..."
    Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $tmpBin
    Invoke-WebRequest -UseBasicParsing -Uri $checksumsUrl -OutFile $tmpSums

    $expected = $null
    foreach ($line in Get-Content -Path $tmpSums) {
        $parts = $line.Trim() -split '\s+'
        if ($parts.Count -ge 2 -and ($parts[1] -replace '^\*', '') -eq $asset) { $expected = $parts[0]; break }
    }
    if (-not $expected) { Fail "checksum de $asset nao encontrado em checksums.txt" }
    $actual = (Get-FileHash -Algorithm SHA256 -Path $tmpBin).Hash
    if ($expected -ne $actual) {
        Fail "checksum divergente para $asset (esperado $expected, obtido $actual); instalacao abortada"
    }
    Write-Output 'Checksum SHA-256 verificado.'

    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
    Move-Item -Force -Path $tmpBin -Destination (Join-Path $installDir 'makercycle.exe')
} finally {
    Remove-Item -Recurse -Force -Path $tmpDir -ErrorAction SilentlyContinue
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$entries = @()
if ($userPath) { $entries = $userPath -split ';' | ForEach-Object { $_.TrimEnd('\') } }
if ($entries -notcontains $installDir.TrimEnd('\')) {
    if ($userPath) { $newPath = "$userPath;$installDir" } else { $newPath = $installDir }
    [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
    $env:Path = "$env:Path;$installDir"
    Write-Output "PATH do usuario atualizado com $installDir (abra um novo terminal)."
}

Write-Output "Instalado: $(Join-Path $installDir 'makercycle.exe') - rode: makercycle -init"
