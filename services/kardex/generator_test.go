package kardex

import (
	"strings"
	"testing"

	"github.com/jung-kurt/gofpdf"
	"kardex-pdf-service/internal/config"
	"kardex-pdf-service/internal/tracker"
)

func TestCalculateMetrics(t *testing.T) {
	cfg := config.Default()
	gen := NewGenerator(Institution{RazonSocial: "Test", RUC: "12345678"}, "Periodo: Test", cfg, "L", "simple", false)

	insumos := []KardexInsumo{
		{
			Kardex: KardexData{
				Movimientos: make([]KardexMovimiento, 5),
			},
		},
		{
			Kardex: KardexData{
				Movimientos: make([]KardexMovimiento, 3),
			},
		},
		{
			Kardex: KardexData{
				Movimientos: []KardexMovimiento{}, // 0 movimientos
			},
		},
	}

	metrics := gen.CalculateMetrics(insumos)

	if metrics.TotalItems != 3 {
		t.Errorf("TotalItems = %d, want 3", metrics.TotalItems)
	}
	if metrics.TotalMovements != 8 {
		t.Errorf("TotalMovements = %d, want 8", metrics.TotalMovements)
	}
}

func TestCalculateMetricsEmpty(t *testing.T) {
	cfg := config.Default()
	gen := NewGenerator(Institution{}, "", cfg, "L", "simple", false)

	metrics := gen.CalculateMetrics([]KardexInsumo{})

	if metrics.TotalItems != 0 {
		t.Errorf("TotalItems = %d, want 0", metrics.TotalItems)
	}
	if metrics.TotalMovements != 0 {
		t.Errorf("TotalMovements = %d, want 0", metrics.TotalMovements)
	}
}

func TestCreateOptimizedBatches(t *testing.T) {
	cfg := config.Default()
	cfg.ItemsPerBatch = 2 // 2 insumos por batch para test

	gen := NewGenerator(Institution{}, "", cfg, "L", "", false)

	insumos := []KardexInsumo{
		{Kardex: KardexData{Movimientos: make([]KardexMovimiento, 10)}},
		{Kardex: KardexData{Movimientos: make([]KardexMovimiento, 20)}},
		{Kardex: KardexData{Movimientos: make([]KardexMovimiento, 30)}},
		{Kardex: KardexData{Movimientos: make([]KardexMovimiento, 40)}},
		{Kardex: KardexData{Movimientos: make([]KardexMovimiento, 50)}},
	}

	batches := gen.createOptimizedBatches(insumos)

	// 5 insumos / 2 per batch = 3 batches (2, 2, 1)
	if len(batches) != 3 {
		t.Errorf("Expected 3 batches, got %d", len(batches))
	}
	if len(batches[0]) != 2 {
		t.Errorf("Batch 0 should have 2 items, got %d", len(batches[0]))
	}
	if len(batches[1]) != 2 {
		t.Errorf("Batch 1 should have 2 items, got %d", len(batches[1]))
	}
	if len(batches[2]) != 1 {
		t.Errorf("Batch 2 should have 1 item, got %d", len(batches[2]))
	}
}

func TestCreateOptimizedBatchesMovementLimit(t *testing.T) {
	cfg := config.Default()
	cfg.ItemsPerBatch = 100
	cfg.MaxMovementsPerBatch = 50

	gen := NewGenerator(Institution{}, "", cfg, "L", "", false)

	insumos := []KardexInsumo{
		{Kardex: KardexData{Movimientos: make([]KardexMovimiento, 30)}},
		{Kardex: KardexData{Movimientos: make([]KardexMovimiento, 30)}}, // 60 total > 50, split
		{Kardex: KardexData{Movimientos: make([]KardexMovimiento, 10)}},
	}

	batches := gen.createOptimizedBatches(insumos)

	// First batch: insumo 0 (30 mov) — fits (empty batch)
	// Second batch: insumo 1 (30 mov) — empty batch, fits alone
	// Still in batch 2: insumo 2 (10 mov) — 30+10=40 < 50, fits
	// Result: 2 batches [0], [1, 2]
	if len(batches) != 2 {
		t.Errorf("Expected 2 batches (movement limit), got %d", len(batches))
	}
}

func TestTotalPagesInitiallyZero(t *testing.T) {
	cfg := config.Default()
	gen := NewGenerator(Institution{}, "", cfg, "L", "", false)

	if gen.TotalPages() != 0 {
		t.Errorf("TotalPages() = %d, want 0 initially", gen.TotalPages())
	}
}

// generateSingleFile necesita filesystem — ese es test de integración (test-go-service.sh)
// El filtro de reportTypes de generate-all: subconjunto, todos y nil
func TestFiltrarFormatsPorReportTypes(t *testing.T) {
	if got := filtrarFormatsPorReportTypes(defaultFormats, []string{"sunat_13_1"}); len(got) != 1 || got[0].ReportType != "sunat_13_1" {
		t.Fatalf("solo sunat_13_1: got %v", got)
	}
	if got := filtrarFormatsPorReportTypes(defaultFormats, []string{"completo", "sunat_13_1"}); len(got) != 2 {
		t.Fatalf("completo + sunat_13_1: got %d formatos", len(got))
	}
	if got := filtrarFormatsPorReportTypes(defaultFormats, nil); len(got) != len(defaultFormats) {
		t.Fatalf("nil = todos: got %d", len(got))
	}
	if got := filtrarFormatsPorReportTypes(defaultFormats, []string{"inexistente"}); len(got) != 0 {
		t.Fatalf("inexistente = vacío: got %d", len(got))
	}
}

// fitCellText: no trunca si cabe; trunca con '...' y cabe en el ancho si no
func TestFitCellText(t *testing.T) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetFont("Arial", "", 7)

	corto := "ABC"
	if got := fitCellText(pdf, corto, 50); got != corto {
		t.Fatalf("cabe → sin truncar, got %q", got)
	}

	largo := strings.Repeat("X", 200)
	got := fitCellText(pdf, largo, 50)
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("debe terminar en '...', got %q", got[len(got)-10:])
	}
	if w := pdf.GetStringWidth(got); w > 50 {
		t.Fatalf("truncado debe caber: %.2fmm > 50mm", w)
	}
}

func TestFormatFechaSunat(t *testing.T) {
	if got := formatFechaSunat("2026-03-07"); got != "07/03/2026" {
		t.Fatalf("got %q", got)
	}
	if got := formatFechaSunat("julio"); got != "julio" {
		t.Fatalf("passthrough: got %q", got)
	}
}

// Tabla 12 SUNAT: códigos de tipo de operación del 13.1
func TestTablaOperacionSunatCode(t *testing.T) {
	casos := map[string]string{
		"venta":        "01",
		"compra":       "02",
		"traslado":     "11",
		"nota":         "99",
		"devolucion":   "06",
		"desconocido":  "99",
	}
	for tipo, want := range casos {
		if got := tablaOperacionSunatCode(tipo); got != want {
			t.Errorf("%s → got %s, want %s", tipo, got, want)
		}
	}
}

// config referenciada por los tests existentes
var _ = config.Default()

// T31: Tabla 12 code derived from the movement (direction/document), not only tipo_operacion
func TestSunatOperationCode(t *testing.T) {
	casos := []struct {
		name string
		m    KardexMovimiento
		want string
	}{
		{"purchase tipo 00 empty op", KardexMovimiento{Asunto: "compra", Fuente: "compra", TipoDocumento: "00"}, "02"},
		{"purchase with catalog-51 op 0101", KardexMovimiento{Asunto: "compra", Fuente: "compra", TipoDocumento: "01", TipoOperacion: "0101"}, "02"},
		{"sale op 01", KardexMovimiento{Asunto: "salida", Fuente: "salida", TipoDocumento: "03", TipoOperacion: "01"}, "01"},
		{"sale empty op", KardexMovimiento{Asunto: "salida", Fuente: "salida", TipoDocumento: "00"}, "01"},
		{"sale credit note", KardexMovimiento{Asunto: "salida", Fuente: "salida", TipoDocumento: "07", TipoOperacion: "01"}, "05"},
		{"purchase credit note", KardexMovimiento{Asunto: "compra", Fuente: "compra", TipoDocumento: "07"}, "06"},
		{"transfer in", KardexMovimiento{Asunto: "ingreso", Fuente: "traslado", TipoDocumento: "TRA"}, "11"},
		{"transfer out", KardexMovimiento{Asunto: "salida", Fuente: "traslado", TipoDocumento: "TRA"}, "11"},
		{"initial inventory", KardexMovimiento{Asunto: "compra", Fuente: "compra", TipoDocumento: "INV"}, "16"},
		{"legacy word op", KardexMovimiento{Asunto: "salida", TipoOperacion: "venta"}, "01"},
		{"inconsistent op 01 on entry", KardexMovimiento{Asunto: "compra", TipoDocumento: "01", TipoOperacion: "01"}, "02"},
	}
	for _, c := range casos {
		if got := sunatOperationCode(c.m); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestOperationLabel(t *testing.T) {
	if got := operationLabel(KardexMovimiento{Asunto: "compra", TipoDocumento: "00"}); got != "02-COMPRA" {
		t.Errorf("purchase: got %q", got)
	}
	if got := operationLabel(KardexMovimiento{Asunto: "salida", TipoDocumento: "03", TipoOperacion: "01"}); got != "01-VENTA" {
		t.Errorf("sale: got %q", got)
	}
}

func sampleInsumos(withMovements bool) []KardexInsumo {
	cant, precio, parcial := 8.0, 12.0, 96.0
	ins := KardexInsumo{
		InsumoDetail: InsumoDetail{Insumo: &InsumoInfo{Denomination: "Pollo Test"}},
		Kardex: KardexData{
			Initial: &KardexInitial{},
			Finish:  &KardexFinish{Cantidad: 8, Precio: 12, Parcial: 96},
		},
	}
	if withMovements {
		ins.Kardex.Movimientos = []KardexMovimiento{{
			Asunto: "compra", Fuente: "compra", TipoDocumento: "00", FechaKardex: "2026-11-20",
			CantidadIngreso: &cant, PrecioIngreso: &precio, ParcialIngreso: &parcial,
			CantidadFinal: &cant, PrecioFinal: &precio, CostoFinal: &parcial,
		}}
	}
	return []KardexInsumo{ins}
}

// T31: the completo report prints the global totals once, without an extra page holding only them
func TestCompletoNoTotalsOnlyPage(t *testing.T) {
	cfg := config.Default()
	gen := NewGenerator(Institution{RazonSocial: "Test", RUC: "1"}, "Histórico completo", cfg, "L", "simple", false)
	trk := tracker.New(60000)
	if _, err := gen.Generate(sampleInsumos(true), "t31-completo", t.TempDir(), "k", trk); err != nil {
		t.Fatal(err)
	}
	if gen.TotalPages() != 1 {
		t.Errorf("TotalPages = %d, want 1 (totals must not take a page of their own)", gen.TotalPages())
	}
}

// T31: generate-all must not hand the 13.1 report insumos whose movements the completo report freed
func TestGenerateAllKeepsMovementsFor13_1(t *testing.T) {
	cfg := config.Default()
	insumos := sampleInsumos(true)
	trk := tracker.New(60000)
	res := GenerateAll(insumos, Institution{RazonSocial: "Test", RUC: "1"}, "Histórico completo", "t31-all", t.TempDir(), "k", trk, cfg, "", "simple", false, nil)
	for _, r := range res {
		if r.Error != "" {
			t.Fatalf("%s: %s", r.ReportType, r.Error)
		}
	}
	if len(insumos[0].Kardex.Movimientos) != 1 {
		t.Errorf("movements consumed by completo: len=%d, want 1", len(insumos[0].Kardex.Movimientos))
	}
}
