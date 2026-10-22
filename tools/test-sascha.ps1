param(
    [string]$TestHost = "freyja@sascha",
    [string]$ArtifactDirectory,
    [string[]]$Cases = @("containers", "no-datastore", "invalid-config", "optional-firmware", "blank-datastore", "cli", "container-tools", "kata", "debug-container", "aaa", "swarm-multinode")
)
$ErrorActionPreference = "Stop"
if ($TestHost -notmatch '^[a-zA-Z0-9_.@-]+$' -or $TestHost.StartsWith("-")) { throw "Invalid SSH host" }
$allowed = @("containers", "no-datastore", "invalid-config", "optional-firmware", "blank-datastore", "cli", "container-tools", "kata", "debug-container", "aaa", "swarm-multinode")
foreach ($case in $Cases) { if ($case -notin $allowed) { throw "Invalid case: $case" } }
$repo = Split-Path -Parent $PSScriptRoot
if (-not $ArtifactDirectory) { $ArtifactDirectory = Join-Path $repo "out\carbon\os" }
$id = [DateTime]::UtcNow.ToString("yyyyMMddTHHmmss") + "-" + [Guid]::NewGuid().ToString("N").Substring(0,8)
$remote = "/machines/fractalcr/test-runs/$id"
$output = Join-Path $repo "out\sascha\$id"
New-Item -ItemType Directory -Force $output | Out-Null
$archive = Join-Path $output "source.tar.gz"
& python (Join-Path $PSScriptRoot 'workspace-archive.py') --output $archive
if ($LASTEXITCODE) { throw "Source archive failed" }
& ssh -o BatchMode=yes $TestHost "mkdir -p $remote/source $remote/artifacts; test -r /dev/kvm -a -w /dev/kvm"
if ($LASTEXITCODE) { throw "Test directory or KVM unavailable" }
& scp -q $archive "${TestHost}:$remote/source.tar.gz"
if ($LASTEXITCODE) { throw "Source upload failed" }
foreach ($file in @("system.bin", "rommon.efi", "OVMF_CODE_4M.secboot.fd", "OVMF_VARS-trusted.fd", "OVMF_VARS-unenrolled.fd")) {
    & scp -q (Join-Path $ArtifactDirectory $file) "${TestHost}:$remote/artifacts/$file"
    if ($LASTEXITCODE) { throw "Image upload failed: $file" }
}
& ssh -o BatchMode=yes $TestHost "tar -xzf $remote/source.tar.gz -C $remote/source"
if ($LASTEXITCODE) { throw "Source extraction failed" }
$failed = @()
$coreRan = $false
foreach ($case in $Cases) {
    if ($case -in @('aaa','swarm-multinode')) {
        $script = if ($case -eq 'aaa') { 'qemu-aaa.py' } else { 'qemu-swarm.py' }
        & ssh -o BatchMode=yes $TestHost "PATH=/usr/sbin:/sbin:`$PATH python3 $remote/source/os/tests/$script --artifacts $remote/artifacts --output $remote/artifacts/$case"
        $result = $LASTEXITCODE
        # Large disposable disks stay on sascha; download only evidence below.
        $caseOutput = Join-Path $output $case
        New-Item -ItemType Directory -Force $caseOutput | Out-Null
        & scp -q "${TestHost}:$remote/artifacts/$case/*.json" "${TestHost}:$remote/artifacts/$case/*.log" $caseOutput
        if ($LASTEXITCODE) { throw "Evidence download failed; retained at $remote/artifacts/$case" }
        Copy-Item -LiteralPath (Join-Path $caseOutput "result-os-$case.json") -Destination $output
        if ($case -eq 'swarm-multinode' -and -not $result) {
            & scp -q "${TestHost}:$remote/artifacts/$case/overlay-only-*.pcap" $caseOutput
            if ($LASTEXITCODE) { throw "Overlay capture download failed" }
        }
        if ($result) { $failed += $case }
        continue
    }
    $coreRan = $true
    & ssh -o BatchMode=yes $TestHost "PATH=/usr/sbin:/sbin:`$PATH python3 $remote/source/os/tests/qemu-system.py --case $case --artifacts $remote/artifacts --accel kvm"
    if ($LASTEXITCODE) { $failed += $case }
}
if ($coreRan) {
    & scp -q "${TestHost}:$remote/artifacts/result-os-*.json" "${TestHost}:$remote/artifacts/qemu-os-*.log" $output
    if ($LASTEXITCODE) { throw "Evidence download failed; retained at $remote" }
}
Write-Host "Evidence: $output"
Write-Host "Disposable VM fixtures retained at ${TestHost}:$remote"
if ($failed.Count) { throw "Failed cases: $($failed -join ', ')" }
