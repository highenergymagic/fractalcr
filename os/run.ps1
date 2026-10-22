param([string]$Qemu = "qemu-system-x86_64", [switch]$Fresh, [switch]$UpdateImage, [switch]$NoSecureBoot, [string]$ArtifactDirectory,
    [ValidateSet("auto", "whpx", "tcg")][string]$Accelerator = "auto",
    [ValidateRange(0,65535)][int]$SshPort = 0)
$ErrorActionPreference = "Stop"
$artifacts = if ($ArtifactDirectory) { $ArtifactDirectory } else {
    Join-Path (Split-Path -Parent $PSScriptRoot) "out\carbon\os"
}
$flash = Join-Path $artifacts "flash-os-interactive.img"
$data = Join-Path $artifacts "data-os-interactive.img"
$variables = Join-Path $artifacts "OVMF_VARS-os-interactive.fd"
$firmware = Join-Path $artifacts "OVMF_CODE_4M.secboot.fd"
$qemuExecutable = (Get-Command $Qemu -CommandType Application -ErrorAction Stop).Source
if ($Accelerator -eq "auto") {
    $Accelerator = if ($NoSecureBoot) { "whpx" } else { "tcg" }
}
if ($Accelerator -eq "whpx" -and -not $NoSecureBoot) {
    throw "WHPX cannot run this appliance's SMM Secure Boot firmware. Use -NoSecureBoot with WHPX, or -Accelerator tcg for Secure Boot."
}
$machine = "q35,smm=on"
$acceleratorOptions = $Accelerator
$cpu = "max"
$firmwareOptions = @("-global", "driver=cfi.pflash01,property=secure,value=on")
if ($Accelerator -eq "whpx") {
    $machine = "q35,smm=off"
    # host/max expose features that fault in PlatformPei on this WHPX build.
    $cpu = "qemu64"
    $firmwareOptions = @()
    $firmware = Join-Path (Split-Path -Parent $qemuExecutable) "share\edk2-x86_64-code.fd"
}
if (-not (Test-Path -LiteralPath $firmware)) { throw "Missing UEFI firmware: $firmware" }
$flashTemplate = "flash-os-containers.img"
$varsTemplate = "OVMF_VARS-trusted.fd"
if ($NoSecureBoot) {
    $flash = Join-Path $artifacts "flash-os-interactive-nosb.img"
    $variables = Join-Path $artifacts "OVMF_VARS-os-interactive-nosb.fd"
    $flashTemplate = "flash-os-optional-firmware.img"
    $varsTemplate = "OVMF_VARS-unenrolled.fd"
}
if ($Accelerator -eq "whpx") {
    # Keep the non-SMM firmware's variable store separate from the TCG firmware.
    $variables = Join-Path $artifacts "OVMF_VARS-os-interactive-whpx.fd"
    $varsTemplate = Join-Path (Split-Path -Parent $qemuExecutable) "share\edk2-i386-vars.fd"
}
foreach ($pair in @(
    @($flashTemplate, $flash),
    @("data-os-containers.img", $data),
    @($varsTemplate, $variables)
)) {
    $source = if ([IO.Path]::IsPathRooted($pair[0])) { $pair[0] } else { Join-Path $artifacts $pair[0] }
    if (-not (Test-Path -LiteralPath $source)) { throw "Missing $source. Run .\os\build.ps1 first." }
    if ($Fresh -or ($UpdateImage -and $pair[1] -eq $flash) -or -not (Test-Path -LiteralPath $pair[1])) {
        Copy-Item -LiteralPath $source -Destination $pair[1] -Force
    }
}
Write-Host "FractalCR appliance VM. Serial only; Ctrl-A X exits. Changes persist."
if ($Accelerator -eq "tcg") {
    Write-Host "CPU: QEMU software emulation (TCG). Allow up to 10 minutes for boot."
} else {
    Write-Host "CPU: Windows Hypervisor Platform (WHPX). Firmware Secure Boot disabled; image signatures mandatory."
}
Write-Host "After image verification, kernel authentication/loading is silent until 'Booting authenticated system image...'."
$network = "user,id=net0"
if ($SshPort) {
    $network += ",hostfwd=tcp:127.0.0.1:${SshPort}-:22"
    Write-Host "SSH forwarding: 127.0.0.1:$SshPort -> guest TCP 22. Enable local AAA and SSH in the appliance first."
}
& $qemuExecutable -machine $machine -accel $acceleratorOptions -cpu $cpu -m 4096 `
    @firmwareOptions `
    -drive "if=pflash,format=raw,unit=0,readonly=on,file=$firmware" `
    -drive "if=pflash,format=raw,unit=1,file=$variables" `
    -drive "if=virtio,format=raw,file=$flash" `
    -drive "if=virtio,format=raw,file=$data" `
    -netdev $network -device virtio-net-pci,netdev=net0 `
    -display none -serial mon:stdio
if ($LASTEXITCODE -ne 0) { throw "QEMU exited with code $LASTEXITCODE" }
