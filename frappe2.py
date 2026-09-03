# -*- coding: utf-8 -*-
import dev
t = dev.login()
st, m = dev.call('/pos/menu', t)
p = next(x for x in m['products'] if x['id'] == 630)
print('producto 630: %-12s base=%s' % (p['name'], p['price']))
print('  precio en Didi (excepcion):', m['platformPrices']['1'].get('630'))
mp = m['platformModPrices']['1']
print()
for g in p.get('groups', []):
    for o in g.get('options', []):
        if str(o['id']) in mp:
            print('  %-24s base=%-6s Didi=%s' % (o['name'], o['priceDelta'], mp[str(o['id'])]))
base = float(p['price'])
deltas = sum(float(v) for v in mp.values())
print()
print('LO QUE EL BOTON MUESTRA  = base %s + deltas de Didi %s = %s' % (base, deltas, base + deltas))
print('LO QUE EL SERVIDOR COBRA = precio Didi 100 + deltas de Didi %s = %s' % (deltas, 100 + deltas))
