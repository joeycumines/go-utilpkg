#!/bin/sh

# ==============================================================================
# SCRIPT: run-on-windows.sh
# PURPOSE:
#   Executes a command on a remote Windows machine within a temporary, ephemeral
#   clone of the current local git repository state.
#
# ARCHITECTURE:
#   Three-phase SSH design. Bulk data never touches PowerShell — it streams
#   directly through SSH's binary channel into WSL's bash/tar.
#
#   SSH #1 (Setup):    PowerShell creates GUID-isolated TEMP dir, translates
#                      path via wslpath(1). Returns Windows + WSL paths.
#   SSH #2 (Transfer): Gzipped tarball streams over SSH → cmd.exe (inherited
#                      stdin) → wsl.exe → bash → tar. No PowerShell in the
#                      data path. No base64 encoding (SSH is binary-safe).
#   SSH #3 (Execute):  PowerShell decodes base64 arguments, executes command
#                      in the workspace, cleans up via try/finally block.
#
#   All PowerShell payloads (SSH #1 and #3) use -EncodedCommand with
#   base64-encoded UTF-16LE to completely bypass cmd.exe quoting issues.
#   Local trap handler provides backup cleanup on connection failure.
#
# REQUIREMENTS:
#   Local:  sh (POSIX), git, tar, perl, ssh, sed, base64
#   Remote: OpenSSH Server (cmd.exe default shell), PowerShell 5.1+,
#           WSL (wsl.exe with wslpath, bash, tar)
#
# USAGE:
#   ./run-on-windows.sh <destination> [command] [arguments...]
#
#   <destination> - SSH destination (e.g., 'user@winbox' or SSH config alias)
#   [command]     - Command to run (defaults to 'ls -Force')
#
# EXAMPLES:
#   hack/run-on-windows.sh winbox make test
#   hack/run-on-windows.sh user@192.168.1.50 go version
# ==============================================================================

set -e

# Enable pipefail if available (not POSIX, but catches mid-pipeline errors)
if set -o | grep -q "pipefail"; then set -o pipefail; fi

# --- 1. Argument Parsing & Validation ---

if [ -z "$1" ]; then
  echo "Usage: $0 <destination> [command] [arguments...]" >&2
  echo "Example: $0 user@winbox make test" >&2
  exit 1
fi

SSH_HOST="$1"
shift

REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)
if [ -z "$REPO_ROOT" ]; then
  echo "Error: Must be run from within a git repository." >&2
  exit 1
fi

# --- 2. Argument Serialization ---
# Serialize command arguments to NUL-delimited base64. This prevents any
# quoting/injection issues when reconstructing args on the remote side.
# The base64 charset [A-Za-z0-9+/=] is inherently safe for sed injection
# (cannot contain the delimiter '|' or special characters '&', '\').

if [ $# -eq 0 ]; then
  ARGS_B64=$(printf '%s\0' "ls" "-Force" | base64 | tr -d '\n')
else
  ARGS_B64=$(printf '%s\0' "$@" | base64 | tr -d '\n')
fi

# ==============================================================================
# SSH #1 — REMOTE SETUP
# ==============================================================================
# Creates a GUID-isolated temp directory under %TEMP% and translates its path
# to WSL mount form using wslpath(1). This respects the actual WSL mount
# configuration (e.g., /mnt/c vs /c) rather than hardcoding any prefix.
#
# The payload uses -EncodedCommand (base64-encoded UTF-16LE) to bypass all
# cmd.exe quoting issues. This is the OFFICIAL PowerShell mechanism for
# complex command passing via non-PowerShell parent processes.
# Returns two lines on stdout: forward-slash Windows path, WSL path.

SETUP_PS=$(cat <<'SETUP_EOF'
$ErrorActionPreference = 'Stop';
$ProgressPreference = 'SilentlyContinue';
$wslExe = 'C:\Windows\System32\wsl.exe';

$tempPath = Join-Path $env:TEMP "wsl-run-$([Guid]::NewGuid())";
$tempDir = New-Item -ItemType Directory -Path $tempPath -Force;

# Forward-slash form prevents backslash escaping issues across layers
$fwdPath = $tempDir.FullName -replace '\\', '/';

# wslpath(1) translates Windows paths to WSL mount paths correctly,
# regardless of the configured automount prefix (/mnt vs custom).
$wslPath = ($null | & $wslExe -e wslpath -u "$fwdPath").Trim();
if ($LASTEXITCODE -ne 0 -or -not $wslPath) {
    Remove-Item $tempDir -Recurse -Force -ErrorAction SilentlyContinue;
    throw "wslpath translation failed (exit=$LASTEXITCODE, input=$fwdPath)";
}

Write-Output $fwdPath;
Write-Output $wslPath;
SETUP_EOF
)

# Wrapper: uses -EncodedCommand (base64-encoded UTF-16LE) to bypass cmd.exe
# quoting issues entirely. This is the OFFICIAL PowerShell mechanism for
# complex command passing. Uses 'powershell' (5.1+) not 'pwsh' for maximum
# compatibility. All APIs ([Guid]::NewGuid, wslpath) are available in PS 5.1+.
SETUP_ENC=$(printf '%s' "$SETUP_PS" | iconv -f UTF-8 -t UTF-16LE | base64 | tr -d '\n')
SETUP_CMD="powershell -NoProfile -NonInteractive -EncodedCommand $SETUP_ENC"

echo ">>> Creating remote workspace on $SSH_HOST..." >&2

PATHS=$(ssh -T "$SSH_HOST" "$SETUP_CMD") || {
  echo "Error: Remote setup failed." >&2
  exit 1
}

# Parse output, stripping Windows \r line endings
WIN_PATH=$(printf '%s\n' "$PATHS" | sed -n '1p' | tr -d '\r')
WSL_PATH=$(printf '%s\n' "$PATHS" | sed -n '2p' | tr -d '\r')

if [ -z "$WIN_PATH" ] || [ -z "$WSL_PATH" ]; then
  echo "Error: Failed to parse remote paths (win='$WIN_PATH', wsl='$WSL_PATH')." >&2
  exit 1
fi

# Sanity check: paths from wslpath + GUID should never contain shell
# metacharacters. If they do, something went very wrong — abort rather
# than risk injection into the SSH #2 bash command.
case "$WSL_PATH" in
  *[\'\"\`\$\!]*)
    echo "Error: WSL path contains unsafe characters: $WSL_PATH" >&2
    exit 1
    ;;
esac

echo ">>>   Windows path: $WIN_PATH" >&2
echo ">>>   WSL path:     $WSL_PATH" >&2

# --- Cleanup Safety Net ---
# Belt-and-suspenders: local trap provides backup cleanup if SSH #3's
# PowerShell finally block doesn't execute (e.g., connectivity loss).
# If the remote dir is already cleaned up, Remove-Item with
# -ErrorAction SilentlyContinue is a harmless no-op.

CLEANUP_FIRED=0
cleanup_remote() {
  if [ "$CLEANUP_FIRED" = "0" ] && [ -n "$WIN_PATH" ]; then
    CLEANUP_FIRED=1
    ssh -o ConnectTimeout=5 -T "$SSH_HOST" \
      "powershell -NoProfile -NonInteractive -Command \"Remove-Item -Path '$WIN_PATH' -Recurse -Force -ErrorAction SilentlyContinue\"" \
      2>/dev/null || true
  fi
}
trap cleanup_remote EXIT INT TERM

# ==============================================================================
# SSH #2 — DATA TRANSFER
# ==============================================================================
# Streams a gzipped tarball of the repo directly over SSH's binary channel.
# The data path is: local tar → SSH transport → cmd.exe → wsl.exe → bash → tar.
# PowerShell is NOT in this path — this avoids the stdin piping bugs that
# plague PowerShell's System.Diagnostics.Process with large binary streams.
#
# Key safety properties:
#   - tar extracts relative paths only (default behavior — absolute path
#     stripping is on unless -P/--absolute-names is explicitly passed).
#   - Extraction is confined to the GUID-isolated temp directory (-C).
#   - gzip compression reduces transfer size ~3-5x vs raw tar.
#   - No base64 encoding (SSH's binary channel is natively binary-safe).
#   - --warning=no-unknown-keyword on extraction suppresses harmless warnings
#     from macOS bsdtar's extended attribute headers (e.g., com.apple.provenance).
#
# Pipeline details:
#   - cd REPO_ROOT: consistent path resolution regardless of caller's CWD.
#   - git ls-files -c -o --exclude-standard -z: lists tracked AND untracked
#     (non-ignored) files, NUL-delimited. Modified files are listed by name;
#     tar archives their actual disk content (not the index version).
#   - perl -0ne 'print if -e': filters ghost files (tracked by git but
#     deleted from disk) that would cause tar to fail with "No such file".
#   - tar --null -T -: reads NUL-delimited file list from stdin.

echo ">>> Transferring repository to $SSH_HOST..." >&2

# COPYFILE_DISABLE=1 prevents macOS bsdtar from embedding Apple extended
# attributes (LIBARCHIVE.xattr.*) that trigger harmless but noisy warnings
# on GNU tar extraction. Harmless no-op on non-macOS platforms.
# --no-mac-metadata (macOS bsdtar only) prevents xattr and ACL metadata.
TAR_CREATE_FLAGS="--null -T - -cz -f -"
case "$(uname -s)" in
  Darwin) TAR_CREATE_FLAGS="--no-mac-metadata $TAR_CREATE_FLAGS" ;;
esac

(cd "$REPO_ROOT" &&
  git ls-files -c -o --exclude-standard -z |
  perl -0ne 'print if -e' |
  COPYFILE_DISABLE=1 tar $TAR_CREATE_FLAGS) |
  ssh -T "$SSH_HOST" "C:\\Windows\\System32\\wsl.exe -e bash -c \"tar --warning=no-unknown-keyword -xzf - -C '${WSL_PATH}'\""

echo ">>> Transfer complete." >&2

# ==============================================================================
# SSH #3 — EXECUTE + CLEANUP
# ==============================================================================
# PowerShell script that decodes base64 arguments, runs the command in the
# workspace, and guarantees cleanup via try/finally.
#
# Like SSH #1, the payload uses -EncodedCommand (base64-encoded UTF-16LE)
# to completely bypass cmd.exe quoting. The user's command arguments are
# separately encoded as UTF-8 base64 (NUL-delimited) inside the script.

EXEC_PS_TEMPLATE=$(cat <<'EXEC_EOF'
$ErrorActionPreference = 'Stop';
$ProgressPreference = 'SilentlyContinue';
$exitCode = 0;
$winPath = '__WIN_PATH__';

try {
    # Decode NUL-delimited base64 arguments
    $b64Args = '__ARGS_B64__';
    $bytes = [System.Convert]::FromBase64String($b64Args);
    $decoded = [System.Text.Encoding]::UTF8.GetString($bytes);
    $allArgs = $decoded.Split([char]0);

    # Handle trailing empty string produced by Split on the final NUL
    if ($allArgs.Length -gt 1) {
        $cleanArgs = $allArgs[0..($allArgs.Length - 2)]
    } else {
        $cleanArgs = @()
    }

    if ($cleanArgs.Length -gt 0) {
        $cmd = $cleanArgs[0];
        $runArgs = @();
        if ($cleanArgs.Length -gt 1) {
            $runArgs = $cleanArgs[1..($cleanArgs.Length - 1)];
        }

        # Execute in the temporary workspace
        Set-Location -Path $winPath;

        # Splatting (@runArgs) prevents single-element array binding errors
        & $cmd @runArgs;

        $exitCode = $LASTEXITCODE;
    }
}
catch {
    Write-Error $_.Exception.Message;
    $exitCode = 1;
}
finally {
    # Guaranteed cleanup regardless of success or failure
    Set-Location $env:TEMP;
    Remove-Item -Path $winPath -Recurse -Force -ErrorAction SilentlyContinue;
}

exit $exitCode;
EXEC_EOF
)

# Template injection:
#   WIN_PATH is forward-slash form — no backslash escaping issues with sed.
#   ARGS_B64 charset [A-Za-z0-9+/=] cannot contain sed delimiter '|',
#   replacement special '&', or escape char '\'. Both are injection-safe.
EXEC_PS=$(printf '%s\n' "$EXEC_PS_TEMPLATE" | sed "s|__WIN_PATH__|$WIN_PATH|;s|__ARGS_B64__|$ARGS_B64|")

# PowerShell's -EncodedCommand accepts a Base64-encoded UTF-16LE string.
# This is the OFFICIAL mechanism for passing complex scripts to PowerShell
# without cmd.exe quoting/escaping concerns. The resulting command line is:
#   powershell -NoProfile -NonInteractive -EncodedCommand <base64>
# — no quotes, no dollar signs, no special characters whatsoever.
#
# Note: -EncodedCommand encoding is UTF-16LE→base64 (not UTF-8→base64).
# The UTF-8 base64 inside the script ($b64Args) is a SEPARATE encoding layer
# for the user's command arguments — decoded by the PowerShell script itself.
EXEC_ENC=$(printf '%s' "$EXEC_PS" | iconv -f UTF-8 -t UTF-16LE | base64 | tr -d '\n')

# Safety: Windows CreateProcess() limit is 32767 chars. Reserve headroom.
EXEC_LEN=${#EXEC_ENC}
if [ "$EXEC_LEN" -gt 30000 ]; then
  echo "Error: Payload exceeds Windows command line limit ($EXEC_LEN > 30000 chars)." >&2
  exit 1
fi

EXEC_CMD="powershell -NoProfile -NonInteractive -EncodedCommand $EXEC_ENC"

EXEC_CMD_LEN=${#EXEC_CMD}
echo ">>>   Payload: ${EXEC_LEN} chars (encoded), ${EXEC_CMD_LEN} chars (full command)" >&2

echo ">>> Executing on $SSH_HOST..." >&2

# Disable set -e so we can capture the exit code from the remote command.
# The PS try/finally handles cleanup regardless of command success/failure.
# Use -o ControlMaster=no to avoid SSH multiplexing issues where the Phase 2
# data session may not have fully closed on the mux master yet.
set +e
ssh -o ControlPath=none -T "$SSH_HOST" "$EXEC_CMD"
EXIT_CODE=$?
echo ">>> SSH exit code: $EXIT_CODE" >&2
set -e

# If SSH completed normally (exit != 255), the PS finally block already
# handled cleanup. Mark the trap as done to avoid a redundant SSH call.
# If SSH itself failed (255 = connection error), the trap will attempt
# cleanup separately.
if [ "$EXIT_CODE" -ne 255 ]; then
  CLEANUP_FIRED=1
fi

exit "$EXIT_CODE"
