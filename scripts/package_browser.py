#!/usr/bin/env python3
"""Developer packaging only: ship explicit static files, never installation data."""
import argparse
import hashlib
import pathlib
import tempfile
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
FILES = ['model.js', 'archive.js', 'app.js', 'app.css']

def build(output):
    output = pathlib.Path(output)
    output.mkdir(parents=True, exist_ok=True)
    name = 'CONTROL_BUCEADORES_NAVEGADOR_9_0_0.zip'
    entries = {}
    html = (ROOT/'browser-edition/ABRIR.html').read_text(encoding='utf-8')
    html = html.replace('href="assets/', 'href="NO TOCAR/assets/').replace('src="assets/', 'src="NO TOCAR/assets/')
    entries['CONTROL_BUCEADORES_NAVEGADOR/ABRIR.html'] = html.encode('utf-8')
    for f in FILES:
        entries['CONTROL_BUCEADORES_NAVEGADOR/NO TOCAR/assets/'+f] = (ROOT/'browser-edition/assets'/f).read_bytes()
    entries['CONTROL_BUCEADORES_NAVEGADOR/LEEME.txt'] = (ROOT/'browser-edition/LEEME.txt').read_bytes()
    path = output/name
    with zipfile.ZipFile(path, 'w', zipfile.ZIP_DEFLATED) as z:
        for n,b in sorted(entries.items()):
            assert n.endswith(('.html','.js','.css','.txt'))
            z.writestr(n,b)
    with zipfile.ZipFile(path) as z:
        assert z.testzip() is None
        assert not any(n.endswith(('.exe','.dll','.bat','.ps1')) for n in z.namelist())
        assert not any('/data/' in n or '/photos/' in n or '/backups/' in n for n in z.namelist())
    sha = hashlib.sha256(path.read_bytes()).hexdigest()
    (output/'SHA256SUMS_BROWSER.txt').write_text(sha+'  '+name+'\n', encoding='utf-8')
    print(path)
    print('SHA-256:',sha)
    return path

if __name__ == '__main__':
    p=argparse.ArgumentParser();p.add_argument('--out',default=str(ROOT/'release-output/browser'));a=p.parse_args();build(a.out)
