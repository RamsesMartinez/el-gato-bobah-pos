//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
)

// EL FAIL-CLOSED QUE `Store.QC` PROMETE, COMPROBADO EN LAS DOS FORMAS DE CONEXIÓN QUE HAY.
//
// El doc-comment de `Store.QC` dice que con el rol de app y sin conexión de tenant, RLS no devuelve
// nada — fail-closed en vez de filtrar entre empresas. Es la propiedad sobre la que se sostiene el
// modelo multi-tenant entero, y hasta hoy nada la comprobaba: el arnés fijaba el default de empresa
// a nivel BASE, así que toda conexión del rol de app heredaba «empresa 1» y la promesa nunca se
// ponía a prueba.
//
// Medido contra Postgres 16 antes de escribir esto, y es la razón de que haya DOS casos:
//
//	conexión virgen     current_setting('app.company_id', true) → NULL   → el cast da NULL → 0 filas
//	conexión reciclada  el mismo setting                        → ''     → el cast REVIENTA (22P02)
//
// El pool recicla, y `AcquireTenant` hace `reset app.company_id` al soltar. Así que la segunda
// forma es la NORMAL en producción, no el borde.

// TestElRolDeAppNoVeNadaSinEmpresa recorre TODA tabla con company_id.
//
// Recorrerlas todas y no una lista escrita a mano es lo que hace que cubra la tabla que alguien
// agregue mañana. Una lista se queda corta en silencio: es el mismo defecto que ya se pagó con la
// enumeración incompleta del anonimizador.
func TestElRolDeAppNoVeNadaSinEmpresa(t *testing.T) {
	st := newTestStore(t)
	appSt := appRoleStore(t)
	ctx := context.Background()

	filas, err := st.Pool.Query(ctx, `
		select c.table_name
		  from information_schema.columns c
		  join information_schema.tables tb
		    on tb.table_name = c.table_name and tb.table_schema = c.table_schema
		 where c.column_name = 'company_id' and c.table_schema = 'public'
		   and tb.table_type = 'BASE TABLE'
		 order by c.table_name`)
	if err != nil {
		t.Fatalf("listar tablas con company_id: %v", err)
	}
	defer filas.Close()
	var tablas []string
	for filas.Next() {
		var n string
		if err := filas.Scan(&n); err != nil {
			t.Fatal(err)
		}
		tablas = append(tablas, n)
	}
	if len(tablas) < 20 {
		t.Fatalf("solo %d tablas con company_id: la consulta que las busca dejó de funcionar", len(tablas))
	}

	var fugas []string
	for _, tabla := range tablas {
		var n int
		// pgx no interpola identificadores; la tabla viene del catálogo de Postgres, no de fuera.
		err := appSt.Pool.QueryRow(ctx, `select count(*) from "`+tabla+`"`).Scan(&n)
		if err != nil {
			// Un error de permiso es OTRA cosa y también hay que verla: significa que falta un
			// grant, no que RLS esté cerrando. Se distingue en el mensaje.
			fugas = append(fugas, tabla+" (error: "+err.Error()+")")
			continue
		}
		if n > 0 {
			fugas = append(fugas, tabla+" ("+itoa(n)+" filas)")
		}
	}
	if len(fugas) > 0 {
		t.Fatalf("sin empresa fijada, el rol de la aplicación alcanzó %d tablas: %s\n"+
			"RLS tiene que fallar CERRADO: es la propiedad sobre la que se sostiene todo el modelo "+
			"multi-tenant, y lo que hace que un `store.Q` olvidado devuelva cero filas en vez de las "+
			"de otra empresa", len(fugas), strings.Join(fugas, ", "))
	}
}

// TestUnaConexionRecicladaSigueCerrada es el caso que el otro NO puede ver.
//
// Una conexión virgen nunca tuvo el GUC, así que `current_setting(..., true)` da NULL y el cast a
// bigint da NULL: RLS compara contra NULL y no devuelve nada. Correcto.
//
// Pero el pool RECICLA. `AcquireTenant` fija el GUC y su release hace `reset`, y tras un reset el
// valor NO vuelve a NULL: vuelve a CADENA VACÍA. `”::bigint` no es NULL, es un error 22P02.
//
// O sea: la misma consulta, en la misma aplicación, falla de dos maneras distintas según le toque
// una conexión nueva o una reusada. En el webhook eso se traduce en un 401 unas veces y un 502
// otras, y la plataforma reintenta distinto ante cada uno.
func TestUnaConexionRecicladaSigueCerrada(t *testing.T) {
	st := newTestStore(t)
	appSt := appRoleStore(t)
	ctx := context.Background()

	empresa := makeCompany(t, st, "empresa-reciclada")

	// Se toma la conexión, se fija la empresa y se suelta: es exactamente lo que hace un request
	// que sí tiene sesión.
	ctxT, soltar, err := appSt.AcquireTenant(ctx, empresa)
	if err != nil {
		t.Fatalf("tomar la conexión de la empresa: %v", err)
	}
	_ = ctxT
	soltar()

	// Y ahora se vuelve a pedir del pool, que con toda probabilidad devuelve LA MISMA.
	conn, err := appSt.Pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("volver a tomar del pool: %v", err)
	}
	defer conn.Release()

	var valor *string
	if err := conn.QueryRow(ctx, `select current_setting('app.company_id', true)`).Scan(&valor); err != nil {
		t.Fatalf("leer el ajuste: %v", err)
	}

	var n int
	err = conn.QueryRow(ctx, `select count(*) from companies`).Scan(&n)
	if err != nil {
		t.Fatalf("una conexión RECICLADA no falla cerrada, revienta: %v\n"+
			"El ajuste vale %q después del reset —cadena vacía, no NULL— y el cast a bigint de la "+
			"política no lo tolera. `Store.QC` promete fail-closed y esto lo desmiente. Producción "+
			"es el único ambiente expuesto: las bases locales y las restauradas traen un default a "+
			"nivel base que lo tapa", err, derefOVacio(valor))
	}
	if n != 0 {
		t.Fatalf("una conexión reciclada vio %d empresas: el reset no limpió el tenant", n)
	}
}

func derefOVacio(p *string) string {
	if p == nil {
		return "<NULL>"
	}
	return *p
}
