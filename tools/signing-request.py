#!/usr/bin/env python3
"""Describe unsigned outputs without reading any private signing material."""
import argparse,hashlib,json
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--artifacts',type=Path,required=True);p.add_argument('--trust',type=Path,required=True);p.add_argument('--stage',choices=['boot','os'],required=True)
a=p.parse_args()
names=['rommon.unsigned.efi','witness.unsigned.efi'] if a.stage=='boot' else ['system.unsigned.efi']
request=dict(schema=1,stage=a.stage,status='pending-offline-signature',trust=json.loads((a.trust/'trust.json').read_text()),
             inputs={name:hashlib.sha256((a.artifacts/name).read_bytes()).hexdigest() for name in names})
(a.artifacts/'signing-request.json').write_text(json.dumps(request,indent=2)+'\n')
print(json.dumps(request))
