# -*- coding: utf-8 -*-
"""Cuantas razas de gato pasan las reglas de la lista de folios."""
import unicodedata
def sa(s):
    return ''.join(c for c in unicodedata.normalize('NFD', s) if unicodedata.category(c) != 'Mn')

# Razas reconocidas (TICA 73, CFA 45, FIFe, WCF) + preliminares/experimentales, en su nombre usual.
razas = """
Abisinio AmericanBobtail AmericanCurl AmericanShorthair AmericanWirehair Angora Aphrodite
Australian Balines Bambino Bengali Birmano Bombay Brasileno British Burmes Burmilla Californiano
Chartreux Chausie Cornish Cymric Devon Donskoy Dragonli Egipcio Europeo Exotico Foldex German
Habana Highlander Himalayo Japones Javanes Khaomanee Korat Kurilian LaPerm Levkoy Lykoi Maine
Mandalay Manx Mau Minskin Minuet Munchkin Nebelung Neva Noruego Ocicat Ojos Oriental Persa
Peterbald Pixiebob Ragamuffin Ragdoll Rusoazul Sabana Sagrado Scottish Selkirk Serengeti Siames
Siberiano Singapura Snowshoe Sokoke Somali Sphynx Suphalak Thai Tonkines Toyger Turco Ukrainiano
Ural Van Wila Yorkchocolate Serrade Kanaani Chantilly Cheetoh Aegean Anatolio Asiatico Bristol
Colorpoint Cyprus Kinkalow Genetta Napoleon Ojosazules Oregon Skookum Tennessee Tiffany
"""
pal = sorted(set(razas.split()))
print('razas listadas:', len(pal))
ok, largo, choque = [], [], []
pref = {}
for r in pal:
    if len(r) > 9:
        largo.append(r); continue
    k = sa(r)[:3].lower()
    if k in pref:
        choque.append((r, pref[k])); continue
    pref[k] = r
    ok.append(r)
print('pasan (<=9 letras, prefijo de 3 unico):', len(ok))
print(' ', ' '.join(ok))
print()
print('fuera por pasar de 9 letras (%d):' % len(largo), ' '.join(largo))
print()
print('fuera por chocar de prefijo (%d):' % len(choque), ', '.join('%s~%s' % c for c in choque))
