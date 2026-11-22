package kardex

import (
	"fmt"

	"github.com/jung-kurt/gofpdf"

	"kardex-pdf-service/internal/shared"
)

// =============================================================================
// GLOBAL TOTALS — Totales globales del kardex (todos los productos)
// =============================================================================

// GlobalTotals contiene los totales globales del kardex para todos los productos.
// Se calcula ANTES de procesar los insumos (antes de que se liberen los movimientos).
type GlobalTotals struct {
	TotalCompras    float64 // Σ todas las entradas (parcial_ingreso)
	TotalVentas     float64 // Σ todas las salidas (parcial_salida)
	TotalInvInicial float64 // Σ todos los saldos iniciales (initial.parcial)
	TotalInvFinal   float64 // Σ todos los saldos finales (finish.parcial)
	TotalNegativos  int     // Cantidad de productos con valores negativos
	CostoVentas     float64 // Inv.Inicial + Compras - Inv.Final (ecuación contable)
	Merma           float64 // 0 para kardex genérico (específico de combustibles)
	CVNeta          float64 // CostoVentas - Merma
}

// ComputeGlobalTotals calcula los totales globales a partir de todos los insumos.
// DEBE llamarse ANTES de procesar los insumos (antes de que se liberen los movimientos).
func ComputeGlobalTotals(insumos []KardexInsumo) GlobalTotals {
	var gt GlobalTotals

	for _, ins := range insumos {
		// Sumar movimientos (entradas y salidas)
		for _, m := range ins.Kardex.Movimientos {
			if m.Asunto == "proforma" {
				continue
			}
			if m.Asunto == "salida" {
				gt.TotalVentas += shared.GetFloatValue(m.ParcialSalida)
			} else {
				gt.TotalCompras += shared.GetFloatValue(m.ParcialIngreso)
			}
		}

		// Sumar saldos iniciales
		if ins.Kardex.Initial != nil {
			gt.TotalInvInicial += ins.Kardex.Initial.Parcial
		}

		// Sumar saldos finales + detectar negativos
		if ins.Kardex.Finish != nil {
			gt.TotalInvFinal += ins.Kardex.Finish.Parcial
			if ins.Kardex.Finish.Cantidad < 0 || ins.Kardex.Finish.Precio < 0 {
				gt.TotalNegativos++
			}
		}
	}

	// Ecuación contable: CostoVentas = Inv.Inicial + Compras - Inv.Final
	gt.CostoVentas = gt.TotalInvInicial + gt.TotalCompras - gt.TotalInvFinal
	gt.Merma = 0 // kardex genérico: sin merma
	gt.CVNeta = gt.CostoVentas - gt.Merma

	return gt
}

// =============================================================================
// PDF RENDERING — Secciones de totales globales
// =============================================================================

// AddGlobalTotalsSection agrega la sección de totales globales al PDF.
// Incluye 3 secciones fieles al formato de completo.txt:
//   - TOTAL GENERAL: COMPRAS | VENTAS | COSTO DE VENTAS
//   - RESUMEN INVENTARIO: Inv.Inicial | Inv.Final | Evaluación | Negativos
//   - ANALISIS DE COSTOS: Merma | CV Neta | Total Salidas
func (g *Generator) AddGlobalTotalsSection(pdf *gofpdf.Fpdf, gt GlobalTotals) {
	leftMargin, _, rightMargin, _ := pdf.GetMargins()
	pageWidth, _ := pdf.GetPageSize()
	available := pageWidth - leftMargin - rightMargin
	rh := rowHeight(1)

	// ─── SEPARATOR ───
	g.drawSeparator(pdf, leftMargin, pageWidth, rightMargin)
	pdf.Ln(1.5)

	// ──────────────────────────────────────────────────────────────
	// SECTION 1: TOTAL GENERAL
	// Formato: TOTAL GENERAL | COMPRAS: x | VENTAS: x | C. VENTAS: x
	// ──────────────────────────────────────────────────────────────
	pdf.SetFont("Arial", "B", 5.5)
	g.style.ApplyTotales(pdf)

	colLabel := available * 0.18
	colValue := available * 0.82 / 3 // 3 columnas de datos

	g.renderGlobalRow(pdf, leftMargin, colLabel, colValue, available, rh, []globalCell{
		{label: "TOTAL GENERAL", value: "", align: "L", isTitle: true},
		{label: "COMPRAS: ", value: shared.FormatNumber(gt.TotalCompras), align: "R"},
		{label: "VENTAS: ", value: shared.FormatNumber(gt.TotalVentas), align: "R"},
		{label: "C. VENTAS: ", value: shared.FormatNumber(gt.CostoVentas), align: "R"},
	})
	pdf.Ln(1.5)

	// ─── DOUBLE SEPARATOR ───
	g.drawDoubleSeparator(pdf, leftMargin, pageWidth, rightMargin)
	pdf.Ln(1.5)

	// ──────────────────────────────────────────────────────────────
	// SECTION 2: RESUMEN INVENTARIO
	// Formato: Inv.Inicial | Inv.Final | Evaluación | Negativos
	// ──────────────────────────────────────────────────────────────
	pdf.SetFont("Arial", "B", 5.5)
	g.style.ApplyTotales(pdf)

	evaluacion := gt.TotalInvFinal - gt.TotalInvInicial
	colW := available / 4

	g.renderGlobalRow(pdf, leftMargin, 0, colW, available, rh, []globalCell{
		{label: "INV. INICIAL: ", value: shared.FormatNumber(gt.TotalInvInicial), align: "R"},
		{label: "INV. FINAL: ", value: shared.FormatNumber(gt.TotalInvFinal), align: "R"},
		{label: "EVALUACION: ", value: shared.FormatNumber(evaluacion), align: "R"},
		{label: "NEGATIVOS: ", value: fmt.Sprintf("%d", gt.TotalNegativos), align: "R"},
	})
	pdf.Ln(1.5)

	// ─── DOUBLE SEPARATOR ───
	g.drawDoubleSeparator(pdf, leftMargin, pageWidth, rightMargin)
	pdf.Ln(2)

	// ──────────────────────────────────────────────────────────────
	// SECTION 3: ANALISIS DE COSTOS
	// Título + 3 filas: Merma, CV Neta, Total Salidas
	// ──────────────────────────────────────────────────────────────
	pdf.SetFont("Arial", "B", 6.5)
	colorBlack.text(pdf)
	pdf.CellFormat(0, 4, "ANALISIS DE COSTOS", "", 1, "C", false, 0, "")
	pdf.Ln(1)

	costItems := []struct {
		label string
		value string
	}{
		{"COSTO POR MERMA:", shared.FormatNumber(gt.Merma)},
		{"COSTO DE VENTAS NETA:", shared.FormatNumber(gt.CVNeta)},
		{"TOTAL COSTO DE SALIDAS:", shared.FormatNumber(gt.CostoVentas)},
	}

	for i, item := range costItems {
		if i%2 == 1 {
			g.style.ApplyData(pdf, i)
		} else {
			g.style.ApplyTotales(pdf)
		}
		pdf.SetFont("Arial", "", 5.5)

		labelW := available * 0.55
		valueW := available * 0.45

		y := pdf.GetY()
		x := leftMargin
		cx := x
		rectFlag := g.style.DataRectFlag()

		pdf.Rect(cx, y, labelW, rh, rectFlag)
		pdf.SetXY(cx+cellGap, y+cellGap)
		pdf.CellFormat(labelW-cellGap*2, lineHt, item.label, "", 0, "L", false, 0, "")

		pdf.Rect(cx+labelW, y, valueW, rh, rectFlag)
		pdf.SetXY(cx+labelW+cellGap, y+cellGap)
		pdf.CellFormat(valueW-cellGap*2, lineHt, item.value, "", 0, "R", false, 0, "")

		pdf.SetXY(x, y+rh)
	}

	// ─── FINAL SEPARATOR ───
	pdf.Ln(1)
	g.drawSeparator(pdf, leftMargin, pageWidth, rightMargin)

	colorBlack.text(pdf)
}

// globalTotalsHeight measures the rendered height of AddGlobalTotalsSection on a
// scratch PDF with the same page setup (the block has a fixed layout).
func (g *Generator) globalTotalsHeight(gt GlobalTotals) float64 {
	scratch := g.createNewPDF()
	scratch.AddPage()
	y0 := scratch.GetY()
	g.AddGlobalTotalsSection(scratch, gt)
	return scratch.GetY() - y0
}

// addFinalGlobalTotals prints the global totals once at the end of the report,
// on the current page when they fit, otherwise on a new page.
func (g *Generator) addFinalGlobalTotals(pdf *gofpdf.Fpdf, gt GlobalTotals) {
	const gap = 4.0
	if pdf.GetY()+gap+g.globalTotalsHeight(gt) > g.maxContentY(pdf) {
		pdf.AddPage()
	} else {
		pdf.Ln(gap)
	}
	g.AddGlobalTotalsSection(pdf, gt)
}

// =============================================================================
// HELPERS
// =============================================================================

// globalCell representa una celda en una fila de totales globales.
type globalCell struct {
	label   string
	value   string
	align   string
	isTitle bool
}

// renderGlobalRow dibuja una fila de celdas globales con label+value en cada una.
func (g *Generator) renderGlobalRow(pdf *gofpdf.Fpdf, leftMargin, colLabel, colValue, available, rh float64, cells []globalCell) {
	y := pdf.GetY()
	x := leftMargin
	cx := x

	// Si el primer cell es título, usar colLabel para él y distribuir el resto
	startIdx := 0
	rectFlag := g.style.DataRectFlag()
	if len(cells) > 0 && cells[0].isTitle {
		pdf.Rect(cx, y, colLabel, rh, rectFlag)
		pdf.SetXY(cx+cellGap, y+cellGap)
		pdf.CellFormat(colLabel-cellGap*2, lineHt, cells[0].label, "", 0, "L", false, 0, "")
		cx += colLabel
		startIdx = 1
	}

	// Calcular ancho para las celdas restantes
	remaining := available - (cx - x)
	cellCount := len(cells) - startIdx
	if cellCount == 0 {
		pdf.SetXY(x, y+rh)
		return
	}
	cellW := remaining / float64(cellCount)

	for _, cell := range cells[startIdx:] {
		pdf.Rect(cx, y, cellW, rh, rectFlag)

		if cell.isTitle {
			pdf.SetXY(cx+cellGap, y+cellGap)
			pdf.CellFormat(cellW-cellGap*2, lineHt, cell.label, "", 0, cell.align, false, 0, "")
		} else {
			// Label side (40%) + Value side (60%)
			labelW := cellW * 0.45
			valueW := cellW * 0.55

			pdf.SetXY(cx+cellGap, y+cellGap)
			pdf.CellFormat(labelW-cellGap, lineHt, cell.label, "", 0, "R", false, 0, "")

			pdf.SetXY(cx+labelW, y+cellGap)
			pdf.CellFormat(valueW-cellGap, lineHt, cell.value, "", 0, cell.align, false, 0, "")
		}
		cx += cellW
	}

	pdf.SetXY(x, y+rh)
}

// drawSeparator dibuja una línea separadora simple.
func (g *Generator) drawSeparator(pdf *gofpdf.Fpdf, leftMargin, pageWidth, rightMargin float64) {
	g.style.ApplySeparator(pdf)
	pdf.Line(leftMargin, pdf.GetY(), pageWidth-rightMargin, pdf.GetY())
}

// drawDoubleSeparator dibuja una línea separadora doble.
func (g *Generator) drawDoubleSeparator(pdf *gofpdf.Fpdf, leftMargin, pageWidth, rightMargin float64) {
	g.style.ApplySeparator(pdf)
	pdf.Line(leftMargin, pdf.GetY(), pageWidth-rightMargin, pdf.GetY())
	pdf.Ln(0.5)
	pdf.Line(leftMargin, pdf.GetY(), pageWidth-rightMargin, pdf.GetY())
}
