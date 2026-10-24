[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$fixture = Join-Path $root ('out\launcher-tests\' + [guid]::NewGuid().ToString('N'))
$artifacts = Join-Path $fixture 'artifacts'
$share = Join-Path $fixture 'share'
New-Item -ItemType Directory -Path $artifacts, $share -Force | Out-Null
$qemu = Join-Path $fixture 'qemu-stub.cmd'
Set-Content -LiteralPath $qemu -Encoding ASCII -Value "@echo off`r`necho FAKE_QEMU_STARTED`r`nexit /b 0"
foreach ($name in @('edk2-x86_64-code.fd', 'edk2-i386-vars.fd')) {
    Set-Content -LiteralPath (Join-Path $share $name) -Value 'firmware fixture'
}
$files = @{}
foreach ($name in @('system.bin', 'flash-os-optional-firmware.img', 'data-os-containers.img')) {
    $path = Join-Path $artifacts $name
    Set-Content -LiteralPath $path -Value ('prepared ' + $name)
    $files[$name] = (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash
}
@{schema=1; files=$files} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $artifacts 'launcher-manifest.json')
$flash = Join-Path $artifacts 'flash-os-interactive-nosb.img'
$data = Join-Path $artifacts 'data-os-interactive.img'
$variables = Join-Path $artifacts 'OVMF_VARS-os-interactive-whpx.fd'
Set-Content -LiteralPath $flash -Value 'older installed OS'
Set-Content -LiteralPath $data -Value 'saved user configuration and containers'
Set-Content -LiteralPath $variables -Value 'saved firmware variables'
$original = @{}
foreach ($path in @($flash, $data, $variables)) { $original[$path] = (Get-FileHash -LiteralPath $path).Hash }
function Assert-True($Condition, $Message) {
    if (-not $Condition) { throw $Message }
}
function Invoke-Launcher([string[]]$Options) {
    # Exercise Windows PowerShell's actual parameter binder and script, while
    # keeping QEMU and all guest storage confined to this disposable fixture.
    $ErrorActionPreference = 'Continue' # Capture expected native stderr failures on PowerShell 5.1.
    $output = & powershell.exe -NoProfile -File (Join-Path $root 'os\run.ps1') -Qemu $qemu -ArtifactDirectory $artifacts -NoSecureBoot @Options 2>&1
    @{Code=$LASTEXITCODE; Text=($output | Out-String)}
}
$result = Invoke-Launcher @('-acceleratopr', 'whpx')
Assert-True ($result.Code -ne 0 -and $result.Text -notmatch 'FAKE_QEMU_STARTED') 'Unknown switch launched QEMU'
Assert-True ($result.Text -match 'acceleratopr') 'Unknown switch was not identified'
Assert-True ((Get-FileHash -LiteralPath $flash).Hash -eq $original[$flash]) 'Rejected invocation changed bootflash'
Write-Output 'PASS unknown switches are rejected before launch or storage mutation'
$result = Invoke-Launcher @('-Accelerator', 'whpx')
Assert-True ($result.Code -eq 0 -and $result.Text -match 'FAKE_QEMU_STARTED') $result.Text
Assert-True ($result.Text -match 'Reusing existing bootflash' -and $result.Text -match '-UpdateImage') 'Older bootflash was reused silently'
Assert-True ((Get-FileHash -LiteralPath $flash).Hash -eq $original[$flash]) 'Launch silently updated bootflash'
Write-Output 'PASS differing bootflash is identified without silently overwriting it'
$result = Invoke-Launcher @('-Accelerator', 'whpx', '-UpdateImage')
Assert-True ($result.Code -eq 0 -and $result.Text -match 'FAKE_QEMU_STARTED') $result.Text
Assert-True ((Get-FileHash -LiteralPath $flash).Hash -eq $files['flash-os-optional-firmware.img']) 'Update did not install the prepared flash'
foreach ($path in @($data, $variables)) {
    Assert-True ((Get-FileHash -LiteralPath $path).Hash -eq $original[$path]) 'Image update changed persistent user state'
}
Write-Output 'PASS UpdateImage installs current flash and preserves datastore and firmware variables'
$result = Invoke-Launcher @()
Assert-True ($result.Code -eq 0 -and $result.Text -notmatch 'Reusing existing bootflash') 'Matching flash reported as different'
Set-Content -LiteralPath (Join-Path $artifacts 'flash-os-optional-firmware.img') -Value 'damaged template'
$result = Invoke-Launcher @('-UpdateImage')
Assert-True ($result.Code -ne 0 -and $result.Text -notmatch 'FAKE_QEMU_STARTED') 'Damaged template launched QEMU'
Assert-True ((Get-FileHash -LiteralPath $flash).Hash -eq $files['flash-os-optional-firmware.img']) 'Damaged template replaced installed flash'
Write-Output 'PASS matching flash stays quiet and damaged templates cannot replace installed flash'
Write-Output "Fixtures retained at $fixture"
