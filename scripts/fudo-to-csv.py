#!/usr/bin/env python3
"""Convierte los exports .xls/.xlsx de FUDO (references/) a CSV limpios en references/csv/.
El importador Go lee esos CSV. Corre con: uv run --with pandas --with xlrd --with openpyxl python3 scripts/fudo-to-csv.py
"""
import sys
from pathlib import Path
import pandas as pd

ROOT = Path(__file__).resolve().parent.parent
REF = ROOT / "references"
OUT = REF / "csv"
OUT.mkdir(exist_ok=True)

# archivo -> {hoja: nombre_csv}
JOBS = {
    "productos.xls": {
        "Productos": "productos",
        "Recetas": "recetas",
        "Subproductos": "subproductos",
        "Modificadores - Grupos": "mod_grupos",
        "Modificadores - Productos": "mod_productos",
    },
    "ingredientes.xls": {
        "Ingredientes": "ingredientes",
        "Subingredientes": "subingredientes",
    },
    "stock.xls": {"Productos": "stock_productos", "Ingredientes": "stock_ingredientes"},
}

def main():
    total = 0
    for fname, sheets in JOBS.items():
        path = REF / fname
        if not path.exists():
            print(f"  (falta {fname}, se omite)")
            continue
        book = pd.read_excel(path, sheet_name=None)
        for sheet, out_name in sheets.items():
            if sheet not in book:
                print(f"  (hoja '{sheet}' no está en {fname})")
                continue
            df = book[sheet]
            dest = OUT / f"{out_name}.csv"
            df.to_csv(dest, index=False)
            print(f"  {fname}::{sheet} -> {dest.name} ({len(df)} filas)")
            total += 1
    print(f"Listo: {total} CSV en {OUT}")

if __name__ == "__main__":
    sys.exit(main())
