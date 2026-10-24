"""IOS interactive workflows shared by disposable-VM qualifications."""


def reload_appliance(terminal, save=False):
    terminal.sendline('reload')
    prompt = terminal.expect_exact([
        'Proceed with reload? [confirm]',
        'System configuration has been modified. Save? [yes/no]: ',
    ])
    if prompt == 1:
        terminal.sendline('yes' if save else 'no')
        terminal.expect_exact('Proceed with reload? [confirm]')
    terminal.sendline('')
