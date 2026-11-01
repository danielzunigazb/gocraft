#!/usr/bin/env python3
"""Keeps the README's progress table in step with docs/progress.json.

    python3 tools/progress.py             rewrite the table in README.md
    python3 tools/progress.py --check     exit 1 if README.md is not up to date (for CI)
    python3 tools/progress.py --snapshot  add today's total to the history, then rewrite

Edit the percentages in docs/progress.json when an area moves, run --snapshot, and commit both files.
"""
import datetime, json, pathlib, subprocess, sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
DATA, README = ROOT / 'docs' / 'progress.json', ROOT / 'README.md'
START, END = '<!-- progress:start -->', '<!-- progress:end -->'

def bar(p, n=20):
    k = round(p * n / 100)
    return '█' * k + '░' * (n - k)

def total(areas):
    return sum(a['weight'] * a['percent'] for a in areas) / sum(a['weight'] for a in areas)

def validate(d):
    w = sum(a['weight'] for a in d['areas'])
    if abs(w - 100) > 0.15:
        sys.exit(f'the weights add up to {w:.1f}, not 100')
    for a in d['areas']:
        if not 0 <= a['percent'] <= 100:
            sys.exit(f"{a['id']}: percent out of range")

def render(d):
    t = total(d['areas'])
    L = [START, '',
         f'**Avance global: {t:.0f} %** `{bar(t, 30)}`', '',
         '| Área | Peso | Avance | | Hecho | Pendiente |', '|---|--:|--:|---|---|---|']
    for a in d['areas']:
        L.append(f"| {a['name']} | {a['weight']:.1f} % | {a['percent']} % | `{bar(a['percent'], 10)}` | {a['done']} | {a['missing']} |")
    L += ['', d['method'], '']
    if d['history']:
        L += ['| Fecha | Commit | Avance global |', '|---|---|--:|']
        for h in reversed(d['history']):
            L.append(f"| {h['date']} | `{h['commit']}` | {h['total']:.1f} % |")
        L.append('')
    L.append(END)
    return '\n'.join(L)

def main():
    d = json.loads(DATA.read_text())
    validate(d)
    if '--snapshot' in sys.argv:
        commit = subprocess.run(['git', 'rev-parse', '--short', 'HEAD'], cwd=ROOT, capture_output=True, text=True).stdout.strip()
        today = datetime.date.today().isoformat()
        d['history'] = [h for h in d['history'] if h['date'] != today]
        d['history'].append({'date': today, 'commit': commit, 'total': round(total(d['areas']), 1),
                             'areas': {a['id']: a['percent'] for a in d['areas']}})
        DATA.write_text(json.dumps(d, ensure_ascii=False, indent=2) + '\n')
    s = README.read_text()
    i, j = s.find(START), s.find(END)
    if i < 0 or j < 0:
        sys.exit('README.md has no progress markers')
    new = s[:i] + render(d) + s[j + len(END):]
    if '--check' in sys.argv:
        sys.exit(0 if new == s else 'README.md is out of date: run tools/progress.py')
    README.write_text(new)

main()
