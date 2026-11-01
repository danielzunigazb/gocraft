#!/usr/bin/env python3
"""Reads a gocraft -netlog of a PvP session and says what went wrong in it.

    python3 tools/pvpreport.py /tmp/pvp.net

It needs a log made by a recent gocraft (it reads the decoded packets and the client's own "#" notes). It tells:
how often the server moved the player and what came before (a blow, a knockback), which of the player's attacks the
server accepted and why the others did not (the rival's invulnerability or the reach), whether swings that sent
nothing had a rival within reach, and the frames that took long.
"""
import collections, math, re, sys

def load(path):
    P = []
    for l in open(path, errors='replace').read().split('\n'):
        m = re.match(r'\s*(\d+)ms ([<>#]) (?:0x([0-9a-f]{2}) )?(\S.*)', l)
        if m:
            P.append((int(m.group(1)), m.group(2), int(m.group(3), 16) if m.group(3) else -1, m.group(4)))
    return P

def main(path):
    P = load(path)
    dur = P[-1][0] / 1000
    ids = collections.Counter(re.search(r'entity (-?\d+) action', p[3]).group(1)
                              for p in P if p[1] == '>' and p[2] == 0x0b and 'action' in p[3])
    me = next((k for k, _ in ids.most_common() if k != '-100'), '0')
    attacks = [p[0] for p in P if p[1] == '>' and p[2] == 2]
    swings = [p[0] for p in P if p[1] == '>' and p[2] == 0x0a]
    kb = [p[0] for p in P if p[1] == '<' and p[2] == 0x12 and ('entity %s ' % me) in p[3]]
    status = [(p[0], re.search(r'entity (\d+) status (\d+)', p[3]).groups())
              for p in P if p[1] == '<' and p[2] == 0x1a and re.search(r'entity (\d+) status', p[3])]
    print('== %s: %.0f s, %d swings, %d attacks sent, %d knockbacks received' % (path, dur, len(swings), len(attacks), len(kb)))

    # corrections: where the server put the player against the last position the client sent
    last = None
    rows = []
    for p in P:
        if p[1] == '>' and p[2] in (4, 6):
            m = re.search(r'pos ([-\d.]+) ([-\d.]+) ([-\d.]+).*ground=(\w+)', p[3])
            if m:
                last = (float(m.group(1)), float(m.group(2)), float(m.group(3)), m.group(4))
        if p[1] == '<' and p[2] == 8 and p[0] > 3000 and last:
            m = re.search(r'to ([-\d.]+) ([-\d.]+) ([-\d.]+)', p[3])
            if m:
                x = tuple(map(float, m.groups()))
                rows.append((p[0], math.dist(x, last[:3]), x, last[3]))
    small = [r for r in rows if r[1] < 2]
    print('\n-- the server moved the player: %d times (%.1f a minute); %d of them moved it 2 blocks or more (a plugin: spawn, arena)'
          % (len(rows), len(rows) / dur * 60, len(rows) - len(small)))
    print('   small corrections: %d (%.1f a minute)' % (len(small), len(small) / dur * 60))
    c = collections.Counter()
    for t, d, x, g in small:
        h = any(0 < t - a <= 400 for a in attacks)
        k = any(0 < t - a <= 400 for a in kb)
        c[('after the player hit ' if h else '') + ('after knockback' if k else '') or 'neither']+= 1
    for k, v in c.most_common():
        print('     %-34s %d' % (k.strip(), v))
    hit_tp = sum(1 for a in attacks if any(0 < t - a <= 400 for t, *_ in small))
    if attacks:
        print('   attacks followed by a correction within 400 ms: %d of %d' % (hit_tp, len(attacks)))
    fixed = collections.Counter((round(x[0], 1), round(x[1], 1), round(x[2], 1)) for _, _, x, _ in small)
    for k, v in fixed.most_common(3):
        if v > 3:
            print('   %d corrections to the same point %s: an invisible limit (the arena)' % (v, k))
    print('   per minute:', sorted(collections.Counter(t // 60000 for t, *_ in small).items()))

    # attacks: did the server take them?
    uses = [(p[0], re.search(r'entity (\d+) use', p[3]).group(1)) for p in P
            if p[1] == '>' and p[2] == 2 and re.search(r'entity (\d+) use', p[3])]
    if uses:
        ok = [any(e == s[1][0] and s[1][1] == '2' and 0 <= s[0] - t <= 300 for s in status) for t, e in uses]
        print('\n-- attacks: the server hurt the target after %d of %d (%.0f%%)' % (sum(ok), len(uses), 100 * sum(ok) / len(uses)))
        gaps = [uses[i][0] - uses[i - 1][0] for i in range(1, len(uses)) if not ok[i]]
        quick = sum(1 for g in gaps if g < 500)
        print('   of the %d it did not, %d came less than 500 ms after the previous attack (the rival\'s invulnerability)' % (len(gaps) + (0 if ok[0] else 1), quick))

    # the client's own notes
    sw = []
    for p in P:
        if p[1] == '#' and p[3].startswith('swing hit='):
            m = re.match(r'swing hit=(\w+) \(target (\d+)\) nearest=(\d+)\((.*?)\) eyeToBox=([\d.]+) ray=([-\d.]+) feet=([\d.]+) \(dy ([-+\d.]+)\) drawn ([\d.]+)', p[3])
            if m:
                sw.append((p[0], m.group(1) == 'true', float(m.group(5)), float(m.group(6)), float(m.group(7)), float(m.group(9))))
            elif 'nobody' in p[3]:
                sw.append((p[0], p[3].startswith('swing hit=true'), None, None, None, None))
    if sw:
        found = [s for s in sw if s[2] is not None]
        miss = [s for s in found if not s[1]]
        print('\n-- swings: %d with a note; %d found a target; %d did not' % (len(sw), len(sw) - len(miss) - len([s for s in sw if s[2] is None]), len(miss)))
        print('   no rival within 10 blocks: %d' % len([s for s in sw if s[2] is None]))
        within = [s for s in miss if 0 <= s[3] <= 3.0]
        print('   !! swings that found nothing while the view\'s ray met the rival within 3.0 blocks: %d  <- a bug if not 0' % len(within))
        for s in within[:8]:
            print('      %dms eyeToBox %.2f ray %.2f feet %.2f drawn %.2f behind' % (s[0], s[2], s[3], s[4], s[5]))
        near = [s for s in miss if s[3] > 3.0 and s[3] <= 4.0]
        print('   misses with the rival 3.0 to 4.0 blocks along the view (just out of reach): %d' % len(near))
        off = [s for s in miss if s[3] < 0 and s[2] <= 3.0]
        print('   misses with the rival within 3.0 of the eyes but the view pointing off their box: %d (aim)' % len(off))
        lag = [s[5] for s in found]
        if lag:
            lag.sort()
            print('   how far the rival is drawn behind the server\'s position: median %.2f, p90 %.2f, max %.2f blocks' % (lag[len(lag) // 2], lag[int(len(lag) * .9)], lag[-1]))

    hitch = [p for p in P if p[1] == '#' and p[3].startswith('hitch')]
    print('\n-- frames that took long (over 40 ms of work): %d' % len(hitch))
    for p in hitch[:8]:
        print('   %dms %s' % (p[0], p[3]))

if __name__ == '__main__':
    main(sys.argv[1] if len(sys.argv) > 1 else '/tmp/pvp.net')
