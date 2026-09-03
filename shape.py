# -*- coding: utf-8 -*-
import json, dev
t = dev.login()
st, m = dev.call('/pos/menu', t)
print('llaves del menu:', list(m.keys()))
for k in ('platforms', 'platformPrices', 'platformModPrices'):
    v = m.get(k)
    if isinstance(v, dict):
        print(k, '-> dict con llaves', list(v.keys())[:5])
        for kk in list(v.keys())[:1]:
            print('   ', kk, '->', json.dumps(v[kk])[:200])
    elif isinstance(v, list):
        print(k, '-> lista:', json.dumps(v)[:300])
