"""Reject low-level kernel/daemon output in the normal appliance transcript."""
import re

def assert_quiet_boot(text):
    forbidden = (
        r"Linux version \d", r"Kernel command line:", r"BIOS-e820", r"BTRFS(?: info|:)",
        r"(?:^|[\r\n])(?:ACPI:|PCI:|NET:|usbcore:|clocksource:|scsi host|ata\d|e1000:)",
        r"Freeing unused kernel", r"Run /init as init process", r"docker0: port",
        r'time="[^"\n]+" level=', r"FRACTALCR_OS_(?:READY|DIAGNOSTIC)",
    )
    for pattern in forbidden:
        match = re.search(pattern, text)
        assert not match, f"Low-level output leaked to console: {match.group(0)}"
