package kardex

import (
	"strings"
	"fmt"

	"github.com/jung-kurt/gofpdf"

	"kardex-pdf-service/internal/shared"
)

// =============================================================================
// CONSTANTS
// =============================================================================

const (
	lineHt     = 2.1  // Altura de cada línea de texto en datos (compacto extremo)
	headerHt   = 2.4  // Altura de cada línea en headers
	cellGap    = 0.1  // Padding interno de cada celda (mínimo absoluto)
	safetyY    = 7.0  // Margen de seguridad para page break (espacio para headers)
)

// =============================================================================
// HELPERS
// =============================================================================

// maxContentY retorna la coordenada Y máxima usable antes de necesitar page break.
// Se calcula dinámicamente según el tamaño de página (portrait vs landscape).
func (g *Generator) maxContentY(pdf *gofpdf.Fpdf) float64 {
	_, pageHeight := pdf.GetPageSize()
	_, _, _, bottomMargin := pdf.GetMargins()
	return pageHeight - bottomMargin - safetyY
}

// wrapText divide un texto en múltiples líneas que caben dentro de maxWidth.
// Usa SplitLines del propio gofpdf (respeta la font actual).
func wrapText(pdf *gofpdf.Fpdf, text string, maxWidth float64) []string {
	if text == "" {
		return []string{""}
	}
	lines := pdf.SplitLines([]byte(text), maxWidth-cellGap-cellGap)
	if len(lines) == 0 {
		return []string{""}
	}
	result := make([]string, len(lines))
	for i, l := range lines {
		result[i] = string(l)
	}
	return result
}

// rowHeight calcula la altura de una fila dadas las líneas de texto
// que debe contener la celda más alta.
func rowHeight(lineCount int) float64 {
	return float64(lineCount)*lineHt + cellGap*2
}

// shortDate convierte 'YYYY-MM-DD' a 'DD/MM/YY' para que el vencimiento
// quepa en la columna F. Venc (8 caracteres en vez de 10).
func shortDate(s string) string {
	if len(s) == 10 && s[4] == '-' && s[7] == '-' {
		return s[8:10] + "/" + s[5:7] + "/" + s[2:4]
	}
	return s
}

// =============================================================================
// PRODUCT SUMMARY TABLE
// =============================================================================

// addProductSummaryPage agrega la tabla de resumen de productos al PDF actual.
func (g *Generator) addProductSummaryPage(pdf *gofpdf.Fpdf, insumos []KardexInsumo) {
	pdf.SetFont("Arial", "B", 10)
	pdf.CellFormat(0, 6, "RESUMEN DE PRODUCTOS", "", 1, "C", false, 0, "")
	pdf.Ln(2)

	_, _, colWidths := g.summaryColWidths(pdf)
	maxY := g.maxContentY(pdf)

	// Headers
	g.renderSummaryHeaders(pdf, colWidths)

	// Datos
	pdf.SetFont("Arial", "", 5.5)
	for i, insumo := range insumos {
		initial := insumo.Kardex.Initial
		finish := insumo.Kardex.Finish

		denomination := "Sin denominacion"
		barcode := "S/C"
		if insumo.InsumoDetail.Insumo != nil {
			if insumo.InsumoDetail.Insumo.Denomination != "" {
				denomination = shared.ToLatin(insumo.InsumoDetail.Insumo.Denomination)
			}
			if insumo.InsumoDetail.Insumo.Barcode != "" {
				barcode = shared.ToLatin(insumo.InsumoDetail.Insumo.Barcode)
			}
		}

		hasNegative := false
		var initialQty, initialPrice, initialTotal float64
		var finalQty, finalPrice, finalTotal float64

		if initial != nil {
			initialQty = initial.Cantidad
			initialPrice = initial.Precio
			initialTotal = initial.Parcial
			if initialQty < 0 || initialPrice < 0 {
				hasNegative = true
			}
		}
		if finish != nil {
			finalQty = finish.Cantidad
			finalPrice = finish.Precio
			finalTotal = finish.Parcial
			if finalQty < 0 || finalPrice < 0 {
				hasNegative = true
			}
		}

		// Pre-calcular líneas de denominación para saber la altura de la fila
		pdf.SetFont("Arial", "", 5.5)
		denomLines := wrapText(pdf, denomination, colWidths[2])
		rh := rowHeight(len(denomLines))

		// Page break dinámico
		if pdf.GetY()+rh > maxY {
			pdf.AddPage()
			g.renderSummaryHeaders(pdf, colWidths)
			pdf.SetFont("Arial", "", 5.5)
		}

		// Aplicar estilo de fila (zebra)
		g.style.ApplyData(pdf, i)

		// Color para valores negativos
		if hasNegative {
			colorRedSoft.text(pdf)
		}

		y := pdf.GetY()
		leftMargin, _, _, _ := pdf.GetMargins()
		x := leftMargin

		// Dibujar bordes de todas las celdas con la altura de la fila
		cx := x
		rectFlag := g.style.DataRectFlag()
		for j := 0; j < 9; j++ {
			pdf.Rect(cx, y, colWidths[j], rh, rectFlag)
			cx += colWidths[j]
		}

		// Contenido de celdas de una sola línea (centrado verticalmente)
		singleLineY := y + cellGap + (rh-lineHt)/2

		// Nro
		pdf.SetXY(x+cellGap, singleLineY)
		pdf.CellFormat(colWidths[0]-cellGap*2, lineHt, fmt.Sprintf("%d", i+1), "", 0, "C", false, 0, "")

		// Codigo
		pdf.SetXY(x+colWidths[0]+cellGap, singleLineY)
		pdf.CellFormat(colWidths[1]-cellGap*2, lineHt, barcode, "", 0, "C", false, 0, "")

		// Denominación (multi-línea)
		denomX := x + colWidths[0] + colWidths[1] + cellGap
		denomLineY := y + cellGap
		for _, line := range denomLines {
			pdf.SetXY(denomX, denomLineY)
			pdf.CellFormat(colWidths[2]-cellGap*2, lineHt, line, "", 0, "L", false, 0, "")
			denomLineY += lineHt
		}

		// Saldo Inicial (cols 3,4,5)
		siX := x + colWidths[0] + colWidths[1] + colWidths[2]
		pdf.SetXY(siX+cellGap, singleLineY)
		pdf.CellFormat(colWidths[3]-cellGap*2, lineHt, shared.FormatNumber(initialQty), "", 0, "R", false, 0, "")
		pdf.SetXY(siX+colWidths[3]+cellGap, singleLineY)
		pdf.CellFormat(colWidths[4]-cellGap*2, lineHt, shared.FormatNumber(initialPrice), "", 0, "R", false, 0, "")
		pdf.SetXY(siX+colWidths[3]+colWidths[4]+cellGap, singleLineY)
		pdf.CellFormat(colWidths[5]-cellGap*2, lineHt, shared.FormatNumber(initialTotal), "", 0, "R", false, 0, "")

		// Saldo Final (cols 6,7,8)
		sfX := siX + colWidths[3] + colWidths[4] + colWidths[5]
		pdf.SetXY(sfX+cellGap, singleLineY)
		pdf.CellFormat(colWidths[6]-cellGap*2, lineHt, shared.FormatNumber(finalQty), "", 0, "R", false, 0, "")
		pdf.SetXY(sfX+colWidths[6]+cellGap, singleLineY)
		pdf.CellFormat(colWidths[7]-cellGap*2, lineHt, shared.FormatNumber(finalPrice), "", 0, "R", false, 0, "")
		pdf.SetXY(sfX+colWidths[6]+colWidths[7]+cellGap, singleLineY)
		pdf.CellFormat(colWidths[8]-cellGap*2, lineHt, shared.FormatNumber(finalTotal), "", 0, "R", false, 0, "")

		pdf.SetXY(leftMargin, y+rh)
	}

	colorBlack.text(pdf)
}

func (g *Generator) renderSummaryHeaders(pdf *gofpdf.Fpdf, colWidths []float64) {
	pdf.SetFont("Arial", "B", 6)
	g.style.ApplyHeader(pdf)
	hbf := g.style.HeaderBorderFlag()

	// Fila 1
	pdf.CellFormat(colWidths[0], headerHt, "Nro", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[1], headerHt, "Codigo", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[2], headerHt, "Producto / Insumo", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[3]+colWidths[4]+colWidths[5], headerHt, "Saldo Inicial", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[6]+colWidths[7]+colWidths[8], headerHt, "Saldo Final", hbf, 0, "C", true, 0, "")
	pdf.Ln(-1)

	// Fila 2
	pdf.CellFormat(colWidths[0], headerHt, "", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[1], headerHt, "", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[2], headerHt, "", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[3], headerHt, "Cant.", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[4], headerHt, "Precio", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[5], headerHt, "Total", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[6], headerHt, "Cant.", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[7], headerHt, "Precio", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[8], headerHt, "Total", hbf, 0, "C", true, 0, "")
	pdf.Ln(-1)
}

// =============================================================================
// INSUMO DETAIL PAGE
// =============================================================================

// addInsumoPage agrega la tabla de detalle de un insumo al PDF actual.
func (g *Generator) addInsumoPage(pdf *gofpdf.Fpdf, insumo *KardexInsumo, index, total int, showSerie, showLotes, showEstado bool) {
	denomination := "Sin denominacion"
	barcode := "S/C"
	if insumo.InsumoDetail.Insumo != nil {
		if insumo.InsumoDetail.Insumo.Denomination != "" {
			denomination = shared.ToLatin(insumo.InsumoDetail.Insumo.Denomination)
		}
		if insumo.InsumoDetail.Insumo.Barcode != "" {
			barcode = shared.ToLatin(insumo.InsumoDetail.Insumo.Barcode)
		}
	}

	title := fmt.Sprintf("%d/%d: %s (%s)", index, total, denomination, barcode)

	// Separador entre productos — línea dasheada corta y delgada
	g.style.ApplySeparator(pdf)
	leftMargin, _, _, _ := pdf.GetMargins()
	pageWidth, _ := pdf.GetPageSize()
	pdf.SetDashPattern([]float64{0.8, 0.8}, 0)
	pdf.Line(leftMargin, pdf.GetY(), pageWidth-leftMargin, pdf.GetY())
	pdf.SetDashPattern([]float64{}, 0) // reset a sólido para los siguientes draws
	pdf.Ln(0.3)

	pdf.SetFont("Arial", "", 6.5)
	colorBlack.text(pdf)
	pdf.CellFormat(0, 3.5, title, "", 1, "L", false, 0, "")
	pdf.Ln(0.3)

	// Filtrar proformas
	validMovements := make([]KardexMovimiento, 0, len(insumo.Kardex.Movimientos))
	for _, m := range insumo.Kardex.Movimientos {
		if m.Asunto != "proforma" {
			validMovements = append(validMovements, m)
		}
	}

	g.buildKardexTable(pdf, title, validMovements, insumo.Kardex.Initial, insumo.Kardex.Finish, showSerie, showLotes, showEstado)
}

// =============================================================================
// KARDEX TABLE
// =============================================================================

// addContinuationHeader maneja el page break agregando una línea "Continua: <producto>".
// Se usa en buildKardexTable cuando los datos no entran en la página actual.
func (g *Generator) addContinuationHeader(pdf *gofpdf.Fpdf, title string) {
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 6.5)
	colorBlack.text(pdf)
	pdf.CellFormat(0, 3.5, "Continua: "+title, "", 1, "L", false, 0, "")
	pdf.Ln(0.3)
	pdf.SetFont("Arial", "", 5.5)
}

func (g *Generator) buildKardexTable(pdf *gofpdf.Fpdf, title string, movements []KardexMovimiento, initial *KardexInitial, finish *KardexFinish, showSerie bool, showLotes bool, showEstado bool) {
	// showLotes/showEstado se deciden a NIVEL SECCIÓN (kardexSectionFlags) para que
	// cabecera y filas compartan exactamente las mismas columnas.
	colWidths := g.kardexColWidths(pdf, showSerie, showLotes, showEstado)
	maxY := g.maxContentY(pdf)
	rh := rowHeight(1) // Todas las filas de movimientos son 1 línea (datos numéricos cortos)
	leftMargin, _, _, _ := pdf.GetMargins()

	// Sin auto page break: la tabla maneja SUS saltos manualmente (con headers) —
	// el auto-break de gofpdf agrega páginas en blanco al final del documento.
	apb, apbMargin := pdf.GetAutoPageBreak()
	pdf.SetAutoPageBreak(false, apbMargin)
	defer pdf.SetAutoPageBreak(apb, apbMargin)

	pdf.SetFont("Arial", "", 5.5)

	// Fila de saldo inicial
	if initial != nil {
		if pdf.GetY()+rh > maxY {
			g.addContinuationHeader(pdf, title)
		}
		g.style.ApplySaldoInit(pdf)
		g.renderKardexDataRowContent(pdf, colWidths, "", "", "", "", "", "S. INICIAL", true,
			"", "", "", "", "", "",
			shared.FormatNumber(initial.Cantidad), shared.FormatNumber(initial.Precio), shared.FormatNumber(initial.Parcial),
			showSerie, showLotes, showEstado, "", "", "", "")
	}

	// Movimientos
	var totals struct {
		cantidadEntradas float64
		costoEntradas    float64
		cantidadSalidas  float64
		costoSalidas     float64
	}

	for idx, m := range movements {
		// Page break dinámico
		if pdf.GetY()+rh > maxY {
			g.addContinuationHeader(pdf, title)
		}

		isSalida := m.Asunto == "salida"
		tipoOp := shared.ToLatin(fmt.Sprintf("%s-VEN", m.TipoDocumento))

		// Estado SUNAT no válido → etiqueta en rojo sobre fondo rojo claro
		// (equivalente a table-danger en la web). No suman stock.
		estadoLbl := ""
		switch {
		case m.EstadoKardex == "0":
			estadoLbl = "(ANULADO)"
		case m.EstadoDocumento == "04":
			estadoLbl = "(RECHAZADO)"
		case m.EstadoDocumento == "03":
			estadoLbl = "(OBSERVADO)"
		}
		if estadoLbl != "" {
			tipoOp += " " + estadoLbl
		}

		var entCant, entPrecio, entTotal string
		var salCant, salPrecio, salTotal string

		if !isSalida {
			cantIngreso := shared.GetFloatValue(m.CantidadIngreso)
			precioIngreso := shared.GetFloatValue(m.PrecioIngreso)
			parcialIngreso := shared.GetFloatValue(m.ParcialIngreso)
			entCant = shared.FormatNumber(cantIngreso)
			entPrecio = shared.FormatNumber(precioIngreso)
			entTotal = shared.FormatNumber(parcialIngreso)
			totals.cantidadEntradas += cantIngreso
			totals.costoEntradas += parcialIngreso
		}

		if isSalida {
			cantSalida := shared.GetFloatValue(m.CantidadSalida)
			precioSalida := shared.GetFloatValue(m.PrecioSalida)
			parcialSalida := shared.GetFloatValue(m.ParcialSalida)
			salCant = shared.FormatNumber(cantSalida)
			salPrecio = shared.FormatNumber(precioSalida)
			salTotal = shared.FormatNumber(parcialSalida)
			totals.cantidadSalidas += cantSalida
			totals.costoSalidas += parcialSalida
		}

		// Columnas extra — espejo del HTML (SERIES / LOTES / F.VENC / ESTADO SUNAT)
		serieCol := ""
		if len(m.Series) > 0 {
			serieCol = shared.ToLatin(m.Series[0].Code)
			if len(m.Series) > 1 {
				serieCol += fmt.Sprintf(" (+%d)", len(m.Series)-1)
			}
		}
		estadoCol := ""
		switch {
		case m.EstadoKardex == "0":
			estadoCol = "ANU"
		case m.EstadoDocumento == "04":
			estadoCol = "REC"
		case m.EstadoDocumento == "03":
			estadoCol = "OBS"
		}
		loteCol, vencCol := "", ""
		if len(m.Lotes) > 0 {
			loteCol = shared.ToLatin(m.Lotes[0].Lote)
			if len(m.Lotes) > 1 {
				loteCol += fmt.Sprintf(" (+%d)", len(m.Lotes)-1)
			}
			vencCol = shared.ToLatin(shortDate(m.Lotes[0].Vencimiento))
		}

		// Aplicar estilo según tipo de fila (salida vs entrada, con zebra)
		if isSalida {
			g.style.ApplySalida(pdf, idx)
		} else {
			g.style.ApplyData(pdf, idx)
		}

		// Fila no válida: fondo rojo claro + texto rojo (estilo table-danger)
		if estadoLbl != "" {
			pdf.SetFillColor(255, 231, 230)
			pdf.SetTextColor(185, 28, 28)
		}

		g.renderKardexDataRowContent(pdf, colWidths,
			fmt.Sprintf("%d", idx+1),
			shared.ToLatin(m.FechaKardex),
			shared.ToLatin(m.TipoDocumento),
			shared.ToLatin(m.SerieDocumento),
			shared.ToLatin(m.NumeroDocumento),
			tipoOp,
			false,
			entCant, entPrecio, entTotal,
			salCant, salPrecio, salTotal,
			shared.FormatNumber(shared.GetFloatValue(m.CantidadFinal)),
			shared.FormatNumber(shared.GetFloatValue(m.PrecioFinal)),
			shared.FormatNumber(shared.GetFloatValue(m.CostoFinal)),
			showSerie, showLotes, showEstado, serieCol, loteCol, vencCol, estadoCol,
		)

		// Restaurar color de texto tras filas no válidas
		if estadoLbl != "" {
			pdf.SetTextColor(0, 0, 0)
		}

		// Sub-línea de series del movimiento: label en negrita + códigos (compacta)
		if len(m.Series) > 0 {
			codes := make([]string, 0, len(m.Series))
			for _, sr := range m.Series {
				codes = append(codes, shared.ToLatin(sr.Code))
			}
			serieTxt := strings.Join(codes, ", ")
			leftM, _, rightM, _ := pdf.GetMargins()
			pageW, _ := pdf.GetPageSize()
			avail := pageW - leftM - rightM

			pdf.SetFont("Arial", "", 4.5)
			lines := wrapText(pdf, serieTxt, avail-11) // 11pt = ancho del label "Series:"

			for li, ln := range lines {
				if pdf.GetY()+1.9 > maxY {
					g.addContinuationHeader(pdf, title)
				}
				pdf.Rect(leftMargin, pdf.GetY(), avail, 1.9, "F")
				if li == 0 {
					pdf.SetFont("Arial", "B", 4.5)
					pdf.SetXY(leftMargin+0.5, pdf.GetY()+0.05)
					pdf.CellFormat(10, 1.5, "Series:", "", 0, "L", false, 0, "")
				}
				pdf.SetFont("Arial", "", 4.5)
				pdf.SetXY(leftMargin+10.5, pdf.GetY()+0.05)
				pdf.CellFormat(avail-11, 1.5, shared.ToLatin(ln), "", 0, "L", false, 0, "")
				pdf.SetY(pdf.GetY() + 1.9)
			}
			pdf.SetFont("Arial", "", 5.5)
		}

		// Sub-línea de lotes del movimiento: lote (vence X) x cant
		if len(m.Lotes) > 0 {
			parts := make([]string, 0, len(m.Lotes))
			for _, lt := range m.Lotes {
				pp := shared.ToLatin(lt.Lote)
				if lt.Vencimiento != "" {
					pp += " (vence " + shared.ToLatin(shortDate(lt.Vencimiento)) + ")"
				}
				if lt.Cantidad > 0 {
					pp += fmt.Sprintf(" x%s", shared.FormatNumber(lt.Cantidad))
				}
				parts = append(parts, pp)
			}
			if pdf.GetY()+1.8 > maxY {
				g.addContinuationHeader(pdf, title)
			}
			pdf.SetFillColor(245, 245, 245)
			pdf.SetFont("Arial", "B", 4.5)
			xl, yl := leftMargin, pdf.GetY()
			pdf.Rect(xl, yl, 8, 1.8, "F")
			pdf.SetXY(xl+0.5, yl+0.05)
			pdf.CellFormat(7, 1.5, "Lote:", "", 0, "L", false, 0, "")
			pdf.SetFont("Arial", "", 4.5)
			pdf.SetXY(xl+8, yl+0.05)
			pdf.CellFormat(0, 1.5, shared.ToLatin(strings.Join(parts, " · ")), "", 0, "L", false, 0, "")
			pdf.SetXY(leftMargin, yl+1.8)
			pdf.SetFont("Arial", "", 5.5)
		}
	}

	// Fila de saldo final
	if finish != nil {
		if pdf.GetY()+rh > maxY {
			g.addContinuationHeader(pdf, title)
		}
		g.style.ApplySaldoFinal(pdf)
		g.renderKardexDataRowContent(pdf, colWidths, "", "", "", "", "", "S. FINAL", true,
			"", "", "", "", "", "",
			shared.FormatNumber(finish.Cantidad), shared.FormatNumber(finish.Precio), shared.FormatNumber(finish.Parcial),
			showSerie, showLotes, showEstado, "", "", "", "")
	}

	// Fila de totales — COSTO DE VENTAS por ecuación contable: Ini + Compras - Fin
	// Solo se dibuja si hay movimientos — evita fila repetida con todos ceros
	if len(movements) > 0 {
		if pdf.GetY()+rh > maxY {
			g.addContinuationHeader(pdf, title)
		}
		var costoNeto float64
		if initial != nil && finish != nil {
			costoNeto = initial.Parcial + totals.costoEntradas - finish.Parcial
		} else {
			costoNeto = totals.costoEntradas - totals.costoSalidas
		}
		pdf.SetFont("Arial", "B", 5.5)
		g.style.ApplyTotales(pdf)
		dbf := g.style.DataBorderFlag()
		// Fila TOTALES con celdas mergeadas — coincide con el formato del txt de referencia
		// (sin celdas vacías: PUnit y Sdo.Cant/PUnit se omiten)
		pdf.CellFormat(colWidths[0]+colWidths[1]+colWidths[2]+colWidths[3]+colWidths[4]+colWidths[5], lineHt, "TOTALES:", dbf, 0, "C", true, 0, "")
		pdf.CellFormat(colWidths[6], lineHt, shared.FormatNumber(totals.cantidadEntradas), dbf, 0, "R", true, 0, "")
		pdf.CellFormat(colWidths[7]+colWidths[8], lineHt, shared.FormatNumber(totals.costoEntradas), dbf, 0, "R", true, 0, "")
		pdf.CellFormat(colWidths[9], lineHt, shared.FormatNumber(totals.cantidadSalidas), dbf, 0, "R", true, 0, "")
		pdf.CellFormat(colWidths[10]+colWidths[11], lineHt, shared.FormatNumber(totals.costoSalidas), dbf, 0, "R", true, 0, "")
		pdf.CellFormat(colWidths[12]+colWidths[13]+colWidths[14], lineHt, fmt.Sprintf("COSTO DE VENTAS: %s", shared.FormatNumber(costoNeto)), dbf, 0, "R", true, 0, "")
		extraIdx := 15
		if showSerie {
			pdf.CellFormat(colWidths[extraIdx], lineHt, "", dbf, 0, "C", true, 0, "")
			extraIdx++
		}
		if showLotes {
			pdf.CellFormat(colWidths[extraIdx]+colWidths[extraIdx+1], lineHt, "", dbf, 0, "C", true, 0, "")
			extraIdx += 2
		}
		if showEstado {
			pdf.CellFormat(colWidths[extraIdx], lineHt, "", dbf, 0, "C", true, 0, "")
		}
		pdf.Ln(-1)
	}
}

// renderKardexDataRowContent dibuja una fila completa de la tabla kardex con bordes.
// Los strings vacíos generan celdas vacías. Si boldLabel != "" se usa font bold.
// NOTA: El color/fondo debe aplicarse ANTES de llamar este método.
func (g *Generator) renderKardexDataRowContent(pdf *gofpdf.Fpdf, cw []float64,
	nro, fecha, tipoDoc, serie, numero, tipoOp string, boldLabel bool,
	entCant, entPrecio, entTotal string,
	salCant, salPrecio, salTotal string,
	saldoCant, saldoPrecio, saldoTotal string,
	showSerie, showLotes, showEstado bool, serieTxt, loteTxt, vencTxt, estadoTxt string,
) {
	if boldLabel {
		pdf.SetFont("Arial", "B", 5.5)
	} else {
		pdf.SetFont("Arial", "", 5.5)
	}

	leftMargin, _, _, _ := pdf.GetMargins()
	x := leftMargin
	y := pdf.GetY()
	rh := rowHeight(1)

	// Columnas: [0]Nro [1]Fecha [2]TipoDoc [3]Serie [4]Numero [5]TipoOp
	// [6]EntCant [7]EntPrecio [8]EntTotal [9]SalCant [10]SalPrecio [11]SalTotal
	// [12]SaldoCant [13]SaldoPrecio [14]SaldoTotal
	texts := []string{nro, fecha, tipoDoc, serie, numero, tipoOp,
		entCant, entPrecio, entTotal, salCant, salPrecio, salTotal,
		saldoCant, saldoPrecio, saldoTotal}
	aligns := []string{"C", "C", "C", "C", "C", "C",
		"R", "R", "R", "R", "R", "R",
		"R", "R", "R"}
	if showSerie {
		texts = append(texts, serieTxt)
		aligns = append(aligns, "L")
	}
	if showLotes {
		texts = append(texts, loteTxt, vencTxt)
		aligns = append(aligns, "L", "C")
	}
	if showEstado {
		texts = append(texts, estadoTxt)
		aligns = append(aligns, "C")
	}

	// Dibujar bordes y texto — auto-fit: si el texto excede su celda baja la
	// fuente (piso 4.2pt) para que NUNCA se desborde (lotes/vencimientos largos).
	cx := x
	rectFlag := g.style.DataRectFlag()
	fontStyle := ""
	if boldLabel {
		fontStyle = "B"
	}
	for i, txt := range texts {
		pdf.Rect(cx, y, cw[i], rh, rectFlag)
		if txt != "" {
			availW := cw[i] - cellGap*2
			fontSize := 5.5
			for fontSize > 4.2 && pdf.GetStringWidth(txt) > availW {
				fontSize -= 0.25
			}
			pdf.SetFont("Arial", fontStyle, fontSize)
			pdf.SetXY(cx+cellGap, y+cellGap)
			pdf.CellFormat(availW, lineHt, txt, "", 0, aligns[i], false, 0, "")
		}
		cx += cw[i]
	}
	pdf.SetFont("Arial", fontStyle, 5.5)

	pdf.SetXY(x, y+rh)
}

// =============================================================================
// HEADER HELPERS (DRY)
// =============================================================================

func (g *Generator) renderKardexHeaderRow1(pdf *gofpdf.Fpdf, colWidths []float64, showSerie, showLotes, showEstado bool) {
	pdf.SetFont("Arial", "B", 5.5)
	g.style.ApplyHeader(pdf)
	// Cabecera de la sección seguimiento siempre con bordes (ignora el estilo)
	hbf := "1"
	colorBlack.draw(pdf)
	pdf.SetLineWidth(0.2)

	pdf.CellFormat(colWidths[0], headerHt, "Nro", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[1], headerHt, "Fecha", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[2]+colWidths[3]+colWidths[4], headerHt, "Comprobante", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[5], headerHt, "Tipo Op.", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[6]+colWidths[7]+colWidths[8], headerHt, "Entradas", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[9]+colWidths[10]+colWidths[11], headerHt, "Salidas", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[12]+colWidths[13]+colWidths[14], headerHt, "Saldo Final", hbf, 0, "C", true, 0, "")
	extraIdx := 15
	if showSerie {
		pdf.CellFormat(colWidths[extraIdx], headerHt, "SERIES", hbf, 0, "C", true, 0, "")
		extraIdx++
	}
	if showLotes {
		pdf.CellFormat(colWidths[extraIdx]+colWidths[extraIdx+1], headerHt, "LOTES", hbf, 0, "C", true, 0, "")
		extraIdx += 2
	}
	if showEstado {
		pdf.CellFormat(colWidths[extraIdx], headerHt, "ESTADO", hbf, 0, "C", true, 0, "")
	}
}

func (g *Generator) renderKardexHeaderRow2(pdf *gofpdf.Fpdf, colWidths []float64, showSerie, showLotes, showEstado bool) {
	pdf.SetFont("Arial", "B", 5.5)
	g.style.ApplyHeader(pdf)
	// Cabecera de la sección seguimiento siempre con bordes (ignora el estilo)
	hbf := "1"
	colorBlack.draw(pdf)
	pdf.SetLineWidth(0.2)

	pdf.CellFormat(colWidths[0], headerHt, "", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[1], headerHt, "", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[2], headerHt, "Tipo", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[3], headerHt, "Serie", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[4], headerHt, "Numero", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[5], headerHt, "", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[6], headerHt, "Cant.", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[7], headerHt, "P.Unit.", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[8], headerHt, "Total", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[9], headerHt, "Cant.", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[10], headerHt, "P.Unit.", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[11], headerHt, "Total", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[12], headerHt, "Cant.", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[13], headerHt, "P.Unit.", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(colWidths[14], headerHt, "Total", hbf, 0, "C", true, 0, "")
	extraIdx := 15
	if showSerie {
		pdf.CellFormat(colWidths[extraIdx], headerHt, shared.ToLatin("Código"), hbf, 0, "C", true, 0, "")
		extraIdx++
	}
	if showLotes {
		pdf.CellFormat(colWidths[extraIdx], headerHt, "Lote", hbf, 0, "C", true, 0, "")
		pdf.CellFormat(colWidths[extraIdx+1], headerHt, "F. Venc", hbf, 0, "C", true, 0, "")
		extraIdx += 2
	}
	if showEstado {
		pdf.CellFormat(colWidths[extraIdx], headerHt, "Estado", hbf, 0, "C", true, 0, "")
	}
}

// =============================================================================
// DYNAMIC COLUMN WIDTHS
// =============================================================================

// kardexColWidths calcula los anchos de las 15 columnas adaptándose a la orientación.
func (g *Generator) kardexColWidths(pdf *gofpdf.Fpdf, showSerie bool, showLotes bool, showEstado bool) []float64 {
	leftMargin, _, rightMargin, _ := pdf.GetMargins()
	pageWidth, _ := pdf.GetPageSize()
	available := pageWidth - leftMargin - rightMargin

	// Ratios base — se normalizan al total para que las celdas ocupen todo el ancho.
	ratios := []float64{
		3.50, 6.58, 5.31, 5.31, 6.58, 9.66,
		6.58, 6.58, 7.86,
		6.58, 6.58, 7.86,
		6.58, 6.58, 7.86,
	}
	if showSerie {
		ratios = append(ratios, 7.0)
	}
	if showLotes {
		ratios = append(ratios, 7.2, 5.4)
	}
	if showEstado {
		ratios = append(ratios, 3.0)
	}

	total := 0.0
	for _, r := range ratios {
		total += r
	}

	widths := make([]float64, len(ratios))
	for i, r := range ratios {
		widths[i] = (r / total) * available
	}
	return widths
}

// kardexSectionFlags decide si la sección de seguimiento muestra las columnas
// LOTES/F.VENC y ESTADO (espejo del HTML). Se calcula a nivel sección para que
// la cabecera única y todas las filas compartan exactamente las mismas columnas.
func kardexSectionFlags(insumos []KardexInsumo) (showSerie, showLotes, showEstado bool) {
	for i := range insumos {
		for _, m := range insumos[i].Kardex.Movimientos {
			if len(m.Series) > 0 {
				showSerie = true
			}
			if len(m.Lotes) > 0 {
				showLotes = true
			}
			if m.EstadoKardex == "0" || m.EstadoDocumento == "03" || m.EstadoDocumento == "04" {
				showEstado = true
			}
			if showSerie && showLotes && showEstado {
				return
			}
		}
	}
	return
}

// summaryColWidths calcula los anchos de las 9 columnas del resumen.
func (g *Generator) summaryColWidths(pdf *gofpdf.Fpdf) (fixedWidths []float64, denominationWidth float64, colWidths []float64) {
	leftMargin, _, rightMargin, _ := pdf.GetMargins()
	pageWidth, _ := pdf.GetPageSize()
	available := pageWidth - leftMargin - rightMargin

	fixedRatios := []float64{3.5, 7.1, 5.7, 5.7, 7.1, 5.7, 5.7, 7.1}
	totalFixedRatio := float64(0)
	for _, r := range fixedRatios {
		totalFixedRatio += r
	}

	fixedWidths = make([]float64, 8)
	for i, r := range fixedRatios {
		fixedWidths[i] = (r / 100.0) * available
	}

	denominationWidth = (100.0 - totalFixedRatio) / 100.0 * available

	colWidths = []float64{
		fixedWidths[0], fixedWidths[1], denominationWidth,
		fixedWidths[2], fixedWidths[3], fixedWidths[4],
		fixedWidths[5], fixedWidths[6], fixedWidths[7],
	}
	return
}
