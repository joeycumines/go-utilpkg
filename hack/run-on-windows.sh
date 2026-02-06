-- hack/run-on-windows.sh --
#!/bin/sh

# ==============================================================================
# SCRIPT: run-on-win.sh
# PURPOSE:
#   Executes a command on a remote Windows machine (running OpenSSH Server &
#   PowerShell 7) within a temporary, ephemeral clone of the current local
#   git repository state.
#
# BEHAVIOR:
#   1. SNAPSHOT: Captures the current local state using `git ls-files`.
#      - Includes: Committed files, Staged changes, Unstaged files.
#      - Excludes: Files ignored by .gitignore, the .git directory itself.
#   2. TRANSPORT: Pipes the state as a tarball over SSH to the remote host.
#   3. PROVISION: On Windows (via PowerShell):
#      - Creates a temporary directory in $env:TEMP.
#      - Translates the Windows path to a WSL path (using `wslpath`).
#   4. EXTRACTION: Uses remote `bash.exe` to untar the stream into the temp dir.
#   5. EXECUTION: Runs the user-provided arguments ("$@") inside that directory
#      using `bash.exe`.
#   6. CLEANUP: GUARANTEES removal of the temporary directory using a PowerShell
#      `try...finally` block, ensuring cleanup occurs even if execution fails.
#
# REQUIREMENTS:
#   Local:  sh, git, tar, ssh, sed, base64
#   Remote: OpenSSH Server, PowerShell 7 (default shell), WSL enabled (bash.exe)
#
# USAGE:
#   ./run-on-win.sh <destination> [command] [arguments...]
#
#   <destination> - The remote user@host (e.g., 'me@192.168.1.50')
#   [command]     - Optional command to run (defaults to 'ls -la')
#
#   Examples:
#     ./run-on-win.sh user@winbox make test
#     ./run-on-win.sh user@winbox bash -c "ls -la && ./build.sh"
#     ./run-on-win.sh user@winbox
# ==============================================================================

set -e

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
# This bypasses all quoting hell (single quotes, double quotes, $, etc.)
# and preserves full fidelity (including newlines).
ARGS_B64=$(printf '%s\0' "$@" | base64 | tr -d '\n')

# If no command provided, default to listing the directory
if [ -z "$ARGS_B64" ]; then
  # Base64 for "ls -la"
  ARGS_B64=$(printf '%s\0' "ls" "-la" | base64 | tr -d '\n')
fi

# --- 3. Construct Remote PowerShell Payload ---
# We pass the Base64 args and the Base64 tarball (from stdin) to the remote.
# Note: We use a specific delimiter strategy to separate the logic.

REMOTE_PS_SCRIPT="
\$ErrorActionPreference = 'Stop';
\$tempDir = New-Item -ItemType Directory -Path \"\$env:TEMP\wsl-run-\$(New-Guid)\";

try {
    # 1. Read Tarball from Stdin (Base64 Encoded)
    # We do NOT capture \$input here. We allow the stdin stream to pass through
    # directly to the bash process in step 3. This prevents PowerShell from
    # corrupting the base64 stream via object enumeration or re-formatting.

    # 2. Convert Windows path to WSL path safely
    # We pass the path via env var to avoid quoting injection
    \$env:WSL_TEMP_DIR = \$tempDir.FullName;
    # We pipe \$null to ensure this bash instance doesn't consume the main stdin stream
    \$wslPath = \$null | bash.exe -c 'wslpath -u \"\$WSL_TEMP_DIR\"';
    if (\$LASTEXITCODE -ne 0) { throw 'WSL Path conversion failed'; }

    # 3. Decode and Extract Tarball
    # bash.exe inherits the stdin stream directly from the parent process
    # raw stdin stream -> bash -> base64 -d -> tar
    bash.exe -c \"base64 -d | tar --warning=no-unknown-keyword -x -f - -C '\$wslPath'\";
    if (\$LASTEXITCODE -ne 0) { throw 'Tar extraction failed'; }

    # 4. Execute User Command
    # We pass the B64 args via env var to avoid injection
    \$env:B64_ARGS = '$ARGS_B64';

    # The bash script reconstructs the args from B64 and execs them
    bash.exe -c \"cd '\$wslPath' && echo \$B64_ARGS | base64 -d | xargs -0 sh -c 'exec \\\"\$@\\\"' --\";

    exit \$LASTEXITCODE;
}
catch {
    Write-Error \$_.Exception.Message;
    exit 1;
}
finally {
    Remove-Item -Path \$tempDir -Recurse -Force -ErrorAction SilentlyContinue;
}
"

# --- 4. Execution Pipeline ---
echo ">>> Syncing dirty state to $SSH_HOST..." >&2

# Check for pipefail support (POSIX sh doesn't always have it, but bash/zsh do)
if (set -o pipefail 2>/dev/null); then set -o pipefail; fi

# Encode the full script to Base64 to ensure safe transport over SSH.
# This prevents the 'unexpected EOF' and syntax errors caused by nested quotes
# within the SSH command arguments.
PS_PAYLOAD_B64=$(printf '%s' "$REMOTE_PS_SCRIPT" | base64 | tr -d '\n')

# Construct the remote wrapper to decode and execute.
# \$encoded holds the safe Base64 string.
# Invoke-Expression executes the decoded logic in the current scope,
# ensuring Standard Input is inherited by the inner bash commands.
REMOTE_WRAPPER="\$encoded='$PS_PAYLOAD_B64';\$script=[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String(\$encoded));Invoke-Expression \$script"

# Logic:
# 1. git ls-files: list files
# 2. tar: create archive
# 3. base64: Encode binary tar stream to text (avoids SSH/PS newline corruption)
# 4. ssh: pass text stream (and execute the Base64 wrapper)
git ls-files -c -o --exclude-standard -z |
  tar --null -T - -c -f - |
  base64 |
  ssh -T "$SSH_HOST" "$REMOTE_WRAPPER"
