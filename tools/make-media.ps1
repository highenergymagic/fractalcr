param(
    [string]$BuildHost = 'carbon',
    [ValidateSet('required','optional')][string]$FirmwarePolicy = 'required',
    [ValidateRange(2048,1048576)][int]$SizeMiB = 2048,
    [switch]$LoaderOnly,
    [string]$OutputDirectory
)
$ErrorActionPreference = 'Stop'
if ($BuildHost -notmatch '^[a-zA-Z0-9_.@-]+$' -or $BuildHost.StartsWith('-')) { throw 'Invalid SSH host' }
$repo = Split-Path -Parent $PSScriptRoot
$job = [DateTime]::UtcNow.ToString('yyyyMMddTHHmmss') + '-' + [Guid]::NewGuid().ToString('N').Substring(0,8)
if (-not $OutputDirectory) { $OutputDirectory = Join-Path $repo "out\media\$job" }
New-Item -ItemType Directory -Force $OutputDirectory | Out-Null
$archive = Join-Path $OutputDirectory 'source.tar.gz'
& python (Join-Path $PSScriptRoot 'workspace-archive.py') --output $archive
if ($LASTEXITCODE) { throw 'Source snapshot failed' }
& ssh -o BatchMode=yes $BuildHost "mkdir -p FractalCR-media/$job/source FractalCR-media/$job/output"
if ($LASTEXITCODE) { throw 'Remote media directory failed' }
& scp -q $archive "${BuildHost}:FractalCR-media/$job/source.tar.gz"
if ($LASTEXITCODE) { throw 'Source upload failed' }
$systemOption = if ($LoaderOnly) { '' } else { '--system /work/artifacts/system.bin' }
$kind = if ($LoaderOnly) { 'loader' } else { 'appliance' }
$name = "fractalcr-$kind-$FirmwarePolicy.img"
$script = @'
set -euo pipefail
root="$HOME/FractalCR-media/JOB"
tar -xzf "$root/source.tar.gz" -C "$root/source"
# Do not read artifacts while the supported builder is replacing them.
mkdir -p "$HOME/FractalCR-build"
exec 9>"$HOME/FractalCR-build/build.lock"
flock -s -n 9 || { echo 'Wait for the current image build to finish.' >&2; exit 1; }
doas -n docker run --rm --user "$(id -u):$(id -g)" --network none --cap-drop ALL --security-opt no-new-privileges \
 --mount type=bind,source="$root/source",target=/src,readonly \
 --mount type=bind,source="$root/output",target=/output \
 --mount type=volume,source=fractalcr-os-work,target=/work,readonly \
 fractalcr-os-builder python3 /src/os/tools/make-appliance.py \
 --efi /work/artifacts/rommon.efi SYSTEM_OPTION --output /output/IMAGE_NAME --size-mib SIZE_MIB --firmware-policy POLICY
cd "$root/output"
sha256sum IMAGE_NAME > IMAGE_NAME.sha256
gzip -n -1 -c IMAGE_NAME > IMAGE_NAME.gz
sha256sum IMAGE_NAME.gz > SHA256SUMS
doas -n docker run --rm --network none --cap-drop ALL --mount type=volume,source=fractalcr-os-work,target=/work,readonly \
 fractalcr-os-builder cat /work/artifacts/firmware.crt > firmware.crt
'@
$script = $script.Replace('JOB',$job).Replace('SYSTEM_OPTION',$systemOption).Replace('IMAGE_NAME',$name).Replace('SIZE_MIB',"$SizeMiB").Replace('POLICY',$FirmwarePolicy)
$script | & ssh -o BatchMode=yes $BuildHost 'tr -d "\r" | bash -s'
if ($LASTEXITCODE) { throw "Media creation failed; files retained at FractalCR-media/$job" }
foreach ($file in @("$name.gz", "$name.json", "$name.sha256", 'SHA256SUMS', 'firmware.crt')) {
    & scp -q "${BuildHost}:FractalCR-media/$job/output/$file" $OutputDirectory
    if ($LASTEXITCODE) { throw "Download failed: $file" }
}
Write-Host "Media exported to $OutputDirectory. Serial: COM1 / 115200 8N1."
