#!/usr/bin/env python3
"""Print bounded runner diagnostics without process arguments or environment."""
import argparse
from pathlib import Path
import subprocess
import time


def cpu():
    return list(map(int, Path('/proc/stat').read_text().splitlines()[0].split()[1:9]))


def command(*args):
    try:
        result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                text=True, timeout=15, check=False)
        return result.stdout.strip()[:12000]
    except (OSError, subprocess.TimeoutExpired) as error:
        return str(error)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--interval', type=int, default=30)
    parser.add_argument('--samples', type=int, default=0)
    args = parser.parse_args()
    if args.interval < 1 or args.samples < 0:
        parser.error('interval must be positive and samples nonnegative')
    previous = cpu()
    count = 0
    while not args.samples or count < args.samples:
        time.sleep(args.interval)
        current = cpu()
        delta = [a-b for a, b in zip(current, previous)]
        previous = current
        total = max(sum(delta), 1)
        print(f'::group::Runner resources {time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}', flush=True)
        print(f'CPU busy={100*(total-delta[3]-delta[4])/total:.1f}% '
              f'iowait={100*delta[4]/total:.1f}% steal={100*delta[7]/total:.1f}%')
        for name in ('MemTotal', 'MemAvailable', 'SwapTotal', 'SwapFree'):
            print(next(line for line in Path('/proc/meminfo').read_text().splitlines() if line.startswith(name+':')))
        for name in ('cpu', 'memory', 'io'):
            path = Path('/proc/pressure') / name
            if path.exists():
                print(f'{name} pressure: {path.read_text().strip()}')
        print(command('df', '-h', '/'))
        print(command('docker', 'stats', '--no-stream', '--format',
                      '{{.Name}} CPU={{.CPUPerc}} memory={{.MemUsage}} IO={{.BlockIO}} PIDs={{.PIDs}}'))
        # comm only: command lines can contain secrets.
        print('\n'.join(command('ps', '-eo', 'pid,ppid,pcpu,pmem,stat,comm', '--sort=-pcpu').splitlines()[:16]))
        print('::endgroup::', flush=True)
        count += 1


if __name__ == '__main__':
    main()
