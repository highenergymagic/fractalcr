param(
    [ValidateSet("boot", "os", "all")][string]$Stage = "all",
    [ValidateSet("Start", "Status", "Logs", "Export")][string]$Action = "Start",
    [ValidateSet("Build", "Test", "All", "Reproduce")][string]$BuildAction = "All",
    [string]$BuildHost = "carbon",
    [ValidateSet("development", "external")][string]$SigningMode = "development",
    [string]$PublicTrustDirectory
)
$ErrorActionPreference = "Stop"
if ($BuildHost -notmatch '^[a-zA-Z0-9_.@-]+$' -or $BuildHost.StartsWith("-")) {
    throw "BuildHost must be an SSH host alias or user@host."
}
$repoRoot = Split-Path -Parent $PSScriptRoot
$volumePrefix = "fractalcr"
$trustID = "development"
if ($SigningMode -eq "external") {
    if (-not $PublicTrustDirectory) { throw "External signing requires -PublicTrustDirectory with firmware.crt, release.pub and trust.json only." }
    foreach ($entry in Get-ChildItem -LiteralPath $PublicTrustDirectory -Force) {
        if ($entry.Name -notin @('firmware.crt','release.pub','trust.json') -or $entry.PSIsContainer -or ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
            throw "The public trust directory must contain only three regular public files."
        }
    }
    foreach ($name in @("firmware.crt", "release.pub", "trust.json")) {
        $path = Join-Path $PublicTrustDirectory $name
        if (-not (Test-Path -LiteralPath $path -PathType Leaf) -or (Get-Content -Raw -LiteralPath $path) -match 'PRIVATE KEY') { throw "Invalid public trust file: $name" }
    }
    $trustID = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $PublicTrustDirectory 'trust.json')).Hash.ToLower().Substring(0,16)
    $volumePrefix = "fractalcr-release-$trustID"
    if ($Action -eq "Start" -and ($Stage -eq "all" -or $BuildAction -ne "Build")) { throw "External signing builds unsigned outputs. Select -Stage boot or os -BuildAction Build, then sign and verify offline." }
}
function Invoke-Remote([string]$Command) {
    & ssh -o BatchMode=yes -o ConnectTimeout=10 $BuildHost $Command
    if ($LASTEXITCODE -ne 0) { throw "Remote command failed ($LASTEXITCODE)" }
}
switch ($Action) {
    "Start" {
        if ($Stage -eq "all" -and $BuildAction -notin @("All", "Reproduce")) { throw "Stage all requires All or Reproduce." }
        Invoke-Remote 'doas -n docker info --format "{{.OSType}}/{{.Architecture}} CPUs={{.NCPU}}"'
        $job = [DateTime]::UtcNow.ToString("yyyyMMddTHHmmss") + "-" + [Guid]::NewGuid().ToString("N").Substring(0,8)
        $archive = Join-Path ([IO.Path]::GetTempPath()) "fractalcr-$job.tar.gz"
        $trustStage = Join-Path ([IO.Path]::GetTempPath()) "fractalcr-public-$job"
        try {
            if ($SigningMode -eq "external") {
                New-Item -ItemType Directory -Path (Join-Path $trustStage 'release-trust') -Force | Out-Null
                foreach ($name in @('firmware.crt','release.pub','trust.json')) { Copy-Item -LiteralPath (Join-Path $PublicTrustDirectory $name) -Destination (Join-Path $trustStage "release-trust/$name") }
            }
            $snapshotArgs = @('--output', $archive)
            if ($SigningMode -eq 'external') { $snapshotArgs += @('--public-trust', (Join-Path $trustStage 'release-trust')) }
            & python (Join-Path $PSScriptRoot 'workspace-archive.py') @snapshotArgs
            if ($LASTEXITCODE -ne 0) { throw "Source archive failed." }
            Invoke-Remote "mkdir -p FractalCR-build/snapshots/$job"
            & scp -q $archive "${BuildHost}:FractalCR-build/snapshots/$job/source.tar.gz"
            if ($LASTEXITCODE -ne 0) { throw "Source upload failed." }
            # Keep the launch lock until the worker has acquired the build lock.
            $launch = @'
set -eu
state="$HOME/FractalCR-build"
exec 8>"$state/launch.lock"
flock -n 8 || { echo "Another build is starting." >&2; exit 1; }
flock -n "$state/build.lock" true || { echo "A build is already running." >&2; exit 1; }
src="$state/snapshots/JOB"
tar -xzf "$src/source.tar.gz" -C "$src"
mkdir -p "$state/logs"
if test -f "$state/build.log"; then
    cp "$state/build.log" "$state/logs/before-JOB.log"
fi
rm -f "$state/exit-code" "$state/finished"
nohup bash "$src/tools/remote-build.sh" STAGE PHASE SIGNING TRUSTID 8>&- >"$state/build.log" 2>&1 </dev/null &
pid=$!
echo "$pid" > "$state/pid"
for attempt in $(seq 1 50); do
    if ! flock -n "$state/build.lock" true; then break; fi
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.1
done
echo "Started build PID $pid from snapshot JOB"
'@
            $launch = $launch.Replace("JOB", $job).Replace("STAGE", $Stage).Replace("PHASE", $BuildAction).Replace("SIGNING", $SigningMode).Replace("TRUSTID", $trustID)
            $launch.Replace("`r", "") | & ssh -o BatchMode=yes $BuildHost bash -s
            if ($LASTEXITCODE -ne 0) { throw "Remote build launch failed." }
        } finally {
            if (Test-Path -LiteralPath $archive) { Remove-Item -LiteralPath $archive }
            if (Test-Path -LiteralPath $trustStage) {
                $resolved = [IO.Path]::GetFullPath($trustStage)
                $expected = [IO.Path]::GetFullPath((Join-Path ([IO.Path]::GetTempPath()) "fractalcr-public-$job"))
                if ($resolved -ne $expected) { throw "Unexpected public trust staging path" }
                Remove-Item -LiteralPath $resolved -Recurse -Force
            }
        }
        Write-Host "Use tools/remote-build.ps1 -Action Status or -Action Logs."
    }
    "Status" {
        Invoke-Remote 'state="$HOME/FractalCR-build"; if test -f "$state/exit-code"; then printf "Exit code: "; cat "$state/exit-code"; elif test -f "$state/build.lock" && ! flock -n "$state/build.lock" true; then echo Running; else echo "No running build"; fi; test ! -f "$state/source" || cat "$state/source"; test ! -f "$state/build.log" || tail -n 12 "$state/build.log"; if test -f "$state/exit-code" && test "$(cat "$state/exit-code")" != 0; then echo "Failure details:"; grep -E "^ERROR:|^fatal:|[Ee]rror:|Exception:" "$state/build.log" | tail -n 8; fi'
    }
    "Logs" {
        Invoke-Remote 'tail -n 70 "$HOME/FractalCR-build/build.log"'
    }
    "Export" {
        $targets = if ($Stage -eq "all") { @("boot", "os") } else { @($Stage) }
        foreach ($target in $targets) {
            # Embedded images are already compressed; fast gzip avoids spending
            # minutes recompressing their repeated copies in QEMU disk fixtures.
            Invoke-Remote "flock -n FractalCR-build/build.lock sh -c 'doas -n docker run --rm --log-driver none --mount type=volume,source=$volumePrefix-$target-work,target=/work,readonly fractalcr-$target-builder tar -C /work/artifacts -I `"gzip -1`" -cf - . > FractalCR-build/$target-artifacts.tar.gz'"
            $output = if ($SigningMode -eq 'external') { Join-Path $repoRoot "out\carbon\release-$trustID\$target" } else { Join-Path $repoRoot "out\carbon\$target" }
            New-Item -ItemType Directory -Force $output | Out-Null
            $archive = Join-Path $output "artifacts.tar.gz"
            & scp -q "${BuildHost}:FractalCR-build/$target-artifacts.tar.gz" $archive
            if ($LASTEXITCODE -ne 0) { throw "Artifact download failed." }
            & tar -xzf $archive -C $output
            if ($LASTEXITCODE -ne 0) { throw "Artifact extraction failed." }
            Remove-Item -LiteralPath $archive
            Write-Host "Exported artifacts to $output"
        }
    }
}
