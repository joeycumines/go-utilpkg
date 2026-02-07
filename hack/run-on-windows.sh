#!/bin/sh

# ==============================================================================
# SCRIPT: run-on-windows.sh
# PURPOSE:
#   Executes a command on a remote Windows machine (running OpenSSH Server &
#   PowerShell 7) within a temporary, ephemeral clone of the current local
#   git repository state.
#
# FLOW:
#   1. LOCAL:  Snapshot git repo -> tar -> base64.
#   2. LOCAL:  Base64 encode user arguments to prevent quoting injection.
#   3. LOCAL:  Construct a PowerShell payload that handles the logic.
#   4. SSH:    Transport the payload and the data stream.
#   5. REMOTE: PowerShell creates an isolated TEMP dir (Security Fix).
#   6. REMOTE: PowerShell translates path using `wslpath`.
#   7. REMOTE: Bash decodes/untars stream strictly into TEMP (Security Fix).
#   8. REMOTE: Bash executes command using decoded arguments.
#   9. REMOTE: PowerShell `finally` block guarantees cleanup.
#
# REQUIREMENTS:
#   Local:  sh, git, tar, ssh, sed, base64
#   Remote: OpenSSH Server, PowerShell 7 (default shell), WSL enabled (bash.exe)
#
# USAGE:
#   ./run-on-windows.sh <destination> [command] [arguments...]
#
#   <destination> - The remote user@host (e.g., 'me@192.168.1.50')
#   [command]     - Optional command to run (defaults to 'ls -la')
#
#   Examples:
#     hack/run-on-windows.sh user@winbox make
# ==============================================================================

set -e

# Use pipefail if available (not in POSIX sh, but common e.g. bash/zsh)
if (set -o pipefail 2>/dev/null); then set -o pipefail; fi

# --- 1. Validation ---

if [ -z "$1" ]; then
  echo "Usage: $0 <destination> [command] [arguments...]" >&2
  echo "Example: $0 user@winbox make test" >&2
  exit 1
fi

SSH_HOST="$1"
shift

if [ ! -d ".git" ]; then
  echo "Error: Must be run from the root of a git repository." >&2
  exit 1
fi

# --- 2. Robust Argument Serialization (Base64) ---
# We serialize the arguments into a NUL-delimited stream, then Base64 encode it.
# This prevents shell injection and preserves whitespace/newlines in arguments.

if [ $# -eq 0 ]; then
  # Default command if none provided
  # N.B. 'ls -la' fails in PowerShell. We use 'Get-ChildItem -Force' (ls -Force).
  ARGS_B64=$(printf '%s\0' "ls" "-Force" | base64 | tr -d '\n')
else
  ARGS_B64=$(printf '%s\0' "$@" | base64 | tr -d '\n')
fi

# --- 3. Construct Remote PowerShell Payload ---
# This script runs on the Windows Host. It is injected with the encoded ARGS.
# It handles the creation of the sandbox and the handover to WSL.

# Note: We use a heredoc capture to prevent local shell expansion,
# EXCEPT for the __ARGS_B64__ placeholder which we sed-replace later.
# We use standard sh capture (cat) instead of read -d to ensure POSIX compliance.

REMOTE_PS_TEMPLATE=$(
  cat <<'EOF'
$ErrorActionPreference = 'Stop';
$exitCode = 0;

# 1. Secure Temporary Directory Creation
# Use Windows standard temp path with a GUID to ensure isolation.
$tempPath = Join-Path $env:TEMP "wsl-run-$([Guid]::NewGuid())";
$tempDir  = New-Item -ItemType Directory -Path $tempPath -Force;

try {
    # 2. Path Translation (Windows -> WSL)
    # We pass the path via Environment Variable to bash to avoid quoting issues.
    $env:WSL_TargetWinPath = $tempDir.FullName;

    # We use wslpath -u. We pipe $null to stdin to ensure bash doesn't steal
    # the main data stream waiting on stdin.
    $wslPath = ($null | bash.exe -c 'wslpath -u "$WSL_TargetWinPath"').Trim();

    if (-not $wslPath) { throw "Failed to translate Windows path to WSL path."; }

    # 3. Stream Extraction
    # The SSH connection is streaming Base64 text (the tarball) to StandardInput.
    # We invoke bash to read that stream, decode it, and untar it.
    # CRITICAL SECURITY FIX: We use -C to enforce extraction ONLY into the temp dir.
    # We strictly use bash.exe to handle the piping to avoid PowerShell encoding mangling.

    # Logic: Read Stdin (Base64) -> Decode -> Tar Extract to $wslPath
    bash.exe -c "base64 -d | tar --warning=no-unknown-keyword -x -f - -C '$wslPath'";

    if ($LASTEXITCODE -ne 0) { throw "Tar extraction failed."; }

    # 4. Command Execution
    # We decode the arguments (Base64) natively in PowerShell and execute
    # directly in the Windows shell context.
    $b64Args = '__ARGS_B64__';
    $bytes = [System.Convert]::FromBase64String($b64Args);
    $decodedArgs = [System.Text.Encoding]::UTF8.GetString($bytes);

    # Arguments are null-delimited. printf appends a trailing null,
    # so splitting yields an empty string at the end which we remove.
    $allArgs = $decodedArgs.Split([char]0);
    $cleanArgs = $allArgs[0..($allArgs.Length - 2)];

    if ($cleanArgs.Length -gt 0) {
        $cmd = $cleanArgs[0];
        $runArgs = @();
        if ($cleanArgs.Length -gt 1) {
            $runArgs = $cleanArgs[1..($cleanArgs.Length - 1)];
        }

        # Logic: Change Dir to Temp -> Execute Native Command
        Set-Location -Path $tempDir.FullName;
        & $cmd $runArgs;
        $exitCode = $LASTEXITCODE;
    }
}
catch {
    Write-Error $_.Exception.Message;
    $exitCode = 1;
}
finally {
    # 5. Guaranteed Cleanup
    # Runs regardless of success or failure.
    Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue;
}

exit $exitCode;
EOF
)

# Inject the calculated Base64 args into the template
REMOTE_PS_SCRIPT=$(echo "$REMOTE_PS_TEMPLATE" | sed "s|__ARGS_B64__|$ARGS_B64|")

# --- 4. Transport Preparation ---

# Base64 encode the *entire* PowerShell logic.
# This ensures that complex characters in the script (quotes, dollars, pipes)
# survive the SSH transport completely intact.
PS_PAYLOAD_B64=$(printf '%s' "$REMOTE_PS_SCRIPT" | base64 | tr -d '\n')

# The wrapper that runs on the remote machine.
# It decodes the payload and executes it.
# We use [Console]::In to ensure the SSH stream remains accessible to the inner script.
REMOTE_WRAPPER="powershell -NoProfile -NonInteractive -Command \"\$encoded='$PS_PAYLOAD_B64'; \$script=[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String(\$encoded)); Invoke-Expression \$script\""

# --- 5. Execution Pipeline ---
echo ">>> Syncing repository to $SSH_HOST..." >&2

# 1. git ls-files: Lists tracked + untracked files (excludes .gitignore).
# 2. tar: Bundles them up.
# 3. base64: Encodes the binary tarball to text for safe SSH transport.
# 4. ssh: Executes the wrapper, passing the base64 stream to it.

git ls-files -c -o --exclude-standard -z |
  tar --null -T - -c -f - |
  base64 |
  ssh -T "$SSH_HOST" "$REMOTE_WRAPPER"
