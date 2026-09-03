# -*- coding: utf-8 -*-
"""Razas de gato reconocidas, con su nombre real, medidas contra las reglas del folio."""
import unicodedata
def sa(s):
    return ''.join(c for c in unicodedata.normalize('NFD', s) if unicodedata.category(c) != 'Mn')

# Nombre tal como se usa en espanol. Una entrada por RAZA (no por sinonimo).
razas = [
 "Abisinio","Angora Turco","Azul Ruso","Balines","Bambino","Bengali","Birmano","Bombay",
 "Bosque de Noruega","British Shorthair","Burmes","Burmilla","Californiano","Chartreux","Chausie",
 "Cornish Rex","Cymric","Devon Rex","Don Sphynx","Exotico","Habana","Highlander","Himalayo",
 "Khao Manee","Korat","Kurilian Bobtail","LaPerm","Lykoi","Maine Coon","Manx","Mau Egipcio",
 "Minskin","Munchkin","Nebelung","Neva Masquerade","Ocicat","Ojos Azules","Oriental","Persa",
 "Peterbald","Pixie-bob","Ragamuffin","Ragdoll","Sagrado de Birmania","Savannah","Scottish Fold",
 "Selkirk Rex","Serengeti","Siames","Siberiano","Singapura","Snowshoe","Sokoke","Somali","Sphynx",
 "Thai","Tonkines","Toyger","Van Turco","American Bobtail","American Curl","American Shorthair",
 "American Wirehair","Aphrodite","Asiatico","Australian Mist","Bobtail Japones","Brasileno",
 "Chantilly-Tiffany","Cheetoh","Colorpoint Shorthair","Cyprus","Dragon Li","Europeo","Foldex",
 "German Rex","Javanes","Kanaani","Kinkalow","Levkoy Ucraniano","Lambkin","Mandalay","Minuet",
 "Napoleon","Oregon Rex","Sam Sawet","Serrade Petit","Skookum","Suphalak","Tennessee Rex",
 "Ural Rex","York Chocolate","Anatolio","Aegean","Bristol","Genetta","Sokoke Forest","Ussuri",
 "Chinchilla","Bengala","Ceylon","Kurilian","Mekong Bobtail","Nibelung",
]
razas = list(dict.fromkeys(razas))
print('razas listadas:', len(razas))

pref, ok, choque = {}, [], []
for r in razas:
    k = sa(r)[:3].lower()
    if k in pref:
        choque.append((r, pref[k])); continue
    pref[k] = r
    ok.append(r)

print('con prefijo de 3 letras unico:', len(ok))
print('largo maximo:', max(len(r) for r in ok), '->', max(ok, key=len))
por_largo = {}
for r in ok:
    por_largo.setdefault(len(r) > 12, []).append(r)
print('pasan de 12 caracteres:', len(por_largo.get(True, [])))
print()
print('CHOCAN DE PREFIJO (%d):' % len(choque))
for a, b in choque:
    print('  %-22s ~ %s' % (a, b))
print()
print('LISTA FINAL (%d):' % len(ok))
for i in range(0, len(ok), 4):
    print('  ' + '  |  '.join('%-22s' % r for r in ok[i:i+4]))
