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

# Enable pipefail if available to catch git/tar errors in the pipeline
if set -o | grep -q "pipefail"; then set -o pipefail; fi

# --- 1. Environment & Context Validation ---

if [ -z "$1" ]; then
  echo "Usage: $0 <destination> [command] [arguments...]" >&2
  echo "Example: $0 user@winbox make test" >&2
  exit 1
fi

SSH_HOST="$1"
shift

# Fix: Resolve Repository Root to prevent CWD errors
REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)
if [ -z "$REPO_ROOT" ]; then
  echo "Error: Must be run from within a git repository." >&2
  exit 1
fi

# --- 2. Robust Argument Serialization ---

if [ $# -eq 0 ]; then
  # Default command: ls -Force (PowerShell syntax)
  ARGS_B64=$(printf '%s\0' "ls" "-Force" | base64 | tr -d '\n')
else
  # Serialize args to NUL-delimited stream -> Base64
  ARGS_B64=$(printf '%s\0' "$@" | base64 | tr -d '\n')
fi

# --- 3. Construct Remote PowerShell Payload ---

# TODO I'm quite sure the escaping here is dodgy...
REMOTE_PS_TEMPLATE=$(
  cat <<'EOF'
$ErrorActionPreference = 'Stop';
$exitCode = 0;

$tempPath = Join-Path $env:TEMP "wsl-run-$([Guid]::NewGuid())";
$tempDir  = New-Item -ItemType Directory -Path $tempPath -Force;

try {
    # 1. Path Translation
    # Use wsl.exe instead of bash.exe for better argument handling.
    # Normalize Windows path to forward slashes to prevent escaping errors in the shell parser.
    $wslExe = "C:\Windows\System32\wsl.exe";

    # Sanitize path: C:\Temp -> C:/Temp
    $cleanTempPath = $tempDir.FullName -replace '\\', '/';

    # Execute wslpath using explicit execution (-e)
    $wslPath = ($null | & $wslExe -e wslpath -u "$cleanTempPath").Trim();

    # Check LASTEXITCODE to catch "Invalid argument" errors even if stdout wasn't empty
    if ($LASTEXITCODE -ne 0 -or -not $wslPath) {
        throw "Failed to translate Windows path to WSL path (Exit Code: $LASTEXITCODE). Path was: $cleanTempPath";
    }

    # 2. Stream Extraction (Stdin -> Tar)
    $p = New-Object System.Diagnostics.Process;
    $p.StartInfo.FileName = $wslExe; # Use wsl.exe here too
    # Pass the clean wslPath.
    $p.StartInfo.Arguments = "-e bash -c 'set -o pipefail; base64 -d | tar -x -f - -C ""$1""' -- ""$wslPath""";
    $p.StartInfo.UseShellExecute = $false;
    $p.StartInfo.RedirectStandardInput = $true;
    $p.StartInfo.RedirectStandardOutput = $false;
    $p.StartInfo.RedirectStandardError = $false;

    $p.Start() | Out-Null;

    $parentIn = [Console]::OpenStandardInput();
    $childIn  = $p.StandardInput.BaseStream;
    $buffer   = New-Object byte[] 81920;

    # Safe writing loop. If tar dies (due to path error), we stop writing to avoid "Pipe Ended" crash.
    do {
        $count = $parentIn.Read($buffer, 0, $buffer.Length);
        if ($count -gt 0) {
            # Check if process is still alive before writing
            if ($p.HasExited) { break; }
            $childIn.Write($buffer, 0, $count);
        }
    } while ($count -gt 0);

    # Only flush/close if still running
    if (-not $p.HasExited) {
        $childIn.Flush();
        $childIn.Close();
    }

    $p.WaitForExit();

    if ($p.ExitCode -ne 0) { throw "Tar extraction failed (Exit Code: $($p.ExitCode))."; }

    # 3. Argument Decoding
    $b64Args = '__ARGS_B64__';
    $bytes = [System.Convert]::FromBase64String($b64Args);
    $decodedArgs = [System.Text.Encoding]::UTF8.GetString($bytes);

    # Split by NULL char.
    $allArgs = $decodedArgs.Split([char]0);

    # Fix: Robust Slicing (Handle trailing empty string from split)
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

        # 4. Execution
        Set-Location -Path $tempDir.FullName;

        # Fix: Use Splatting (@runArgs) to prevent array binding errors
        & $cmd @runArgs;

        $exitCode = $LASTEXITCODE;
    }
}
catch {
    Write-Error $_.Exception.Message;
    $exitCode = 1;
}
finally {
    # 5. Cleanup
    Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue;
}

exit $exitCode;
EOF
)

# Inject args
REMOTE_PS_SCRIPT=$(printf '%s\n' "$REMOTE_PS_TEMPLATE" | sed "s|__ARGS_B64__|$ARGS_B64|")

# --- 4. Transport Preparation ---

PS_PAYLOAD_B64=$(printf '%s' "$REMOTE_PS_SCRIPT" | base64 | tr -d '\n')

# SAFETY CHECK: Windows Command Line Limit
# 32767 chars is the hard limit for CreateProcess.
# We reserve ~2000 chars for the wrapper overhead.
PAYLOAD_LEN=${#PS_PAYLOAD_B64}
if [ "$PAYLOAD_LEN" -gt 30000 ]; then
  echo "Error: Argument list too long ($PAYLOAD_LEN bytes)." >&2
  echo "        Total serialized payload exceeds Windows command line limit (30KB)." >&2
  exit 1
fi

# Wrapper: Decodes and executes the payload.
# Note: We use 'powershell' (v5.1+) for maximum compatibility, though 'pwsh' (v7) is preferred if available.
REMOTE_WRAPPER="powershell -NoProfile -NonInteractive -Command \"\$encoded='$PS_PAYLOAD_B64'; \$script=[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String(\$encoded)); Invoke-Expression \$script\""

# --- 5. Execution Pipeline ---
echo ">>> Syncing repository to $SSH_HOST..." >&2

# Pipeline Logic:
# 1. git ls-files: List all files (Nul terminated) relative to root.
# 2. perl: Filters out files that don't exist on disk (Fixes 'Ghost File' crash).
# 3. tar: Archives files. Critical: -C "$REPO_ROOT" ensures correct context.
# 4. base64: Encodes for transport.
# 5. ssh: Executes wrapper.

git ls-files -c -o --exclude-standard -z |
  perl -0ne 'print if -e' |
  tar -C "$REPO_ROOT" --null -T - -c -f - |
  base64 |
  ssh -T "$SSH_HOST" "$REMOTE_WRAPPER"
